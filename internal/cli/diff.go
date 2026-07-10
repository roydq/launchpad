package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/launchpad/launchpad/pkg/apiclient"
)

// FoldedPending is the effective staged batch after last-write-wins folding.
type FoldedPending struct {
	Image  string
	Config map[string]*string // nil value means delete
	Scales map[string]int
}

func foldChanges(changes []apiclient.ChangesetChange) (FoldedPending, error) {
	out := FoldedPending{
		Config: make(map[string]*string),
		Scales: make(map[string]int),
	}
	for _, c := range changes {
		switch c.Type {
		case "config":
			var p struct {
				Key   string  `json:"key"`
				Value *string `json:"value"`
			}
			if err := json.Unmarshal(c.Payload, &p); err != nil {
				return FoldedPending{}, fmt.Errorf("config payload: %w", err)
			}
			if p.Key == "" {
				return FoldedPending{}, fmt.Errorf("config change missing key")
			}
			out.Config[p.Key] = p.Value
		case "scale":
			var p struct {
				Process  string `json:"process"`
				Quantity int    `json:"quantity"`
			}
			if err := json.Unmarshal(c.Payload, &p); err != nil {
				return FoldedPending{}, fmt.Errorf("scale payload: %w", err)
			}
			if p.Process == "" {
				return FoldedPending{}, fmt.Errorf("scale change missing process")
			}
			out.Scales[p.Process] = p.Quantity
		case "image":
			var p struct {
				ArtifactRef string `json:"artifact_ref"`
			}
			if err := json.Unmarshal(c.Payload, &p); err != nil {
				return FoldedPending{}, fmt.Errorf("image payload: %w", err)
			}
			if p.ArtifactRef == "" {
				return FoldedPending{}, fmt.Errorf("image change missing artifact_ref")
			}
			out.Image = p.ArtifactRef
		default:
			return FoldedPending{}, fmt.Errorf("unknown change type %q", c.Type)
		}
	}
	return out, nil
}

func (f FoldedPending) isEmpty() bool {
	return f.Image == "" && len(f.Config) == 0 && len(f.Scales) == 0
}

func formatDiff(pending FoldedPending, baseline *apiclient.Release) string {
	if pending.isEmpty() {
		return "No pending changes\n"
	}

	var baselineImage string
	baselineConfig := map[string]string{}
	baselineScales := map[string]int{}
	if baseline != nil {
		baselineImage = baseline.ArtifactRef
		if baseline.ConfigResolved != nil {
			baselineConfig = baseline.ConfigResolved
		}
		for name, snap := range baseline.ProcessSnapshot {
			baselineScales[name] = snap.Quantity
		}
	}

	var b strings.Builder
	wrote := false

	if pending.Image != "" && pending.Image != baselineImage {
		wrote = true
		b.WriteString("## Image\n")
		old := baselineImage
		if old == "" {
			old = "(none)"
		}
		fmt.Fprintf(&b, "  %s → %s\n", old, pending.Image)
	}

	type cfgLine struct {
		sortKey string
		line    string
	}
	var cfgLines []cfgLine
	for key, val := range pending.Config {
		old, had := baselineConfig[key]
		if val == nil {
			if !had {
				continue // delete of missing key — no effective delta
			}
			cfgLines = append(cfgLines, cfgLine{key, fmt.Sprintf("  - %s (was %s)\n", key, old)})
			continue
		}
		if !had {
			cfgLines = append(cfgLines, cfgLine{key, fmt.Sprintf("  + %s=%s\n", key, *val)})
			continue
		}
		if old == *val {
			continue
		}
		cfgLines = append(cfgLines, cfgLine{key, fmt.Sprintf("  ~ %s: %s → %s\n", key, old, *val)})
	}
	if len(cfgLines) > 0 {
		wrote = true
		sort.Slice(cfgLines, func(i, j int) bool { return cfgLines[i].sortKey < cfgLines[j].sortKey })
		b.WriteString("## Config\n")
		for _, l := range cfgLines {
			b.WriteString(l.line)
		}
	}

	type scaleLine struct {
		sortKey string
		line    string
	}
	var scaleLines []scaleLine
	for proc, qty := range pending.Scales {
		old, had := baselineScales[proc]
		if had && old == qty {
			continue
		}
		if !had {
			scaleLines = append(scaleLines, scaleLine{proc, fmt.Sprintf("  %s: (none) → %d\n", proc, qty)})
			continue
		}
		scaleLines = append(scaleLines, scaleLine{proc, fmt.Sprintf("  %s: %d → %d\n", proc, old, qty)})
	}
	if len(scaleLines) > 0 {
		wrote = true
		sort.Slice(scaleLines, func(i, j int) bool { return scaleLines[i].sortKey < scaleLines[j].sortKey })
		b.WriteString("## Scale\n")
		for _, l := range scaleLines {
			b.WriteString(l.line)
		}
	}

	if !wrote {
		return "Staged changes match last release (no effective delta)\n"
	}
	return b.String()
}
