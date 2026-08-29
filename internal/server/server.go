// Package server exposes the proxmops HTTP API and serves the embedded web UI.
// It is a thin transport layer with no reconciliation logic of its own.
package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/prop4n/proxmops/internal/auth"
	"github.com/prop4n/proxmops/internal/settings"
	"github.com/prop4n/proxmops/internal/status"
)

// Server wraps an http.Server with the proxmops routes mounted.
type Server struct {
	http         *http.Server
	log          *slog.Logger
	auth         *auth.Service
	status       *status.Store
	settings     *settings.Service
	cookieSecure bool
}

// Options configures a Server.
type Options struct {
	Addr         string
	Auth         *auth.Service
	Status       *status.Store
	Settings     *settings.Service
	CookieSecure bool
}

// New returns a Server listening on opts.Addr.
func New(opts Options, log *slog.Logger) *Server {
	s := &Server{
		log:          log,
		auth:         opts.Auth,
		status:       opts.Status,
		settings:     opts.Settings,
		cookieSecure: opts.CookieSecure,
	}
	s.http = &http.Server{
		Addr:              opts.Addr,
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
