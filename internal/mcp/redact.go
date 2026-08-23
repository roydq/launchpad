package mcp

import (
	"github.com/launchpad/launchpad/internal/domain"
)

// redactSecrets walks MCP JSON output and replaces secret config values with the
// API sentinel. GET /changeset still stores plaintext payloads; MCP must not
// echo them into agent transcripts.
func redactSecrets(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = redactSecrets(val)
		}
		if changes, ok := out["changes"].([]any); ok {
			redacted := make([]any, len(changes))
			for i, c := range changes {
				redacted[i] = redactChange(c)
			}
			out["changes"] = redacted
		}
		if ch, ok := out["change"]; ok {
			out["change"] = redactChange(ch)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = redactSecrets(e)
		}
		return out
	default:
		return v
	}
}

func redactChange(c any) any {
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
	if !domain.IsSecret(sens) {
		return out
	}
	cp := make(map[string]any, len(payload))
	for k, val := range payload {
		cp[k] = val
	}
	if _, has := cp["value"]; has && cp["value"] != nil {
		cp["value"] = domain.SecretSentinel
	}
	out["payload"] = cp
	return out
}
