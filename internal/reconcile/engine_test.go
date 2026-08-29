package reconcile

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

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
