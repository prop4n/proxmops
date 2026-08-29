package reconcile

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/prop4n/proxmops/internal/proxmox"
)

// ExecuteOptions controls how a plan is applied.
type ExecuteOptions struct {
	// AutoSync applies the plan; when false the engine only reports drift.
	AutoSync bool
	// DryRun logs the actions without calling the cluster.
	DryRun bool
	// Prune enables deletion of owned resources removed from the repository.
	Prune bool
}

// Executor applies plans against a Proxmox cluster.
type Executor struct {
	client proxmox.Client
	log    *slog.Logger
}

// NewExecutor returns an Executor backed by client.
func NewExecutor(client proxmox.Client, log *slog.Logger) *Executor {
	return &Executor{client: client, log: log}
}

// Apply carries out every action in the plan subject to opts. Deletions are
// skipped unless pruning is enabled, honouring the opt-in prune safety rule.
func (e *Executor) Apply(ctx context.Context, plan Plan, opts ExecuteOptions) error {
	for _, a := range plan.Actions {
		if a.Type == ActionDelete && !opts.Prune {
			e.log.Info("skipping delete (prune disabled)", "action", a.String())
			continue
		}
		if opts.DryRun {
			e.log.Info("dry-run", "action", a.String(), "reason", a.Reason)
			continue
		}
		if err := e.dispatch(ctx, a); err != nil {
			return fmt.Errorf("apply %s: %w", a, err)
		}
		e.log.Info("applied", "action", a.String())
	}
	return nil
}

// dispatch routes a single action to the matching client call.
func (e *Executor) dispatch(ctx context.Context, a Action) error {
	switch a.Type {
	case ActionCreate, ActionUpdate:
		return e.client.Apply(ctx, a.Object)
	case ActionDelete:
		return e.client.Delete(ctx, a.Object)
	default:
		return fmt.Errorf("unknown action type %q", a.Type)
	}
}
