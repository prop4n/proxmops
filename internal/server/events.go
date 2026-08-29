package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	// sseKeepAlive bounds how often a comment is sent on an idle stream, so
	// proxies and browsers keep the connection alive.
	sseKeepAlive = 20 * time.Second
	// sseWriteTimeout bounds each write, so a client that stops reading cannot
	// pin the handler goroutine forever on TCP backpressure.
	sseWriteTimeout = 10 * time.Second
)

// handleEvents streams status snapshots over Server-Sent Events: the current
// snapshot on connect, then one event per reconciliation pass. The stream ends
// when the client disconnects or a write fails.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
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

	events, cancel := s.status.Subscribe()
	defer cancel()

	// emit writes one SSE frame and flushes it, reporting whether the stream
	// is still alive.
	emit := func(format string, args ...any) bool {
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(sseWriteTimeout))
		if _, err := fmt.Fprintf(w, format, args...); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	snap := s.status.Get()
	data, err := json.Marshal(snap)
	if err != nil {
		return
	}
	// Send the current snapshot immediately so the UI paints without waiting
	// for the next pass.
	if !emit("event: snapshot\ndata: %s\n\n", data) {
		return
	}

	keepAlive := time.NewTicker(sseKeepAlive)
	defer keepAlive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case snap := <-events:
			data, err := json.Marshal(snap)
			if err != nil {
				continue
			}
			if !emit("event: snapshot\ndata: %s\n\n", data) {
				return
			}
		case <-keepAlive.C:
			if !emit(": keepalive\n\n") {
				return
			}
		}
	}
}
