// Package status holds the latest reconciliation result so the API and web UI
// can report what proxmops sees, ArgoCD-style. The store is safe for concurrent
// use: the reconciliation loop writes it and HTTP handlers read it.
package status

import (
	"sync"
	"time"
)

// State is the sync state of a single resource.
type State string

// Recognised states.
const (
	StateSynced    State = "Synced"
	StateOutOfSync State = "OutOfSync"
)

// Resource is the observed sync status of one managed resource.
type Resource struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Node   string `json:"node,omitempty"`
	State  State  `json:"state"`
	Action string `json:"action,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// Snapshot is the result of the most recent reconciliation pass.
type Snapshot struct {
	UpdatedAt time.Time  `json:"updatedAt"`
	InSync    bool       `json:"inSync"`
	Error     string     `json:"error,omitempty"`
	Resources []Resource `json:"resources"`
}

// Store holds the current Snapshot.
type Store struct {
	mu   sync.RWMutex
	snap Snapshot
}

// NewStore returns an empty Store.
func NewStore() *Store {
	return &Store{snap: Snapshot{Resources: []Resource{}}}
}

// Set replaces the current snapshot.
func (s *Store) Set(snap Snapshot) {
	if snap.Resources == nil {
		snap.Resources = []Resource{}
	}
	s.mu.Lock()
	s.snap = snap
	s.mu.Unlock()
}

// Get returns the current snapshot.
func (s *Store) Get() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snap
}
