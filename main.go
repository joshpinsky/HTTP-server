package main

import (
	"errors"
	"log"
	"log/slog"
	"net/http"
	"time"

	"httpserver/handlers"
	"httpserver/middleware"
)

const (
	addr = ":8080"

	// Timeouts are a security control, not a tuning knob: without a header
	// deadline a client can hold a socket open indefinitely by dripping bytes.
	//
	//   ReadHeaderTimeout bounds the request line and headers (slowloris).
	//   ReadTimeout       bounds reading the whole request including the body.
	//   WriteTimeout      bounds writing the response to a slow reader.
	//   IdleTimeout       bounds how long a keep-alive connection may sit idle.
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 10 * time.Second
	idleTimeout       = 60 * time.Second

	// maxHeaderBytes caps the request line plus all headers at 1 MiB.
	maxHeaderBytes = 1 << 20
)

func main() {
	api := handlers.New()

	srv := &http.Server{
		Addr:              addr,
		Handler:           middleware.Log(api),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
	}

	slog.Info("listening", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server: %v", err)
	}
}
