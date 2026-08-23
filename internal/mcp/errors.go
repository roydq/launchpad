package mcp

import (
	"encoding/json"
	"errors"

	"github.com/launchpad/launchpad/pkg/apiclient"
)

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
