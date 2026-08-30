package reconcile

import (
	"context"
	"log/slog"
	"sync"

	"github.com/prop4n/proxmops/internal/manifest"
)

// Dispatcher applies actions in the background, bounded and de-duplicated, so the
// reconcile loop never blocks on a slow action and keeps scanning. An action
// already running is not started again; when the pool is full, extra actions are
// dropped and picked up on a later scan.
type Dispatcher struct {
	log      *slog.Logger
	events   EventSink
	sem      chan struct{}
	mu       sync.Mutex
	inflight map[actionKey]struct{}
	wg       sync.WaitGroup
}

type actionKey struct {
	kind manifest.Kind
	name string
}

// NewDispatcher returns a Dispatcher bounded to limit concurrent actions.
func NewDispatcher(limit int, log *slog.Logger) *Dispatcher {
	if limit <= 0 {
		limit = defaultConcurrency
	}
	return &Dispatcher{
		log:      log,
		sem:      make(chan struct{}, limit),
		inflight: make(map[actionKey]struct{}),
	}
}

// SetEventSink attaches a sink so applied/failed actions are recorded in the
// resource history. Optional; nil means no history is recorded.
func (d *Dispatcher) SetEventSink(s EventSink) { d.events = s }

// Submit starts action a in the background unless it is already running or the
// pool is full. It never blocks; a dropped action is retried on the next scan.
func (d *Dispatcher) Submit(ctx context.Context, a Action) {
	k := actionKey{a.Kind, a.Name}

	d.mu.Lock()
	if _, running := d.inflight[k]; running {
		d.mu.Unlock()
		return
	}
	select {
	case d.sem <- struct{}{}:
	default:
		d.mu.Unlock()
		return
	}
	d.inflight[k] = struct{}{}
	d.mu.Unlock()

	d.wg.Go(func() {
		defer func() {
			<-d.sem
			d.mu.Lock()
			delete(d.inflight, k)
			d.mu.Unlock()
		}()
		d.log.Info("applying", "action", a.String(), "reason", a.Reason)
		if err := a.Apply(ctx); err != nil {
			d.log.Error("apply failed", "action", a.String(), "err", err)
			d.record(Event{Kind: a.Kind, Name: a.Name, Type: EventFailed, Reason: err.Error(), Commit: a.Commit})
			return
		}
		d.log.Info("applied", "action", a.String())
		d.record(Event{Kind: a.Kind, Name: a.Name, Type: EventApplied, Reason: string(a.Type), Commit: a.Commit})
	})
}

// record forwards an event to the sink when one is attached.
func (d *Dispatcher) record(ev Event) {
	if d.events != nil {
		d.events.Record(context.Background(), ev)
	}
}

// Wait blocks until every in-flight action has finished.
func (d *Dispatcher) Wait() {
	d.wg.Wait()
}
