package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/prop4n/proxmops/internal/logbuf"
)

// LogSource provides the daemon's recent log lines and a live feed.
type LogSource interface {
	Snapshot() []logbuf.Entry
	Subscribe() (<-chan logbuf.Entry, func())
}

// handleLogs returns the retained log lines, oldest first.
func (s *Server) handleLogs(w http.ResponseWriter, _ *http.Request) {
	if s.logs == nil {
		writeJSON(w, http.StatusOK, []logbuf.Entry{})
		return
	}
	writeJSON(w, http.StatusOK, s.logs.Snapshot())
}

// handleLogStream streams new log lines over Server-Sent Events.
func (s *Server) handleLogStream(w http.ResponseWriter, r *http.Request) {
	if s.logs == nil {
		writeError(w, http.StatusServiceUnavailable, "logs unavailable")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	entries, cancel := s.logs.Subscribe()
	defer cancel()

	emit := func(e logbuf.Entry) bool {
		data, err := json.Marshal(e)
		if err != nil {
			return true
		}
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(sseWriteTimeout))
		if _, err := fmt.Fprintf(w, "event: log\ndata: %s\n\n", data); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	keepAlive := time.NewTicker(sseKeepAlive)
	defer keepAlive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-entries:
			if !emit(e) {
				return
			}
		case <-keepAlive.C:
			_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(sseWriteTimeout))
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
