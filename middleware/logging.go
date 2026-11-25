package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// recorder captures what the handler actually sent so the log line reports the
// real status code rather than an assumed one.
type recorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *recorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *recorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		// A handler that writes without calling WriteHeader sends 200.
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// Log reports one structured line per request: method, path, status, bytes and
// duration. Path is logged rather than the full URL so query strings (and any
// credentials a client puts there) stay out of the log.
func Log(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &recorder{ResponseWriter: w}

		next.ServeHTTP(rec, r)

		status := rec.status
		if status == 0 {
			// The handler never wrote anything at all.
			status = http.StatusOK
		}

		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", status,
			"bytes", rec.bytes,
			"duration", time.Since(start),
			"remote", r.RemoteAddr,
		)
	})
}
