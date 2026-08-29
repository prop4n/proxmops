package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/prop4n/proxmops/internal/config"
	"github.com/prop4n/proxmops/internal/proxmox"
	"github.com/prop4n/proxmops/internal/settings"
	"github.com/prop4n/proxmops/internal/source"
)

// testTimeout bounds each connection probe of the settings test endpoint.
const testTimeout = 10 * time.Second

// handleGetSettings returns the current settings with secrets masked.
func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	if s.settings == nil {
		writeJSON(w, http.StatusOK, settings.NotConfigured())
		return
	}
	st, err := s.settings.Get(r.Context())
	if errors.Is(err, settings.ErrNotConfigured) {
		writeJSON(w, http.StatusOK, settings.NotConfigured())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, st.Masked())
}

// handlePutSettings validates and saves settings sent from the UI. Empty
// secret fields keep the stored values.
func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var update settings.Masked
	if !decodeJSON(w, r, &update) {
		return
	}

	ctx := r.Context()
	current, err := s.settings.Get(ctx)
	if err != nil && !errors.Is(err, settings.ErrNotConfigured) {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	next := update.Settings(current)
	if err := s.settings.Save(ctx, next); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.log.Info("settings updated")
	writeJSON(w, http.StatusOK, next.Masked())
}

// handleTestSettings probes the saved cluster connection and desired-state
// source, so the UI can verify credentials before relying on them.
func (s *Server) handleTestSettings(w http.ResponseWriter, r *http.Request) {
	st, err := s.settings.Get(r.Context())
	if errors.Is(err, settings.ErrNotConfigured) {
		writeError(w, http.StatusBadRequest, "settings not configured yet")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	cfg := st.Config()
	result := settingsTestResult{
		Cluster: settingsTestProbe{OK: true},
		Source:  settingsTestProbe{OK: true},
	}
	if err := testCluster(r.Context(), cfg); err != nil {
		result.Cluster = settingsTestProbe{OK: false, Error: err.Error()}
	}
	if err := testSource(r.Context(), cfg); err != nil {
		result.Source = settingsTestProbe{OK: false, Error: err.Error()}
	}
	writeJSON(w, http.StatusOK, result)
}

// settingsTestResult reports each probe of the connection test.
type settingsTestResult struct {
	Cluster settingsTestProbe `json:"cluster"`
	Source  settingsTestProbe `json:"source"`
}

type settingsTestProbe struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// testCluster verifies the Proxmox API endpoint and token.
func testCluster(ctx context.Context, cfg config.Config) error {
	ctx, cancel := context.WithTimeout(ctx, testTimeout)
	defer cancel()
	return proxmox.New(cfg.Cluster).Ping(ctx)
}

// testSource verifies the desired-state source is reachable.
func testSource(ctx context.Context, cfg config.Config) error {
	ctx, cancel := context.WithTimeout(ctx, testTimeout)
	defer cancel()
	return source.CheckURL(ctx, cfg.Source)
}
