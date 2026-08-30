// Package status holds the latest reconciliation result so the API and web UI
// can report what proxmops sees, ArgoCD-style. The store is safe for concurrent
// use: the reconciliation loop writes it and HTTP handlers read it. Handlers
// may also subscribe and push updates to browsers over SSE.
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
	VMID   int    `json:"vmid,omitempty"`
	State  State  `json:"state"`
	Action string `json:"action,omitempty"`
	Reason string `json:"reason,omitempty"`
	// LastTransition is when the resource entered its current state, so the
	// UI can show "in sync for 5m" without a per-resource history.
	LastTransition time.Time `json:"lastTransition,omitempty"`
}

// Snapshot is the result of the most recent reconciliation pass.
type Snapshot struct {
	UpdatedAt time.Time `json:"updatedAt"`
	InSync    bool      `json:"inSync"`
	// Configured reports whether the daemon has a usable target. A false
	// value means the web UI must be filled in before anything reconciles.
	Configured bool `json:"configured"`
	// Commit is the Git commit the desired state was read from, empty for a
	// local (non-Git) source.
	Commit    string     `json:"commit,omitempty"`
	Error     string     `json:"error,omitempty"`
	Resources []Resource `json:"resources"`
}

// Store holds the current Snapshot and fans out updates to subscribers.
type Store struct {
	mu   sync.RWMutex
	snap Snapshot

	subsMu sync.Mutex
	subs   map[chan Snapshot]struct{}
}

// NewStore returns an empty Store.
func NewStore() *Store {
	return &Store{
		snap: Snapshot{Resources: []Resource{}},
		subs: make(map[chan Snapshot]struct{}),
	}
}

// Set replaces the current snapshot and notifies subscribers.
func (s *Store) Set(snap Snapshot) {
	if snap.Resources == nil {
		snap.Resources = []Resource{}
	}
	s.mu.Lock()
	s.snap = snap
	s.mu.Unlock()
	s.broadcast(snap)
}

// Get returns the current snapshot.
func (s *Store) Get() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snap
}

// Subscribe registers a listener for snapshot updates. The channel is
// buffered; a slow consumer misses older snapshots rather than recent ones,
// since every event carries the full snapshot anyway. The returned cancel
// func unregisters the listener.
func (s *Store) Subscribe() (<-chan Snapshot, func()) {
	ch := make(chan Snapshot, 8)
	s.subsMu.Lock()
	s.subs[ch] = struct{}{}
	s.subsMu.Unlock()
	return ch, func() {
		s.subsMu.Lock()
		delete(s.subs, ch)
		s.subsMu.Unlock()
	}
}

// broadcast sends snap to every subscriber without ever blocking on a slow
// consumer. When a subscriber's buffer is full, its oldest retained snapshot
// is dropped so the channel always keeps the most recent ones.
func (s *Store) broadcast(snap Snapshot) {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	for ch := range s.subs {
		select {
		case ch <- snap:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- snap:
			default:
			}
		}
	}
}
