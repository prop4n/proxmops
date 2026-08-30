package reconcile

import (
	"context"

	"github.com/prop4n/proxmops/internal/manifest"
)

// Event types recorded in a resource's history.
const (
	EventDrifted = "drifted" // went OutOfSync
	EventSynced  = "synced"  // returned to Synced
	EventRemoved = "removed" // disappeared from the desired/observed set
	EventApplied = "applied" // an action ran successfully
	EventFailed  = "failed"  // an action failed
)

// Event is one entry in a resource's history.
type Event struct {
	Kind   manifest.Kind
	Name   string
	Type   string
	Reason string
	Commit string
}

// EventSink records resource events. Implementations must be safe for
// concurrent use, since the dispatcher records from background goroutines.
type EventSink interface {
	Record(ctx context.Context, e Event)
}
