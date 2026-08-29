package reconcile

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/prop4n/proxmops/internal/manifest"
	"github.com/prop4n/proxmops/internal/status"
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
	return NewEngine(staticSource{}, []Reconciler{staticReconciler{plan}}, opts, testLogger(), nil)
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

func TestEngineRecordsStatus(t *testing.T) {
	// Two desired VMs; the reconciler reports a create for web-01 only.
	src := staticSource{res: []manifest.Resource{vmResource("web-01"), vmResource("web-02")}}
	plan := Plan{Actions: []Action{{
		Type: ActionCreate, Kind: manifest.KindVirtualMachine, Name: "web-01", Reason: "missing",
		Apply: func(context.Context) error { return nil },
	}}}
	st := status.NewStore()
	eng := NewEngine(src, []Reconciler{staticReconciler{plan}}, Options{AutoSync: false}, testLogger(), st)

	if _, err := eng.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}

	byName := map[string]status.Resource{}
	for _, r := range st.Get().Resources {
		byName[r.Name] = r
	}
	if byName["web-01"].State != status.StateOutOfSync {
		t.Errorf("web-01 = %q, want OutOfSync", byName["web-01"].State)
	}
	if byName["web-02"].State != status.StateSynced {
		t.Errorf("web-02 = %q, want Synced", byName["web-02"].State)
	}
}

func TestEngineAnnouncesPlanBeforeApplying(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	applied := 0
	plan := Plan{Actions: []Action{countingAction(ActionCreate, &applied)}}
	// Detect-only mode: it must still announce the drift and the planned action.
	eng := NewEngine(staticSource{}, []Reconciler{staticReconciler{plan}}, Options{AutoSync: false}, log, nil)

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

func TestEngineRecordsVMIDAndTransitions(t *testing.T) {
	src := staticSource{res: []manifest.Resource{func() manifest.VirtualMachine {
		vm := vmResource("web-01")
		vm.Spec.VMID = 101
		return vm
	}()}}

	// A mutable reconciler so successive passes can change the plan.
	rec := &staticReconciler{}
	st := status.NewStore()
	eng := NewEngine(src, []Reconciler{rec}, Options{AutoSync: false}, testLogger(), st)
	stateOf := func() status.Resource {
		return st.Get().Resources[0]
	}

	// Pass 1: drift (create) — the state is stamped now.
	rec.plan = Plan{Actions: []Action{{Type: ActionCreate, Kind: manifest.KindVirtualMachine, Name: "web-01"}}}
	if _, err := eng.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	first := stateOf()
	if first.VMID != 101 {
		t.Errorf("vmid = %d, want 101", first.VMID)
	}
	if first.State != status.StateOutOfSync || first.LastTransition.IsZero() {
		t.Fatalf("pass 1 = %+v, want OutOfSync with a transition timestamp", first)
	}

	// Pass 2: drift resolved — a new transition timestamp is stamped.
	time.Sleep(2 * time.Millisecond)
	rec.plan = Plan{}
	if _, err := eng.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	second := stateOf()
	if second.State != status.StateSynced {
		t.Fatalf("pass 2 state = %q, want Synced", second.State)
	}
	if !second.LastTransition.After(first.LastTransition) {
		t.Fatalf("transition not re-stamped after state change: %v -> %v", first.LastTransition, second.LastTransition)
	}

	// Pass 3: still in sync — the timestamp is preserved, not reset.
	time.Sleep(2 * time.Millisecond)
	if _, err := eng.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	third := stateOf()
	if !third.LastTransition.Equal(second.LastTransition) {
		t.Fatalf("transition reset on unchanged state: %v -> %v", second.LastTransition, third.LastTransition)
	}
}
