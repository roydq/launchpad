package mcp

import (
	"context"

	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/pkg/apiclient"
)

func liveSecretKeys(ctx context.Context, cl *apiclient.Client, project string) (map[string]bool, error) {
	if cl == nil || project == "" {
		return nil, errJSON("cannot load live config to redact secrets", nil)
	}
	cfg, err := cl.GetConfig(ctx, project)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for k, v := range cfg {
		if v == domain.SecretSentinel {
			out[k] = true
		}
	}
	return out, nil
}

// redactSecrets walks MCP JSON output and replaces secret config values with the
// API sentinel. GET /changeset still stores plaintext payloads; MCP must not
// echo them into agent transcripts. liveSecrets are keys whose live config is
// already secret (sticky). lookupFailed is fail-closed: omitted sensitivity
// is treated as secret so a config GET error cannot leak sticky rotates.
func redactSecrets(v any, liveSecrets map[string]bool, lookupFailed bool) any {
	if liveSecrets == nil {
		liveSecrets = map[string]bool{}
	}
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = redactSecrets(val, liveSecrets, lookupFailed)
		}
		if changes, ok := out["changes"].([]any); ok {
			redacted := make([]any, len(changes))
			for i, c := range changes {
				redacted[i] = redactChange(c, liveSecrets, lookupFailed)
			}
			out["changes"] = redacted
		}
		if ch, ok := out["change"]; ok {
			out["change"] = redactChange(ch, liveSecrets, lookupFailed)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = redactSecrets(e, liveSecrets, lookupFailed)
		}
		return out
	default:
		return v
	}
}

func redactChange(c any, liveSecrets map[string]bool, lookupFailed bool) any {
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
	if sens == domain.SensitivityPlain {
		return out
	}
	key, _ := payload["key"].(string)
	sticky := key != "" && liveSecrets[key]
	if domain.IsSecret(sens) || lookupFailed || sticky {
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

func changesetPin(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	s, _ := m["environment"].(string)
	return s
}

func clientForPin(cl *apiclient.Client, pin string) *apiclient.Client {
	if cl == nil || pin == "" {
		return cl
	}
	cp := *cl
	cp.Environment = pin
	return &cp
}

func redactWithLive(ctx context.Context, cl *apiclient.Client, project string, v any) any {
	lookup := clientForPin(cl, changesetPin(v))
	secrets, err := liveSecretKeys(ctx, lookup, project)
	return redactSecrets(v, secrets, err != nil)
}
