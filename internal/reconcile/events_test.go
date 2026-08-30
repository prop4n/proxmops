package reconcile

import (
	"context"
	"sync"
	"testing"

	"github.com/prop4n/proxmops/internal/manifest"
	"github.com/prop4n/proxmops/internal/status"
)

// recordingSink collects events for assertions, safe for concurrent use.
type recordingSink struct {
	mu     sync.Mutex
	events []Event
}

func (s *recordingSink) Record(_ context.Context, e Event) {
	s.mu.Lock()
	s.events = append(s.events, e)
	s.mu.Unlock()
}

func (s *recordingSink) types() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.events))
	for i, e := range s.events {
		out[i] = e.Type
	}
	return out
}

func TestEngineRecordsDriftThenSynced(t *testing.T) {
	st := status.NewStore()
	rec := &staticReconciler{}
	sink := &recordingSink{}
	src := staticSource{res: []manifest.Resource{vmResource("web-01")}}
	eng := NewEngine(src, []Reconciler{rec}, Options{AutoSync: false}, testLogger(), st)
	eng.SetEventSink(sink)
	d := NewDispatcher(1, testLogger())

	// Pass 1: a create is planned -> the resource is OutOfSync -> drifted event.
	rec.plan = Plan{Actions: []Action{{Type: ActionCreate, Kind: manifest.KindVirtualMachine, Name: "web-01"}}}
	if err := eng.Scan(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	// Pass 2: plan empty -> resource returns to Synced -> synced event.
	rec.plan = Plan{}
	if err := eng.Scan(context.Background(), d); err != nil {
		t.Fatal(err)
	}

	got := sink.types()
	if len(got) != 2 || got[0] != EventDrifted || got[1] != EventSynced {
		t.Fatalf("events = %v, want [drifted synced]", got)
	}
}

func TestDispatcherRecordsAppliedAndFailed(t *testing.T) {
	sink := &recordingSink{}
	d := NewDispatcher(2, testLogger())
	d.SetEventSink(sink)

	ok := Action{Type: ActionCreate, Kind: manifest.KindIso, Name: "ok", Commit: "c1",
		Apply: func(context.Context) error { return nil }}
	bad := Action{Type: ActionCreate, Kind: manifest.KindIso, Name: "bad", Commit: "c1",
		Apply: func(context.Context) error { return context.DeadlineExceeded }}
	d.Submit(context.Background(), ok)
	d.Submit(context.Background(), bad)
	d.Wait()

	var applied, failed int
	for _, e := range sink.events {
		switch e.Type {
		case EventApplied:
			applied++
			if e.Commit != "c1" || e.Reason != string(ActionCreate) {
				t.Errorf("applied event fields: %+v", e)
			}
		case EventFailed:
			failed++
		}
	}
	if applied != 1 || failed != 1 {
		t.Fatalf("applied=%d failed=%d, want 1/1", applied, failed)
	}
}
