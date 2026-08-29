package reconcile

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/prop4n/proxmops/internal/manifest"
)

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

// Engine orchestrates the reconcilers and enforces policy. It is
// transport-agnostic and reused by both the daemon loop and the plan command.
type Engine struct {
	source      Source
	reconcilers []Reconciler
	opts        Options
	log         *slog.Logger
}

// NewEngine wires an Engine from its collaborators.
func NewEngine(source Source, reconcilers []Reconciler, opts Options, log *slog.Logger) *Engine {
	return &Engine{source: source, reconcilers: reconcilers, opts: opts, log: log}
}

// Plan computes the combined reconciliation plan without changing anything.
func (e *Engine) Plan(ctx context.Context) (Plan, error) {
	desired, err := e.source.Desired(ctx)
	if err != nil {
		return Plan{}, err
	}
	var full Plan
	for _, r := range e.reconcilers {
		p, err := r.Plan(ctx, desired)
		if err != nil {
			return Plan{}, err
		}
		full.add(p)
	}
	return full, nil
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
	return plan, e.apply(ctx, plan)
}

// apply carries out a plan subject to policy. Deletes are skipped unless
// pruning is enabled, honouring the opt-in prune rule.
func (e *Engine) apply(ctx context.Context, plan Plan) error {
	for _, a := range plan.Actions {
		switch {
		case a.Type == ActionDelete && !e.opts.Prune:
			e.log.Info("skipping delete (prune disabled)", "action", a.String())
		case e.opts.DryRun:
			e.log.Info("dry-run", "action", a.String(), "reason", a.Reason)
		default:
			if err := a.Apply(ctx); err != nil {
				return fmt.Errorf("apply %s: %w", a, err)
			}
			e.log.Info("applied", "action", a.String())
		}
	}
	return nil
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
