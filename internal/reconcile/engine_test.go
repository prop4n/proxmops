package reconcile

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
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

func TestEngineAnnouncesPlanBeforeApplying(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	applied := 0
	plan := Plan{Actions: []Action{countingAction(ActionCreate, &applied)}}
	// Detect-only mode: it must still announce the drift and the planned action.
	eng := NewEngine(staticSource{}, []Reconciler{staticReconciler{plan}}, Options{AutoSync: false}, log)

	if _, err := eng.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "out of sync") {
		t.Errorf("missing drift announcement in log:\n%s", out)
	}
	if !strings.Contains(out, "planned") || !strings.Contains(out, "create VirtualMachine/x") {
		t.Errorf("missing planned action in log:\n%s", out)
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
