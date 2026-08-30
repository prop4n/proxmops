package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/prop4n/proxmops/internal/store"
)

// EventsReader reads a resource's recorded history.
type EventsReader interface {
	EventsFor(ctx context.Context, kind, name string, limit int) ([]store.ResourceEvent, error)
}

// ResourceDeleter removes a managed resource from the cluster. It is implemented
// outside the transport layer and injected through Options.
type ResourceDeleter interface {
	DeleteResource(ctx context.Context, kind, name string) error
}

// ResourceDetail is the desired manifest plus best-effort observed cluster state
// for one resource.
type ResourceDetail struct {
	DesiredYAML string            `json:"desiredYAML"`
	Observed    *ObservedResource `json:"observed,omitempty"`
}

// ObservedResource is the resource's actual state on the cluster.
type ObservedResource struct {
	Present    bool   `json:"present"`
	Cores      int    `json:"cores,omitempty"`
	MemoryMB   int    `json:"memoryMB,omitempty"`
	CPU        string `json:"cpu,omitempty"`
	Running    bool   `json:"running,omitempty"`
	IP         string `json:"ip,omitempty"`
	Nameserver string `json:"nameserver,omitempty"`
}

// ResourceDetailer returns the desired manifest and observed state of a resource.
type ResourceDetailer interface {
	ResourceDetail(ctx context.Context, kind, name string) (ResourceDetail, error)
}

var (
	// ErrResourceNotFound is returned when no managed resource matches.
	ErrResourceNotFound = errors.New("resource not found")
	// ErrDeleteUnsupported is returned for kinds whose deletion is not built yet.
	ErrDeleteUnsupported = errors.New("deletion not supported for this kind")
)

// handleDeleteResource deletes a single managed resource from the cluster. The
// resource reappears on the next reconcile if it is still declared in the repo.
func (s *Server) handleDeleteResource(w http.ResponseWriter, r *http.Request) {
	if s.deleter == nil {
		writeError(w, http.StatusServiceUnavailable, "deletion unavailable")
		return
	}
	kind := chi.URLParam(r, "kind")
	name := chi.URLParam(r, "name")

	switch err := s.deleter.DeleteResource(r.Context(), kind, name); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, ErrResourceNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrDeleteUnsupported):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		s.log.Error("delete resource failed", "kind", kind, "name", name, "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

// handleResourceDetail returns the desired manifest and observed state.
func (s *Server) handleResourceDetail(w http.ResponseWriter, r *http.Request) {
	if s.detailer == nil {
		writeError(w, http.StatusServiceUnavailable, "detail unavailable")
		return
	}
	kind := chi.URLParam(r, "kind")
	name := chi.URLParam(r, "name")
	detail, err := s.detailer.ResourceDetail(r.Context(), kind, name)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, detail)
	case errors.Is(err, ErrResourceNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		s.log.Error("resource detail failed", "kind", kind, "name", name, "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

// handleResourceEvents returns a resource's recorded history, newest first.
func (s *Server) handleResourceEvents(w http.ResponseWriter, r *http.Request) {
	if s.events == nil {
		writeJSON(w, http.StatusOK, []store.ResourceEvent{})
		return
	}
	kind := chi.URLParam(r, "kind")
	name := chi.URLParam(r, "name")
	events, err := s.events.EventsFor(r.Context(), kind, name, 200)
	if err != nil {
		s.log.Error("read events", "kind", kind, "name", name, "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if events == nil {
		events = []store.ResourceEvent{}
	}
	writeJSON(w, http.StatusOK, events)
}
