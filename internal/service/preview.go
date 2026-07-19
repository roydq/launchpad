package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/pkg/launchpad"
)

// FoldedPending is the effective staged batch after last-write-wins folding.
type FoldedPending struct {
	Image  string             `json:"image,omitempty"`
	Config map[string]*string `json:"config,omitempty"` // nil value means delete
	// ConfigSensitivity maps keys to plain|secret for staged config (when known).
	ConfigSensitivity map[string]string `json:"config_sensitivity,omitempty"`
	Scales            map[string]int    `json:"scales,omitempty"`
}

func (f FoldedPending) IsEmpty() bool {
	return f.Image == "" && len(f.Config) == 0 && len(f.Scales) == 0
}

// ConfigDiffOp is one effective config delta.
type ConfigDiffOp struct {
	Op          string  `json:"op"` // add | change | remove
	Key         string  `json:"key"`
	From        *string `json:"from,omitempty"`
	To          *string `json:"to,omitempty"`
	Sensitivity string  `json:"sensitivity,omitempty"` // plain | secret when known
}

// ScaleDiffOp is one effective scale delta.
type ScaleDiffOp struct {
	Process string `json:"process"`
	From    *int   `json:"from,omitempty"`
	To      int    `json:"to"`
}

// ImageDiff is image from → to when changed.
type ImageDiff struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// EffectiveDiff is structured delta of pending (or target release) vs baseline.
type EffectiveDiff struct {
	Image  *ImageDiff     `json:"image,omitempty"`
	Config []ConfigDiffOp `json:"config,omitempty"`
	Scale  []ScaleDiffOp  `json:"scale,omitempty"`
}

func (d EffectiveDiff) IsEmpty() bool {
	return d.Image == nil && len(d.Config) == 0 && len(d.Scale) == 0
}

// PreviewResult is the API shape for server-side diff preview.
type PreviewResult struct {
	Mode             string         `json:"mode"` // pending | releases
	Environment      string         `json:"environment,omitempty"`
	BaselineVersion  *int           `json:"baseline_version,omitempty"`
	FromVersion      *int           `json:"from_version,omitempty"`
	ToVersion        *int           `json:"to_version,omitempty"`
	HasPending       bool           `json:"has_pending"`
	MatchesBaseline  bool           `json:"matches_baseline"`
	Pending          *FoldedPending `json:"pending,omitempty"`
	Diff             EffectiveDiff  `json:"diff"`
	Summary          string         `json:"summary"`
}

// FoldChanges applies last-write-wins over changeset rows (shared_config treated as config for preview).
func FoldChanges(changes []domain.ChangesetChange) (FoldedPending, error) {
	out := FoldedPending{
		Config:            make(map[string]*string),
		ConfigSensitivity: make(map[string]string),
		Scales:            make(map[string]int),
	}
	for _, c := range changes {
		switch c.Type {
		case domain.ChangeTypeConfig, domain.ChangeTypeSharedConfig:
			var p domain.ConfigChangePayload
			if err := json.Unmarshal(c.Payload, &p); err != nil {
				return FoldedPending{}, fmt.Errorf("%w: config payload", launchpad.ErrBadRequest)
			}
			if p.Key == "" {
				return FoldedPending{}, fmt.Errorf("%w: config change missing key", launchpad.ErrBadRequest)
			}
			out.Config[p.Key] = p.Value
			if p.Sensitivity != nil {
				if n := domain.NormalizeSensitivity(*p.Sensitivity); n != "" {
					out.ConfigSensitivity[p.Key] = n
				}
			}
		case domain.ChangeTypeScale:
			var p domain.ScaleChangePayload
			if err := json.Unmarshal(c.Payload, &p); err != nil {
				return FoldedPending{}, fmt.Errorf("%w: scale payload", launchpad.ErrBadRequest)
			}
			if p.Process == "" {
				return FoldedPending{}, fmt.Errorf("%w: scale change missing process", launchpad.ErrBadRequest)
			}
			out.Scales[p.Process] = p.Quantity
		case domain.ChangeTypeImage:
			var p domain.ImageChangePayload
			if err := json.Unmarshal(c.Payload, &p); err != nil {
				return FoldedPending{}, fmt.Errorf("%w: image payload", launchpad.ErrBadRequest)
			}
			if p.ArtifactRef == "" {
				return FoldedPending{}, fmt.Errorf("%w: image change missing artifact_ref", launchpad.ErrBadRequest)
			}
			out.Image = p.ArtifactRef
		default:
			return FoldedPending{}, fmt.Errorf("%w: unknown change type %q", launchpad.ErrBadRequest, c.Type)
		}
	}
	return out, nil
}

