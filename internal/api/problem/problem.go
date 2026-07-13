package problem

import (
	"encoding/json"
	"net/http"
)

// Hint is a machine- and human-oriented recovery suggestion (RFC 7807 extension).
type Hint struct {
	Action  string `json:"action"`
	Message string `json:"message"`
	Command string `json:"command,omitempty"`
}

// Detail is an RFC 7807 problem document with optional Launchpad extensions.
type Detail struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail"`
	Instance string `json:"instance,omitempty"`
	Code     string `json:"code,omitempty"`
	Hints    []Hint `json:"hints,omitempty"`
}

// Write encodes a basic problem response (no code/hints).
func Write(w http.ResponseWriter, status int, title, detail, instance string) {
	WriteDetail(w, Detail{
		Type:     "https://launchpad.dev/errors/" + http.StatusText(status),
		Title:    title,
		Status:   status,
		Detail:   detail,
		Instance: instance,
	})
}

// WriteDetail encodes a full problem document.
func WriteDetail(w http.ResponseWriter, d Detail) {
	if d.Type == "" {
		d.Type = "https://launchpad.dev/errors/" + http.StatusText(d.Status)
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(d.Status)
	_ = json.NewEncoder(w).Encode(d)
}

func BadRequest(w http.ResponseWriter, detail string) {
	writeWithHints(w, http.StatusBadRequest, "Bad Request", detail)
}

func NotFound(w http.ResponseWriter, detail string) {
	writeWithHints(w, http.StatusNotFound, "Not Found", detail)
}

func Conflict(w http.ResponseWriter, detail string) {
	writeWithHints(w, http.StatusConflict, "Conflict", detail)
}

func Internal(w http.ResponseWriter, detail string) {
	Write(w, http.StatusInternalServerError, "Internal Server Error", detail, "")
}

func NotImplemented(w http.ResponseWriter, detail string) {
	Write(w, http.StatusNotImplemented, "Not Implemented", detail, "")
}

// writeWithHints attaches catalog code/hints based on the detail string alone
// when callers do not pass the original error (legacy helpers).
func writeWithHints(w http.ResponseWriter, status int, title, detail string) {
	code, hints := hintsForDetail(detail)
	WriteDetail(w, Detail{
		Type:   "https://launchpad.dev/errors/" + http.StatusText(status),
		Title:  title,
		Status: status,
		Detail: detail,
		Code:   code,
		Hints:  hints,
	})
}

// WriteError maps a Go error to problem+json with recovery catalog.
func WriteError(w http.ResponseWriter, err error) {
	status, title := statusTitle(err)
	code, hints := HintsFor(err)
	WriteDetail(w, Detail{
		Type:   "https://launchpad.dev/errors/" + http.StatusText(status),
		Title:  title,
		Status: status,
		Detail: err.Error(),
		Code:   code,
		Hints:  hints,
	})
}
