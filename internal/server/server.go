// Package server exposes the proxmops HTTP API and serves the embedded web UI.
// It is a thin transport layer with no reconciliation logic of its own.
package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

// Server wraps an http.Server with the proxmops routes mounted.
type Server struct {
	http *http.Server
	log  *slog.Logger
}

// New returns a Server listening on addr.
func New(addr string, log *slog.Logger) *Server {
	s := &Server{log: log}
	s.http = &http.Server{
		Addr:              addr,
		Handler:           s.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s
}

// Start begins serving and blocks until the server stops. A clean shutdown
// returns nil.
func (s *Server) Start() error {
	s.log.Info("http server listening", "addr", s.http.Addr)
	if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}
