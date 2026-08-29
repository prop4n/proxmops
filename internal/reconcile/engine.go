package reconcile

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/prop4n/proxmops/internal/manifest"
	"github.com/prop4n/proxmops/internal/status"
)

// defaultConcurrency bounds background apply when none is configured.
const defaultConcurrency = 4

// reportedKinds are the resource kinds surfaced in the sync status.
var reportedKinds = []manifest.Kind{
	manifest.KindVirtualMachine,
	manifest.KindContainer,
	manifest.KindIso,
}

// Source supplies the desired state for a reconciliation pass.
type Source interface {
	Desired(ctx context.Context) ([]manifest.Resource, error)
}

// Options controls how a plan is applied.
type Options struct {
	// AutoSync applies the plan; when false the engine only reports drift.
	AutoSync bool
	// DryRun logs the actions without touching the cluster.
	DryRun bool
	// Prune enables deletion of owned resources removed from the repository.
	Prune bool
}

// Engine plans reconciliation and reports status. It never applies directly:
// actions are handed to a Dispatcher so a slow one cannot block the next scan.
// It is transport-agnostic and reused by both the daemon loop and the plan command.
type Engine struct {
	source      Source
	reconcilers []Reconciler
	opts        Options
	log         *slog.Logger
	status      *status.Store
}

// NewEngine wires an Engine from its collaborators. status may be nil (e.g. for
// the read-only plan command).
func NewEngine(source Source, reconcilers []Reconciler, opts Options, log *slog.Logger, status *status.Store) *Engine {
	return &Engine{source: source, reconcilers: reconcilers, opts: opts, log: log, status: status}
}

// computePlan loads the desired state and aggregates every reconciler's plan.
func (e *Engine) computePlan(ctx context.Context) ([]manifest.Resource, Plan, error) {
	desired, err := e.source.Desired(ctx)
	if err != nil {
		return nil, Plan{}, err
	}
	var full Plan
	for _, r := range e.reconcilers {
		p, err := r.Plan(ctx, desired)
		if err != nil {
			return nil, Plan{}, err
		}
		full.add(p)
	}
	return desired, full, nil
}

// Plan computes the combined reconciliation plan without changing anything.
func (e *Engine) Plan(ctx context.Context) (Plan, error) {
	_, plan, err := e.computePlan(ctx)
	return plan, err
}

// Scan computes a plan, records the status snapshot, and hands the applicable
// actions to the dispatcher without waiting for them. Because it never blocks on
// an action, drift appearing while an earlier action is still running is caught
// on the very next scan.
func (e *Engine) Scan(ctx context.Context, d *Dispatcher) error {
	desired, plan, err := e.computePlan(ctx)
	if err != nil {
		e.recordError(err)
		return err
	}
	e.recordStatus(desired, plan)

	if plan.Empty() || !e.opts.AutoSync {
		return nil
	}
	for _, a := range plan.Actions {
		switch {
		case a.Type == ActionDelete && !e.opts.Prune:
			// Prune disabled: owned orphans are reported but never deleted.
		case e.opts.DryRun:
			e.log.Info("dry-run", "action", a.String(), "reason", a.Reason)
		default:
			d.Submit(ctx, a)
		}
	}
	return nil
}

// recordStatus builds a status snapshot from the desired state and the plan and
// stores it. Desired resources with no pending action are Synced; those with an
// action, and owned orphans scheduled for deletion, are OutOfSync. State
// transition timestamps are preserved across passes so the UI can show how
// long a resource has been in its current state.
func (e *Engine) recordStatus(desired []manifest.Resource, plan Plan) {
	if e.status == nil {
		return
	}

	type key struct {
		kind manifest.Kind
		name string
	}
	lastTransition := make(map[key]time.Time)
	prevState := make(map[key]status.State)
	for _, prev := range e.status.Get().Resources {
		k := key{manifest.Kind(prev.Kind), prev.Name}
		lastTransition[k] = prev.LastTransition
		prevState[k] = prev.State
	}

	// transition stamps now, or keeps the previous stamp when the state is
	// unchanged.
	transition := func(k key, state status.State) time.Time {
		if prevState[k] == state && !lastTransition[k].IsZero() {
			return lastTransition[k]
		}
		return time.Now()
	}

	actions := make(map[key]Action, len(plan.Actions))
	for _, a := range plan.Actions {
		actions[key{a.Kind, a.Name}] = a
	}

	resources := make([]status.Resource, 0, len(desired))
	seen := make(map[key]bool)
	for _, r := range desired {
		kind := r.GetTypeMeta().Kind
		if !isReported(kind) {
			continue
		}
		om := r.GetObjectMeta()
		k := key{kind, om.Name}
		seen[k] = true
		res := status.Resource{Kind: string(kind), Name: om.Name, Node: om.Node, State: status.StateSynced}
		if ider, ok := r.(manifest.VMIDer); ok {
			res.VMID = ider.GetVMID()
		}
		if a, ok := actions[k]; ok {
			res.State = status.StateOutOfSync
			res.Action = string(a.Type)
			res.Reason = a.Reason
		}
		res.LastTransition = transition(k, res.State)
		resources = append(resources, res)
	}
	// Owned orphans (deletes) are not in the desired set.
	for k, a := range actions {
		if seen[k] || !isReported(k.kind) {
			continue
		}
		res := status.Resource{
			Kind:   string(k.kind),
			Name:   k.name,
			State:  status.StateOutOfSync,
			Action: string(a.Type),
			Reason: a.Reason,
		}
		res.LastTransition = transition(k, res.State)
		resources = append(resources, res)
	}

	e.status.Set(status.Snapshot{
		UpdatedAt:  time.Now(),
		InSync:     plan.Empty(),
		Configured: true,
		Resources:  resources,
	})
}

// recordError marks the current snapshot as failed while keeping prior results.
func (e *Engine) recordError(err error) {
	if e.status == nil {
		return
	}
	snap := e.status.Get()
	snap.UpdatedAt = time.Now()
	snap.InSync = false
	snap.Configured = true
	snap.Error = err.Error()
	e.status.Set(snap)
}

// isReported reports whether a kind is surfaced in the sync status.
func isReported(kind manifest.Kind) bool {
	return slices.Contains(reportedKinds, kind)
}
