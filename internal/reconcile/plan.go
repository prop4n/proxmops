// Package reconcile computes and applies the difference between the desired
// state (manifests) and the observed state (the Proxmox cluster). It has no
// knowledge of transport or the control loop, so the same core powers both the
// daemon and the read-only plan command.
package reconcile

import (
	"fmt"

	"github.com/prop4n/proxmops/internal/proxmox"
)

// ActionType is the kind of change a single Action represents.
type ActionType string

// Recognised action types.
const (
	ActionCreate ActionType = "create"
	ActionUpdate ActionType = "update"
	ActionDelete ActionType = "delete"
)

// Action is a single change that brings one resource toward the desired state.
type Action struct {
	Type   ActionType
	Object proxmox.Object
	Reason string
}

// String renders an action for display, e.g. "create VirtualMachine/web-01".
func (a Action) String() string {
	return fmt.Sprintf("%s %s/%s", a.Type, a.Object.Kind, a.Object.Name)
}

// Plan is an ordered set of actions. An empty plan means the cluster is in sync.
type Plan struct {
	Actions []Action
}

// Empty reports whether the plan contains no actions.
func (p Plan) Empty() bool { return len(p.Actions) == 0 }
