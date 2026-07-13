package apiclient

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestParseAPIErrorPreservesHints(t *testing.T) {
	body, _ := json.Marshal(map[string]any{
		"type":   "https://launchpad.dev/errors/Conflict",
		"title":  "Conflict",
		"status": 409,
		"detail": "conflict: deployment already in progress",
		"code":   "deployment_in_progress",
		"hints": []map[string]string{
			{"action": "wait", "message": "wait", "command": "launchpad deploy --wait"},
		},
	})
	err := parseAPIError("POST", "/v1/projects/x/promote", 409, body)
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want *APIError, got %T %v", err, err)
	}
	if ae.Code != "deployment_in_progress" {
		t.Fatalf("code: %s", ae.Code)
	}
	if len(ae.Hints) != 1 || ae.Hints[0].Command != "launchpad deploy --wait" {
		t.Fatalf("hints: %+v", ae.Hints)
	}
	if msg := ae.Error(); msg == "" || !contains(msg, "try: launchpad deploy --wait") {
		t.Fatalf("Error(): %s", msg)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
