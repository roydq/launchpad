package problem

import (
	"encoding/json"
	"net/http"
)

type Detail struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
	Instance string `json:"instance,omitempty"`
}

func Write(w http.ResponseWriter, status int, title, detail, instance string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Detail{
		Type:     "https://launchpad.dev/errors/" + http.StatusText(status),
		Title:    title,
		Status:   status,
		Detail:   detail,
		Instance: instance,
	})
}

func BadRequest(w http.ResponseWriter, detail string) {
	Write(w, http.StatusBadRequest, "Bad Request", detail, "")
}

func NotFound(w http.ResponseWriter, detail string) {
	Write(w, http.StatusNotFound, "Not Found", detail, "")
}

func Conflict(w http.ResponseWriter, detail string) {
	Write(w, http.StatusConflict, "Conflict", detail, "")
}

func Internal(w http.ResponseWriter, detail string) {
	Write(w, http.StatusInternalServerError, "Internal Server Error", detail, "")
}

func NotImplemented(w http.ResponseWriter, detail string) {
	Write(w, http.StatusNotImplemented, "Not Implemented", detail, "")
}