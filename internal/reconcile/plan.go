// Package reconcile computes and applies the difference between the desired
// state (manifests) and the observed state (the Proxmox cluster).
//
// Each resource family is handled by its own Reconciler, so the kinds that
// reconcile differently (tag-owned guests, tag-less ISOs) stay isolated. The
// Engine orchestrates the reconcilers and enforces policy (dry-run, prune,
// auto-sync); it has no knowledge of transport or the control loop, so the same
// core powers both the daemon and the read-only plan command.
package reconcile

import (
	"context"
	"fmt"

	"github.com/prop4n/proxmops/internal/manifest"
)

// ActionType is the kind of change a single Action represents.
type ActionType string

// Recognised action types.
const (
	ActionCreate ActionType = "create"
	ActionUpdate ActionType = "update"
	ActionDelete ActionType = "delete"
)

// Action is a single change that converges one resource toward the desired
// state. Apply carries out the change; the Engine decides whether to call it
// based on policy.
type Action struct {
	Type   ActionType
	Kind   manifest.Kind
	Name   string
	Reason string
	Apply  func(ctx context.Context) error
	// Informational actions report a condition (e.g. "reboot required") in the
	// status without being applied. The engine never dispatches them.
	Informational bool
	// Commit is the Git commit this action was planned from, set by the engine
	// before dispatch so applied/failed events are attributed to it.
	Commit string
}

// String renders an action for display, e.g. "create VirtualMachine/web-01".
func (a Action) String() string {
	return fmt.Sprintf("%s %s/%s", a.Type, a.Kind, a.Name)
}

// Plan is an ordered set of actions. An empty plan means the cluster is in sync.
type Plan struct {
	Actions []Action
}

// Empty reports whether the plan contains no actions.
func (p Plan) Empty() bool { return len(p.Actions) == 0 }

// add appends actions from another plan.
func (p *Plan) add(other Plan) { p.Actions = append(p.Actions, other.Actions...) }
