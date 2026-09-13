package service

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
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
	// Processes is API overlay only (pending sparse / release full to-map). Not BuildDiff input.
	Processes map[string]*domain.ProcessSnapshot `json:"processes,omitempty"`

	processApply  *domain.ProcessApplyPayload
	processSets   []domain.ProcessSetPayload
	processUnsets []string
}

func (f FoldedPending) hasProcessOps() bool {
	return f.processApply != nil || len(f.processSets) > 0 || len(f.processUnsets) > 0
}

func (f FoldedPending) IsEmpty() bool {
	return f.Image == "" && len(f.Config) == 0 && len(f.Scales) == 0 && !f.hasProcessOps()
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

// ProcessDiffOp is one effective process-definition delta.
type ProcessDiffOp struct {
	Op     string                  `json:"op"` // add | change | remove
	Name   string                  `json:"name"`
	From   *domain.ProcessSnapshot `json:"from,omitempty"`
	To     *domain.ProcessSnapshot `json:"to,omitempty"`
	Fields []string                `json:"fields,omitempty"`
}

// EffectiveDiff is structured delta of pending (or target release) vs baseline.
type EffectiveDiff struct {
	Image   *ImageDiff      `json:"image,omitempty"`
	Config  []ConfigDiffOp  `json:"config,omitempty"`
	Scale   []ScaleDiffOp   `json:"scale,omitempty"`
	Process []ProcessDiffOp `json:"process,omitempty"`
}

func (d EffectiveDiff) IsEmpty() bool {
	return d.Image == nil && len(d.Config) == 0 && len(d.Scale) == 0 && len(d.Process) == 0
}

// PreviewResult is the API shape for server-side diff preview.
type PreviewResult struct {
	Mode            string         `json:"mode"` // pending | releases | environments
	Environment     string         `json:"environment,omitempty"`
	FromEnvironment string         `json:"from_environment,omitempty"`
	ToEnvironment   string         `json:"to_environment,omitempty"`
	BaselineVersion *int           `json:"baseline_version,omitempty"`
	FromVersion     *int           `json:"from_version,omitempty"`
	ToVersion       *int           `json:"to_version,omitempty"`
	HasPending      bool           `json:"has_pending"`
	MatchesBaseline bool           `json:"matches_baseline"`
	Pending         *FoldedPending `json:"pending,omitempty"`
	Diff            EffectiveDiff  `json:"diff"`
	Summary         string         `json:"summary"`
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
		case domain.ChangeTypeProcessSet:
			var p domain.ProcessSetPayload
			if err := json.Unmarshal(c.Payload, &p); err != nil {
				return FoldedPending{}, fmt.Errorf("%w: process.set payload", launchpad.ErrBadRequest)
			}
			if p.Name == "" {
				return FoldedPending{}, fmt.Errorf("%w: process.set missing name", launchpad.ErrBadRequest)
			}
			out.processSets = append(out.processSets, p)
		case domain.ChangeTypeProcessUnset:
			var p domain.ProcessUnsetPayload
			if err := json.Unmarshal(c.Payload, &p); err != nil {
				return FoldedPending{}, fmt.Errorf("%w: process.unset payload", launchpad.ErrBadRequest)
			}
			if p.Name == "" {
				return FoldedPending{}, fmt.Errorf("%w: process.unset missing name", launchpad.ErrBadRequest)
			}
			out.processUnsets = append(out.processUnsets, p.Name)
		case domain.ChangeTypeProcessApply:
			var p domain.ProcessApplyPayload
			if err := json.Unmarshal(c.Payload, &p); err != nil {
				return FoldedPending{}, fmt.Errorf("%w: process.apply payload", launchpad.ErrBadRequest)
			}
			if p.Procfile == "" {
				return FoldedPending{}, fmt.Errorf("%w: process.apply missing procfile", launchpad.ErrBadRequest)
			}
			if _, err := domain.ParseProcfile(p.Procfile); err != nil {
				return FoldedPending{}, fmt.Errorf("%w: %v", launchpad.ErrBadRequest, err)
			}
			cp := p
			out.processApply = &cp
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

// BuildSnapshotDiff compares two full release snapshots (union of config keys and processes).
// Nil releases are treated as empty snapshots. Secret values are redacted.
func BuildSnapshotDiff(from, to *domain.Release) EffectiveDiff {
	fromImage, fromConfig, fromSens, fromScales := releaseSnapshotParts(from)
	toImage, toConfig, toSens, toScales := releaseSnapshotParts(to)

	var diff EffectiveDiff
	if fromImage != toImage {
		diff.Image = &ImageDiff{From: fromImage, To: toImage}
	}

	keySet := map[string]struct{}{}
	for k := range fromConfig {
		keySet[k] = struct{}{}
	}
	for k := range toConfig {
		keySet[k] = struct{}{}
	}
	var keys []string
	for k := range keySet {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		old, hadFrom := fromConfig[key]
		newVal, hadTo := toConfig[key]
		sens := configDiffSensitivity(fromSens[key], toSens[key])
		switch {
		case hadFrom && !hadTo:
			from := domain.RedactConfigValue(old, sens)
			diff.Config = append(diff.Config, ConfigDiffOp{Op: "remove", Key: key, From: &from, Sensitivity: sens})
		case !hadFrom && hadTo:
			to := domain.RedactConfigValue(newVal, sens)
			diff.Config = append(diff.Config, ConfigDiffOp{Op: "add", Key: key, To: &to, Sensitivity: sens})
		case hadFrom && hadTo && old != newVal:
			from := domain.RedactConfigValue(old, sens)
			to := domain.RedactConfigValue(newVal, sens)
			diff.Config = append(diff.Config, ConfigDiffOp{Op: "change", Key: key, From: &from, To: &to, Sensitivity: sens})
		}
	}

	procSet := map[string]struct{}{}
	for p := range fromScales {
		procSet[p] = struct{}{}
	}
	for p := range toScales {
		procSet[p] = struct{}{}
	}
	var procs []string
	for p := range procSet {
		procs = append(procs, p)
	}
	sort.Strings(procs)
	for _, proc := range procs {
		old, hadFrom := fromScales[proc]
		qty, hadTo := toScales[proc]
		switch {
		case hadFrom && !hadTo:
			o := old
			diff.Scale = append(diff.Scale, ScaleDiffOp{Process: proc, From: &o, To: 0})
		case !hadFrom && hadTo:
			diff.Scale = append(diff.Scale, ScaleDiffOp{Process: proc, To: qty})
		case hadFrom && hadTo && old != qty:
			o := old
			diff.Scale = append(diff.Scale, ScaleDiffOp{Process: proc, From: &o, To: qty})
		}
	}

	var fromSnap, toSnap map[string]domain.ProcessSnapshot
	if from != nil {
		fromSnap = from.ProcessSnapshot
	}
	if to != nil {
		toSnap = to.ProcessSnapshot
	}
	scaleNames := make(map[string]bool, len(diff.Scale))
	for _, s := range diff.Scale {
		scaleNames[s.Process] = true
	}
	diff.Process = diffProcessSnapshots(fromSnap, toSnap, scaleNames)
	return diff
}

func releaseSnapshotParts(r *domain.Release) (image string, config map[string]string, sens map[string]string, scales map[string]int) {
	config = map[string]string{}
	sens = map[string]string{}
	scales = map[string]int{}
	if r == nil {
		return "", config, sens, scales
	}
	image = r.ArtifactRef
	if r.ConfigResolved != nil {
		config = r.ConfigResolved
	}
	if r.ConfigSensitivity != nil {
		sens = r.ConfigSensitivity
	}
	for name, snap := range r.ProcessSnapshot {
		scales[name] = snap.Quantity
	}
	return image, config, sens, scales
}

// FormatSnapshotDiffSummary formats a full snapshot EffectiveDiff (env↔env / full release compare).
func FormatSnapshotDiffSummary(diff EffectiveDiff) string {
	if diff.IsEmpty() {
		return "No differences\n"
	}
	return formatEffectiveDiff(diff)
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
	return formatEffectiveDiff(diff)
}

func formatEffectiveDiff(diff EffectiveDiff) string {
	var b strings.Builder
	if diff.Image != nil {
		b.WriteString("## Image\n")
		old := diff.Image.From
		if old == "" {
			old = "(none)"
		}
		to := diff.Image.To
		if to == "" {
			to = "(none)"
		}
		fmt.Fprintf(&b, "  %s → %s\n", old, to)
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
	if len(diff.Process) > 0 {
		b.WriteString("## Process\n")
		for _, p := range diff.Process {
			switch p.Op {
			case "add":
				to := domain.ProcessSnapshot{}
				if p.To != nil {
					to = *p.To
				}
				fmt.Fprintf(&b, "  + %s command=%q quantity=%d expose=%s\n", p.Name, to.Command, to.Quantity, to.Expose)
			case "remove":
				fmt.Fprintf(&b, "  - %s\n", p.Name)
			case "change":
				from, to := domain.ProcessSnapshot{}, domain.ProcessSnapshot{}
				if p.From != nil {
					from = *p.From
				}
				if p.To != nil {
					to = *p.To
				}
				for _, field := range p.Fields {
					fmt.Fprintf(&b, "  ~ %s %s: %s → %s\n", p.Name, field, processFieldDisplay(from, field), processFieldDisplay(to, field))
				}
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
	out := FoldedPending{
		Image:             p.Image,
		Config:            p.Config,
		ConfigSensitivity: p.ConfigSensitivity,
		Scales:            p.Scales,
		Processes:         p.Processes,
	}
	if len(p.Config) == 0 {
		return out
	}
	out.Config = make(map[string]*string, len(p.Config))
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
	var baselineSnap map[string]domain.ProcessSnapshot
	if baseline != nil {
		baselineSnap = baseline.ProcessSnapshot
	}
	effective := applyProcessOpsToSnapshot(baselineSnap, folded)
	diff := BuildDiff(folded, baseline)
	diff.Process = diffProcessSnapshots(baselineSnap, effective, scaleNameSet(folded.Scales))
	folded.Processes = sparseProcessOverlay(folded, baselineSnap, effective)
	redacted := redactFoldedPending(folded)
	summary := formatEffectiveDiff(diff)
	if folded.IsEmpty() {
		summary = "No pending changes\n"
	} else if diff.IsEmpty() {
		summary = "Staged changes match last release (no effective delta)\n"
	}
	res := &PreviewResult{
		Mode:            "pending",
		Environment:     envLabel,
		HasPending:      !folded.IsEmpty(),
		MatchesBaseline: !folded.IsEmpty() && diff.IsEmpty(),
		Pending:         &redacted,
		Diff:            diff,
		Summary:         summary,
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
	var fromSnap, toSnap map[string]domain.ProcessSnapshot
	if from != nil {
		fromSnap = from.ProcessSnapshot
	}
	if to != nil {
		toSnap = to.ProcessSnapshot
	}
	diff.Process = diffProcessSnapshots(fromSnap, toSnap, scaleNameSet(pending.Scales))
	redacted := redactFoldedPending(pending)
	return &PreviewResult{
		Mode:            "releases",
		FromVersion:     &fromV,
		ToVersion:       &toV,
		HasPending:      true,
		MatchesBaseline: diff.IsEmpty(),
		Pending:         &redacted,
		Diff:            diff,
		Summary:         formatEffectiveDiff(diff),
	}, nil
}

// PreviewEnvironments compares the latest deployed release in fromEnv vs toEnv.
func (s *ChangesetService) PreviewEnvironments(ctx context.Context, projectName, fromEnv, toEnv string) (*PreviewResult, error) {
	fromEnv = strings.TrimSpace(fromEnv)
	toEnv = strings.TrimSpace(toEnv)
	if fromEnv == "" || toEnv == "" {
		return nil, fmt.Errorf("%w: from_env and to_env are required", launchpad.ErrBadRequest)
	}
	if fromEnv == toEnv {
		return nil, fmt.Errorf("%w: from_env and to_env must differ", launchpad.ErrBadRequest)
	}
	// Ensure both environments exist (404 if not).
	if _, err := s.projectService.GetEnvironment(ctx, projectName, fromEnv); err != nil {
		return nil, err
	}
	if _, err := s.projectService.GetEnvironment(ctx, projectName, toEnv); err != nil {
		return nil, err
	}
	fromRel, err := s.releaseService.GetLatestReleaseForEnvironment(ctx, projectName, fromEnv)
	if err != nil {
		return nil, err
	}
	toRel, err := s.releaseService.GetLatestReleaseForEnvironment(ctx, projectName, toEnv)
	if err != nil {
		return nil, err
	}
	diff := BuildSnapshotDiff(fromRel, toRel)
	res := &PreviewResult{
		Mode:            "environments",
		FromEnvironment: fromEnv,
		ToEnvironment:   toEnv,
		HasPending:      false,
		MatchesBaseline: diff.IsEmpty(),
		Diff:            diff,
		Summary:         FormatSnapshotDiffSummary(diff),
	}
	if fromRel != nil {
		v := fromRel.Version
		res.FromVersion = &v
	}
	if toRel != nil {
		v := toRel.Version
		res.ToVersion = &v
	}
	return res, nil
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
	if len(r.ProcessSnapshot) > 0 {
		out.Processes = make(map[string]*domain.ProcessSnapshot, len(r.ProcessSnapshot))
	}
	for name, snap := range r.ProcessSnapshot {
		out.Scales[name] = snap.Quantity
		s := snap
		out.Processes[name] = &s
	}
	return out
}

func scaleNameSet(scales map[string]int) map[string]bool {
	out := make(map[string]bool, len(scales))
	for name := range scales {
		out[name] = true
	}
	return out
}

func applyProcessOpsToSnapshot(baseline map[string]domain.ProcessSnapshot, folded FoldedPending) map[string]domain.ProcessSnapshot {
	cur := copyProcessSnapshotMap(baseline)
	if folded.processApply != nil {
		if entries, err := domain.ParseProcfile(folded.processApply.Procfile); err == nil {
			cur = make(map[string]domain.ProcessSnapshot, len(entries))
			for _, e := range entries {
				cur[e.Name] = domain.ProcessSnapshot{
					Command:  e.Command,
					Quantity: e.Quantity,
					Expose:   e.Expose,
				}
			}
		}
	}
	for _, set := range folded.processSets {
		mergeProcessSet(cur, set)
	}
	for _, name := range folded.processUnsets {
		delete(cur, name)
	}
	for name, qty := range folded.Scales {
		if snap, ok := cur[name]; ok {
			snap.Quantity = qty
			cur[name] = snap
		}
	}
	return cur
}

func copyProcessSnapshotMap(in map[string]domain.ProcessSnapshot) map[string]domain.ProcessSnapshot {
	out := make(map[string]domain.ProcessSnapshot, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mergeProcessSet(cur map[string]domain.ProcessSnapshot, set domain.ProcessSetPayload) {
	snap, ok := cur[set.Name]
	if !ok {
		snap = domain.ProcessSnapshot{Quantity: 1, Expose: "none"}
		if set.Name == "web" {
			snap.Expose = "http"
		}
	}
	if set.Command != nil {
		snap.Command = *set.Command
	}
	if set.Quantity != nil {
		snap.Quantity = *set.Quantity
	}
	if set.Expose != nil {
		snap.Expose = *set.Expose
	}
	if set.Health != nil {
		if set.Health.Type == "none" || set.Health.Type == "" {
			snap.Health = nil
		} else {
			h := *set.Health
			if h.Type == "http" && h.Path == "" {
				h.Path = "/healthz"
			}
			snap.Health = &h
		}
	}
	if set.TargetExtensions != nil {
		snap.TargetExtensions = set.TargetExtensions
	}
	cur[set.Name] = snap
}

func sparseProcessOverlay(folded FoldedPending, baseline, effective map[string]domain.ProcessSnapshot) map[string]*domain.ProcessSnapshot {
	if !folded.hasProcessOps() {
		return nil
	}
	keys := map[string]struct{}{}
	for _, set := range folded.processSets {
		keys[set.Name] = struct{}{}
	}
	for _, name := range folded.processUnsets {
		keys[name] = struct{}{}
	}
	if folded.processApply != nil {
		for name := range effective {
			keys[name] = struct{}{}
		}
		for name := range baseline {
			if _, ok := effective[name]; !ok {
				keys[name] = struct{}{}
			}
		}
	}
	if len(keys) == 0 {
		return nil
	}
	out := make(map[string]*domain.ProcessSnapshot, len(keys))
	for name := range keys {
		if snap, ok := effective[name]; ok {
			s := snap
			out[name] = &s
		} else {
			out[name] = nil
		}
	}
	return out
}

func diffProcessSnapshots(from, to map[string]domain.ProcessSnapshot, scaleNames map[string]bool) []ProcessDiffOp {
	names := map[string]struct{}{}
	for name := range from {
		names[name] = struct{}{}
	}
	for name := range to {
		names[name] = struct{}{}
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)

	var ops []ProcessDiffOp
	for _, name := range ordered {
		f, hadFrom := from[name]
		t, hadTo := to[name]
		switch {
		case !hadFrom && hadTo:
			toSnap := t
			ops = append(ops, ProcessDiffOp{Op: "add", Name: name, To: &toSnap})
		case hadFrom && !hadTo:
			fromSnap := f
			ops = append(ops, ProcessDiffOp{Op: "remove", Name: name, From: &fromSnap})
		case hadFrom && hadTo:
			fields := processSnapshotDiffFields(f, t)
			if len(fields) == 0 {
				continue
			}
			if len(fields) == 1 && fields[0] == "quantity" && scaleNames[name] {
				continue
			}
			fromSnap, toSnap := f, t
			ops = append(ops, ProcessDiffOp{Op: "change", Name: name, From: &fromSnap, To: &toSnap, Fields: fields})
		}
	}
	return ops
}

func processSnapshotDiffFields(a, b domain.ProcessSnapshot) []string {
	var fields []string
	if a.Command != b.Command {
		fields = append(fields, "command")
	}
	if a.Quantity != b.Quantity {
		fields = append(fields, "quantity")
	}
	if a.Expose != b.Expose {
		fields = append(fields, "expose")
	}
	if !healthEqual(a.Health, b.Health) {
		fields = append(fields, "health")
	}
	if !processExtensionsEqual(a.TargetExtensions, b.TargetExtensions) {
		fields = append(fields, "target_extensions")
	}
	return fields
}

func healthIsNone(h *domain.ProcessHealth) bool {
	return h == nil || h.Type == "" || h.Type == "none"
}

func processExtensionsEqual(a, b map[string]json.RawMessage) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	toAny := func(m map[string]json.RawMessage) (map[string]any, bool) {
		out := make(map[string]any, len(m))
		for k, raw := range m {
			if len(raw) == 0 {
				out[k] = nil
				continue
			}
			var v any
			if err := json.Unmarshal(raw, &v); err != nil {
				return nil, false
			}
			out[k] = v
		}
		return out, true
	}
	ma, ok1 := toAny(a)
	mb, ok2 := toAny(b)
	if !ok1 || !ok2 {
		return false
	}
	return reflect.DeepEqual(ma, mb)
}

func processFieldDisplay(snap domain.ProcessSnapshot, field string) string {
	switch field {
	case "command":
		return fmt.Sprintf("%q", snap.Command)
	case "quantity":
		return fmt.Sprintf("%d", snap.Quantity)
	case "expose":
		return snap.Expose
	case "health":
		return formatHealthValue(snap.Health)
	case "target_extensions":
		return formatExtensionsValue(snap.TargetExtensions)
	default:
		return ""
	}
}

func formatHealthValue(h *domain.ProcessHealth) string {
	if healthIsNone(h) {
		return "(none)"
	}
	if h.Type == "http" {
		if h.Path != "" {
			return "http " + h.Path
		}
		return "http"
	}
	return h.Type
}

func formatExtensionsValue(m map[string]json.RawMessage) string {
	if len(m) == 0 {
		return "{}"
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}