// BuildDiff computes effective deltas of pending vs a baseline release (nil = empty baseline).
// Secret values are redacted using baseline.ConfigSensitivity and pending.ConfigSensitivity.
func BuildDiff(pending FoldedPending, baseline *domain.Release) EffectiveDiff {
	var baselineImage string
	baselineConfig := map[string]string{}
	baselineSens := map[string]string{}
	baselineScales := map[string]int{}
	if baseline != nil {
		baselineImage = baseline.ArtifactRef
		if baseline.ConfigResolved != nil {
			baselineConfig = baseline.ConfigResolved
		}
		if baseline.ConfigSensitivity != nil {
			baselineSens = baseline.ConfigSensitivity
		}
		for name, snap := range baseline.ProcessSnapshot {
			baselineScales[name] = snap.Quantity
		}
	}
	pendingSens := pending.ConfigSensitivity
	if pendingSens == nil {
		pendingSens = map[string]string{}
	}

	var diff EffectiveDiff
	if pending.Image != "" && pending.Image != baselineImage {
		diff.Image = &ImageDiff{From: baselineImage, To: pending.Image}
	}

	var keys []string
	for k := range pending.Config {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		val := pending.Config[key]
		old, had := baselineConfig[key]
		sens := configDiffSensitivity(baselineSens[key], pendingSens[key])
		if val == nil {
			if !had {
				continue
			}
			from := domain.RedactConfigValue(old, sens)
			diff.Config = append(diff.Config, ConfigDiffOp{Op: "remove", Key: key, From: &from, Sensitivity: sens})
			continue
		}
		if !had {
			to := domain.RedactConfigValue(*val, sens)
			diff.Config = append(diff.Config, ConfigDiffOp{Op: "add", Key: key, To: &to, Sensitivity: sens})
			continue
		}
		if old == *val {
			continue
		}
		from := domain.RedactConfigValue(old, sens)
		to := domain.RedactConfigValue(*val, sens)
		diff.Config = append(diff.Config, ConfigDiffOp{Op: "change", Key: key, From: &from, To: &to, Sensitivity: sens})
	}

	var procs []string
	for p := range pending.Scales {
		procs = append(procs, p)
	}
	sort.Strings(procs)
	for _, proc := range procs {
		qty := pending.Scales[proc]
		old, had := baselineScales[proc]
		if had && old == qty {
			continue
		}
		op := ScaleDiffOp{Process: proc, To: qty}
		if had {
			o := old
			op.From = &o
		}
		diff.Scale = append(diff.Scale, op)
	}
	return diff
}

// FormatDiffSummary is human-readable text matching legacy CLI diff style.
func FormatDiffSummary(pending FoldedPending, baseline *domain.Release) string {
	if pending.IsEmpty() {
		return "No pending changes\n"
	}
	diff := BuildDiff(pending, baseline)
	if diff.IsEmpty() {
		return "Staged changes match last release (no effective delta)\n"
	}
	var b strings.Builder
	if diff.Image != nil {
		b.WriteString("## Image\n")
		old := diff.Image.From
		if old == "" {
			old = "(none)"
		}
		fmt.Fprintf(&b, "  %s → %s\n", old, diff.Image.To)
	}
	if len(diff.Config) > 0 {
		b.WriteString("## Config\n")
		for _, c := range diff.Config {
			switch c.Op {
			case "remove":
				fmt.Fprintf(&b, "  - %s (was %s)\n", c.Key, displayConfigVal(strOr(c.From, "")))
			case "add":
				fmt.Fprintf(&b, "  + %s=%s\n", c.Key, displayConfigVal(strOr(c.To, "")))
			case "change":
				fmt.Fprintf(&b, "  ~ %s: %s → %s\n", c.Key, displayConfigVal(strOr(c.From, "")), displayConfigVal(strOr(c.To, "")))
			}
		}
	}
	if len(diff.Scale) > 0 {
		b.WriteString("## Scale\n")
		for _, s := range diff.Scale {
			if s.From == nil {
				fmt.Fprintf(&b, "  %s: (none) → %d\n", s.Process, s.To)
			} else {
				fmt.Fprintf(&b, "  %s: %d → %d\n", s.Process, *s.From, s.To)
			}
		}
	}
	return b.String()
}

func strOr(p *string, fallback string) string {
	if p == nil {
		return fallback
	}
	return *p
}

func displayConfigVal(v string) string {
	if v == domain.SecretSentinel {
		return "[secret]"
	}
	return v
}

// configDiffSensitivity: secret if either side is secret.
func configDiffSensitivity(baseline, pending string) string {
	if domain.IsSecret(baseline) || domain.IsSecret(pending) {
		return domain.SensitivitySecret
	}
	if pending != "" {
		return pending
	}
	if baseline != "" {
		return baseline
	}
	return domain.SensitivityPlain
}

