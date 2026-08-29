package reconcile

import (
	"context"
	"log/slog"
	"time"

	"github.com/prop4n/proxmops/internal/manifest"
	"github.com/prop4n/proxmops/internal/proxmox"
)

// Source supplies the desired state for a reconciliation pass.
type Source interface {
	Desired(ctx context.Context) ([]manifest.Resource, error)
}

// Engine reconciles desired state onto a cluster. It is transport-agnostic and
// safe to reuse from both the daemon loop and the plan command.
type Engine struct {
	source   Source
	client   proxmox.Client
	executor *Executor
	opts     ExecuteOptions
	log      *slog.Logger
}

// NewEngine wires an Engine from its collaborators.
func NewEngine(source Source, client proxmox.Client, opts ExecuteOptions, log *slog.Logger) *Engine {
	return &Engine{
		source:   source,
		client:   client,
		executor: NewExecutor(client, log),
		opts:     opts,
		log:      log,
	}
}

// Plan computes the current reconciliation plan without changing anything.
func (e *Engine) Plan(ctx context.Context) (Plan, error) {
	desired, err := e.source.Desired(ctx)
	if err != nil {
		return Plan{}, err
	}
	observed, err := e.client.List(ctx)
	if err != nil {
		return Plan{}, err
	}
	return Diff(desired, observed), nil
}

// Reconcile computes a plan and, when auto-sync is enabled, applies it once.
func (e *Engine) Reconcile(ctx context.Context) (Plan, error) {
	plan, err := e.Plan(ctx)
	if err != nil {
		return Plan{}, err
	}
	if plan.Empty() {
		e.log.Info("cluster in sync")
		return plan, nil
	}
	if !e.opts.AutoSync {
		e.log.Info("out of sync (auto-sync disabled)", "actions", len(plan.Actions))
		return plan, nil
	}
	if err := e.executor.Apply(ctx, plan, e.opts); err != nil {
		return plan, err
	}
	return plan, nil
}

// Run reconciles once immediately and then on every tick of interval until ctx
// is cancelled.
func (e *Engine) Run(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if _, err := e.Reconcile(ctx); err != nil {
			e.log.Error("reconcile failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
