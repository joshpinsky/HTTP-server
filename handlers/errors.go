package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
)

// ErrorResponse is the single JSON envelope returned for every non-2xx response.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// WriteError is the only place in the program that writes a non-2xx response.
//
// The envelope's headers are always set before WriteHeader, so a status line
// can never be preceded by an implicit 200. Callers that need extra headers
// (for example Allow on a 405) must set them on w.Header() before calling this.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, ErrorResponse{Error: code, Message: message})
}

// writeJSON marshals first and writes second, so a marshal failure cannot leave
// a half-written body behind an already-sent status line.
func writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		// Reached only for values that cannot be marshalled, which is a bug in
		// this program rather than bad client input.
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":"internal_error","message":"response could not be encoded"}`)
		return
	}
	writeJSONBytes(w, status, body)
}

// writeJSONBytes writes an already-validated JSON document unchanged.
func writeJSONBytes(w http.ResponseWriter, status int, body []byte) {
	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
