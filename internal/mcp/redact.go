package mcp

import (
	"context"

	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/pkg/apiclient"
)

func liveSecretKeys(ctx context.Context, cl *apiclient.Client, project string) map[string]bool {
	out := map[string]bool{}
	if cl == nil || project == "" {
		return out
	}
	for _, layer := range []string{"", "shared", "service"} {
		cfg, err := cl.GetConfigLayer(ctx, project, layer)
		if err != nil {
			continue
		}
		for k, v := range cfg {
			if v == domain.SecretSentinel {
				out[k] = true
			}
		}
	}
	return out
}

// redactSecrets walks MCP JSON output and replaces secret config values with the
// API sentinel. GET /changeset still stores plaintext payloads; MCP must not
// echo them into agent transcripts. liveSecrets are keys whose live config is
// already secret (sticky) even when the staged payload omits sensitivity.
func redactSecrets(v any, liveSecrets map[string]bool) any {
	if liveSecrets == nil {
		liveSecrets = map[string]bool{}
	}
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = redactSecrets(val, liveSecrets)
		}
		if changes, ok := out["changes"].([]any); ok {
			redacted := make([]any, len(changes))
			for i, c := range changes {
				redacted[i] = redactChange(c, liveSecrets)
			}
			out["changes"] = redacted
		}
		if ch, ok := out["change"]; ok {
			out["change"] = redactChange(ch, liveSecrets)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = redactSecrets(e, liveSecrets)
		}
		return out
	default:
		return v
	}
}

func redactChange(c any, liveSecrets map[string]bool) any {
	m, ok := c.(map[string]any)
	if !ok {
		return c
	}
	out := make(map[string]any, len(m))
	for k, val := range m {
		out[k] = val
	}
	typ, _ := out["type"].(string)
	if typ != "config" && typ != "shared_config" {
		return out
	}
	payload, ok := out["payload"].(map[string]any)
	if !ok {
		return out
	}
	sens, _ := payload["sensitivity"].(string)
	key, _ := payload["key"].(string)
	sticky := key != "" && liveSecrets[key]
	if domain.IsSecret(sens) || (sens == "" && sticky) {
		cp := make(map[string]any, len(payload))
		for k, val := range payload {
			cp[k] = val
		}
		if _, has := cp["value"]; has && cp["value"] != nil {
			cp["value"] = domain.SecretSentinel
		}
		out["payload"] = cp
	}
	return out
}
