package mcp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/launchpad/launchpad/pkg/apiclient"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// jsonAny re-encodes v so json.RawMessage fields become objects, not byte arrays.
func jsonAny(v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func jsonResult(ctx context.Context, cl *apiclient.Client, project string, v any, err error) (*mcpsdk.CallToolResult, any, error) {
	if err != nil {
		return nil, nil, wrapErr(err)
	}
	out, jerr := jsonAny(v)
	if jerr != nil {
		return nil, nil, wrapErr(jerr)
	}
	return nil, redactSecrets(out, liveSecretKeys(ctx, cl, project)), nil
}

func errMissingToken() error {
	b, _ := json.Marshal(map[string]any{
		"detail": "LAUNCHPAD_TOKEN is not set",
		"hints": []map[string]string{{
			"action":  "set_token",
			"message": "Export a workspace token (same env var as the CLI).",
			"command": "export LAUNCHPAD_TOKEN=lp_...",
		}},
	})
	return errors.New(string(b))
}

func errJSON(detail string, extra map[string]any) error {
	payload := map[string]any{"detail": detail}
	for k, v := range extra {
		payload[k] = v
	}
	b, _ := json.Marshal(payload)
	return errors.New(string(b))
}

// FormatToolError maps API and local errors to MCP isError text JSON.
func FormatToolError(err error) string {
	if err == nil {
		return `{"detail":"unknown error"}`
	}
	var ae *apiclient.APIError
	if errors.As(err, &ae) && ae != nil {
		detail := ae.Detail
		if detail == "" {
			detail = ae.Error()
		}
		payload := map[string]any{
			"status": ae.Status,
			"code":   ae.Code,
			"title":  ae.Title,
			"detail": detail,
		}
		if len(ae.Hints) > 0 {
			payload["hints"] = ae.Hints
		}
		b, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return `{"detail":"api error"}`
		}
		return string(b)
	}
	msg := err.Error()
	if len(msg) > 0 && msg[0] == '{' {
		return msg
	}
	b, _ := json.Marshal(map[string]any{"detail": msg})
	return string(b)
}

func wrapErr(err error) error {
	if err == nil {
		return nil
	}
	var ae *apiclient.APIError
	if errors.As(err, &ae) {
		return errors.New(FormatToolError(err))
	}
	if msg := err.Error(); len(msg) > 0 && msg[0] == '{' {
		return err
	}
	return errors.New(FormatToolError(err))
}
