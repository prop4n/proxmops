package reconcile

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prop4n/proxmops/internal/manifest"
)

func isoAction(name string, apply func(context.Context) error) Action {
	return Action{Type: ActionCreate, Kind: manifest.KindIso, Name: name, Apply: apply}
}

func TestDispatcherRunsInBackground(t *testing.T) {
	d := NewDispatcher(4, testLogger())
	done := make(chan struct{})
	d.Submit(context.Background(), isoAction("x", func(context.Context) error {
		close(done)
		return nil
	})) // must return immediately

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("action did not run in the background")
	}
	d.Wait()
}

// TestDispatcherSlowActionDoesNotBlockOthers is the regression: a slow action in
// flight must not stop a different one from being started — the whole point of
// decoupling scanning from applying.
func TestDispatcherSlowActionDoesNotBlockOthers(t *testing.T) {
	d := NewDispatcher(4, testLogger())
	nixStarted := make(chan struct{})
	nixBlock := make(chan struct{})
	alpineDone := make(chan struct{})

	d.Submit(context.Background(), isoAction("nix", func(context.Context) error {
		close(nixStarted)
		<-nixBlock
		return nil
	}))
	<-nixStarted // nix is now in flight and blocked

	d.Submit(context.Background(), isoAction("alpine", func(context.Context) error {
		close(alpineDone)
		return nil
	}))

	select {
	case <-alpineDone:
	case <-time.After(2 * time.Second):
		t.Fatal("alpine did not run while a slow action was in flight")
	}
	close(nixBlock)
	d.Wait()
}

func TestDispatcherDeduplicatesInflight(t *testing.T) {
	d := NewDispatcher(4, testLogger())
	var count atomic.Int32
	started := make(chan struct{}, 1)
	block := make(chan struct{})
	a := isoAction("same", func(context.Context) error {
		count.Add(1)
		select {
		case started <- struct{}{}:
		default:
		}
		<-block
		return nil
	})

	d.Submit(context.Background(), a)
	<-started // first run is in flight
	d.Submit(context.Background(), a)
	d.Submit(context.Background(), a)
	time.Sleep(30 * time.Millisecond)

	if got := count.Load(); got != 1 {
		t.Fatalf("ran %d times, want 1 (deduplicated while in flight)", got)
	}
	close(block)
	d.Wait()
}

func TestDispatcherRespectsLimit(t *testing.T) {
	d := NewDispatcher(2, testLogger())
	var started atomic.Int32
	block := make(chan struct{})
	mk := func(name string) Action {
		return isoAction(name, func(context.Context) error {
			started.Add(1)
			<-block
			return nil
		})
	}
	for _, n := range []string{"a", "b", "c", "d"} {
		d.Submit(context.Background(), mk(n))
	}
	time.Sleep(50 * time.Millisecond)

	if got := started.Load(); got != 2 {
		t.Fatalf("started = %d, want 2 (extra actions wait for a slot)", got)
	}
	close(block)
	d.Wait()
}