// redactFoldedPending replaces secret values with the sentinel for API responses.
func redactFoldedPending(p FoldedPending) FoldedPending {
	if len(p.Config) == 0 {
		return p
	}
	out := FoldedPending{
		Image:             p.Image,
		Config:            make(map[string]*string, len(p.Config)),
		ConfigSensitivity: p.ConfigSensitivity,
		Scales:            p.Scales,
	}
	sens := p.ConfigSensitivity
	if sens == nil {
		sens = map[string]string{}
	}
	for k, v := range p.Config {
		if v == nil {
			out.Config[k] = nil
			continue
		}
		red := domain.RedactConfigValue(*v, sens[k])
		out.Config[k] = &red
	}
	return out
}

// PreviewPending returns structured preview of open changeset vs last deploy in env.
func (s *ChangesetService) PreviewPending(ctx context.Context, projectName, envName string) (*PreviewResult, error) {
	view, err := s.GetChangeset(ctx, projectName, envName)
	if err != nil {
		return nil, err
	}
	envLabel := envName
	if view.EnvironmentName != "" {
		envLabel = view.EnvironmentName
	}
	folded, err := FoldChanges(view.Changes)
	if err != nil {
		return nil, err
	}
	baseline, err := s.releaseService.GetLatestReleaseForEnvironment(ctx, projectName, envName)
	if err != nil {
		return nil, err
	}
	// Merge live resolved sensitivity so sticky secrets redact even without staged sensitivity.
	if project, svc, env, err := s.projectService.resolvePrimaryService(ctx, projectName, envName); err == nil {
		if _, liveSens, err := s.store.ResolveConfigWithSensitivity(ctx, project.ID, svc.ID, env.ID); err == nil {
			if folded.ConfigSensitivity == nil {
				folded.ConfigSensitivity = map[string]string{}
			}
			for k := range folded.Config {
				if folded.ConfigSensitivity[k] == "" {
					if s, ok := liveSens[k]; ok {
						folded.ConfigSensitivity[k] = s
					}
				}
			}
		}
	}
	diff := BuildDiff(folded, baseline)
	redacted := redactFoldedPending(folded)
	res := &PreviewResult{
		Mode:            "pending",
		Environment:     envLabel,
		HasPending:      !folded.IsEmpty(),
		MatchesBaseline: !folded.IsEmpty() && diff.IsEmpty(),
		Pending:         &redacted,
		Diff:            diff,
		Summary:         FormatDiffSummary(folded, baseline),
	}
	if baseline != nil {
		v := baseline.Version
		res.BaselineVersion = &v
	}
	if folded.IsEmpty() {
		res.Pending = nil
	}
	return res, nil
}

// PreviewReleases compares two release versions (service-scoped).
func (s *ChangesetService) PreviewReleases(ctx context.Context, projectName, envName string, fromV, toV int) (*PreviewResult, error) {
	if fromV < 1 || toV < 1 {
		return nil, fmt.Errorf("%w: from_release and to_release must be >= 1", launchpad.ErrBadRequest)
	}
	_, svc, _, err := s.projectService.resolvePrimaryService(ctx, projectName, envName)
	if err != nil {
		return nil, err
	}
	from, err := s.store.GetReleaseByVersion(ctx, svc.ID, fromV)
	if err != nil {
		return nil, err
	}
	to, err := s.store.GetReleaseByVersion(ctx, svc.ID, toV)
	if err != nil {
		return nil, err
	}
	// Treat "to" as folded pending against "from" baseline.
	pending := foldedFromRelease(to)
	diff := BuildDiff(pending, from)
	redacted := redactFoldedPending(pending)
	return &PreviewResult{
		Mode:            "releases",
		FromVersion:     &fromV,
		ToVersion:       &toV,
		HasPending:      true,
		MatchesBaseline: diff.IsEmpty(),
		Pending:         &redacted,
		Diff:            diff,
		Summary:         FormatDiffSummary(pending, from),
	}, nil
}

func foldedFromRelease(r *domain.Release) FoldedPending {
	out := FoldedPending{
		Config:            make(map[string]*string),
		ConfigSensitivity: make(map[string]string),
		Scales:            make(map[string]int),
	}
	if r == nil {
		return out
	}
	out.Image = r.ArtifactRef
	for k, v := range r.ConfigResolved {
		val := v
		out.Config[k] = &val
	}
	if r.ConfigSensitivity != nil {
		for k, s := range r.ConfigSensitivity {
			out.ConfigSensitivity[k] = s
		}
	}
	for name, snap := range r.ProcessSnapshot {
		out.Scales[name] = snap.Quantity
	}
	return out
}
