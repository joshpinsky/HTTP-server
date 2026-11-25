package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
)

// maxEchoBodyBytes is the hard cap on a POST /echo request body. The limit is
// enforced while reading, not after buffering, so an oversized body costs a
// bounded amount of memory.
const maxEchoBodyBytes = 1 << 20 // 1 MiB

type healthResponse struct {
	Status string `json:"status"`
}

func (a *API) handleRoot(w http.ResponseWriter, _ *http.Request) {
	writePlain(w, http.StatusOK, "ok\n")
}

func (a *API) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

// handleEchoGet reflects the msg query parameter back as plain text.
//
// Presence is checked separately from emptiness: ?msg= is a present-but-empty
// value and is echoed as an empty line, while a missing msg is a client error.
func (a *API) handleEchoGet(w http.ResponseWriter, r *http.Request) {
	values, ok := r.URL.Query()["msg"]
	if !ok {
		WriteError(w, http.StatusBadRequest, "missing_query_parameter", `query parameter "msg" is required`)
		return
	}
	writePlain(w, http.StatusOK, values[0]+"\n")
}

// handleEchoPost validates and echoes a JSON document.
//
// The body is decoded into json.RawMessage so the echo is byte-faithful:
// numbers keep their original text instead of being reformatted through
// float64, and object key order is preserved.
func (a *API) handleEchoPost(w http.ResponseWriter, r *http.Request) {
	body := http.MaxBytesReader(w, r.Body, maxEchoBodyBytes)
	defer body.Close()

	dec := json.NewDecoder(body)

	var payload json.RawMessage
	if err := dec.Decode(&payload); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			WriteError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "request body exceeds 1 MiB")
			return
		}
		WriteError(w, http.StatusBadRequest, "invalid_json", "request body is not valid JSON")
		return
	}

	// A second decode must hit EOF, otherwise the body held more than the one
	// JSON value we accepted and the request is ambiguous.
	if err := dec.Decode(new(json.RawMessage)); !errors.Is(err, io.EOF) {
		WriteError(w, http.StatusBadRequest, "invalid_json", "request body must contain exactly one JSON value")
		return
	}

	writeJSONBytes(w, http.StatusCreated, payload)
}

func writePlain(w http.ResponseWriter, status int, body string) {
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}
