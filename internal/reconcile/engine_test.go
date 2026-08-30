package reconcile

import (
	"context"
	"io"
	"log/slog"
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

// scan runs one pass and waits for its dispatched actions to finish.
func scan(t *testing.T, plan Plan, opts Options) {
	t.Helper()
	eng := newEngineWith(plan, opts)
	d := NewDispatcher(4, testLogger())
	if err := eng.Scan(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	d.Wait()
}

func TestScanAppliesWhenAutoSync(t *testing.T) {
	applied := 0
	scan(t, Plan{Actions: []Action{countingAction(ActionCreate, &applied)}}, Options{AutoSync: true})
	if applied != 1 {
		t.Fatalf("applied = %d, want 1", applied)
	}
}

func TestScanDoesNotApplyWhenAutoSyncOff(t *testing.T) {
	applied := 0
	scan(t, Plan{Actions: []Action{countingAction(ActionCreate, &applied)}}, Options{AutoSync: false})
	if applied != 0 {
		t.Fatalf("applied = %d, want 0 (auto-sync off)", applied)
	}
}

func TestScanDryRunSkipsApply(t *testing.T) {
	applied := 0
	scan(t, Plan{Actions: []Action{countingAction(ActionCreate, &applied)}}, Options{AutoSync: true, DryRun: true})
	if applied != 0 {
		t.Fatalf("applied = %d, want 0 (dry-run)", applied)
	}
}

func TestScanSkipsDeleteWhenPruneOff(t *testing.T) {
	applied := 0
	scan(t, Plan{Actions: []Action{countingAction(ActionDelete, &applied)}}, Options{AutoSync: true, Prune: false})
	if applied != 0 {
		t.Fatalf("applied = %d, want 0 (prune off)", applied)
	}
}

func TestScanAppliesDeleteWhenPruneOn(t *testing.T) {
	applied := 0
	scan(t, Plan{Actions: []Action{countingAction(ActionDelete, &applied)}}, Options{AutoSync: true, Prune: true})
	if applied != 1 {
		t.Fatalf("applied = %d, want 1 (prune on)", applied)
	}
}

func TestScanRecordsStatus(t *testing.T) {
	// Two desired VMs; the reconciler reports a create for web-01 only.
	src := staticSource{res: []manifest.Resource{vmResource("web-01"), vmResource("web-02")}}
	plan := Plan{Actions: []Action{{
		Type: ActionCreate, Kind: manifest.KindVirtualMachine, Name: "web-01", Reason: "missing",
		Apply: func(context.Context) error { return nil },
	}}}
	st := status.NewStore()
	eng := NewEngine(src, []Reconciler{staticReconciler{plan}}, Options{AutoSync: false}, testLogger(), st)

	if err := eng.Scan(context.Background(), NewDispatcher(4, testLogger())); err != nil {
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

func TestScanRecordsVMIDAndTransitions(t *testing.T) {
	src := staticSource{res: []manifest.Resource{func() manifest.VirtualMachine {
		vm := vmResource("web-01")
		vm.Spec.VMID = 101
		return vm
	}()}}

	rec := &staticReconciler{}
	st := status.NewStore()
	eng := NewEngine(src, []Reconciler{rec}, Options{AutoSync: false}, testLogger(), st)
	d := NewDispatcher(4, testLogger())
	stateOf := func() status.Resource { return st.Get().Resources[0] }

	// Pass 1: drift (create) — the state is stamped now.
	rec.plan = Plan{Actions: []Action{{Type: ActionCreate, Kind: manifest.KindVirtualMachine, Name: "web-01"}}}
	if err := eng.Scan(context.Background(), d); err != nil {
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
	if err := eng.Scan(context.Background(), d); err != nil {
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
	if err := eng.Scan(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	third := stateOf()
	if !third.LastTransition.Equal(second.LastTransition) {
		t.Fatalf("transition reset on unchanged state: %v -> %v", second.LastTransition, third.LastTransition)
	}
}
