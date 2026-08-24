package apiclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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

func TestGetLogsFourOhFourIsAPIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"title": "Not Found", "detail": "no logs", "code": "not_found",
		})
	}))
	t.Cleanup(ts.Close)
	cl := New(ts.URL, "tok")
	_, err := cl.GetLogs(context.Background(), "p", "web")
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want *APIError, got %T %v", err, err)
	}
	if ae.Code != "not_found" || ae.Status != http.StatusNotFound {
		t.Fatalf("got status=%d code=%s", ae.Status, ae.Code)
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
