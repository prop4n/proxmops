package reconcile

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/prop4n/proxmops/internal/manifest"
)

// staticSource returns a fixed desired set.
type staticSource struct{ res []manifest.Resource }

func (s staticSource) Desired(context.Context) ([]manifest.Resource, error) { return s.res, nil }

// staticReconciler returns a fixed plan whose actions record when applied.
type staticReconciler struct{ plan Plan }

func (r staticReconciler) Plan(context.Context, []manifest.Resource) (Plan, error) {
	return r.plan, nil
}

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// countingAction returns an action of the given type that records applies.
func countingAction(t ActionType, applied *int) Action {
	return Action{
		Type: t,
		Kind: manifest.KindVirtualMachine,
		Name: "x",
		Apply: func(context.Context) error {
			*applied++
			return nil
		},
	}
}

func newEngineWith(plan Plan, opts Options) *Engine {
	return NewEngine(staticSource{}, []Reconciler{staticReconciler{plan}}, opts, testLogger())
}

func TestEngineAppliesWhenAutoSync(t *testing.T) {
	applied := 0
	eng := newEngineWith(Plan{Actions: []Action{countingAction(ActionCreate, &applied)}}, Options{AutoSync: true})
	if _, err := eng.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if applied != 1 {
		t.Fatalf("applied = %d, want 1", applied)
	}
}

func TestEngineDoesNotApplyWhenAutoSyncOff(t *testing.T) {
	applied := 0
	eng := newEngineWith(Plan{Actions: []Action{countingAction(ActionCreate, &applied)}}, Options{AutoSync: false})
	if _, err := eng.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if applied != 0 {
		t.Fatalf("applied = %d, want 0 (auto-sync off)", applied)
	}
}

func TestEngineDryRunSkipsApply(t *testing.T) {
	applied := 0
	eng := newEngineWith(Plan{Actions: []Action{countingAction(ActionCreate, &applied)}}, Options{AutoSync: true, DryRun: true})
	if _, err := eng.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if applied != 0 {
		t.Fatalf("applied = %d, want 0 (dry-run)", applied)
	}
}

func TestEngineSkipsDeleteWhenPruneOff(t *testing.T) {
	applied := 0
	eng := newEngineWith(Plan{Actions: []Action{countingAction(ActionDelete, &applied)}}, Options{AutoSync: true, Prune: false})
	if _, err := eng.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if applied != 0 {
		t.Fatalf("applied = %d, want 0 (prune off)", applied)
	}
}

func TestEngineAppliesDeleteWhenPruneOn(t *testing.T) {
	applied := 0
	eng := newEngineWith(Plan{Actions: []Action{countingAction(ActionDelete, &applied)}}, Options{AutoSync: true, Prune: true})
	if _, err := eng.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if applied != 1 {
		t.Fatalf("applied = %d, want 1 (prune on)", applied)
	}
}
