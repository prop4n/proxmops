package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/prop4n/proxmops/internal/auth"
	"github.com/prop4n/proxmops/internal/config"
	"github.com/prop4n/proxmops/internal/crypt"
	"github.com/prop4n/proxmops/internal/manifest"
	"github.com/prop4n/proxmops/internal/proxmox"
	"github.com/prop4n/proxmops/internal/reconcile"
	"github.com/prop4n/proxmops/internal/server"
	"github.com/prop4n/proxmops/internal/settings"
	"github.com/prop4n/proxmops/internal/source"
	"github.com/prop4n/proxmops/internal/status"
	"github.com/prop4n/proxmops/internal/store"
)

const shutdownTimeout = 10 * time.Second

type App struct {
	cfg    config.Config
	log    *slog.Logger
	set    *settings.Service
	events reconcile.EventSink
}

// eventSink adapts the store to reconcile.EventSink, recording resource history.
type eventSink struct {
	store *store.Store
	log   *slog.Logger
}

func (s *eventSink) Record(ctx context.Context, e reconcile.Event) {
	err := s.store.AppendEvent(ctx, store.ResourceEvent{
		Kind: string(e.Kind), Name: e.Name, Type: e.Type, Reason: e.Reason, Commit: e.Commit,
	})
	if err != nil {
		s.log.Error("record event", "err", err)
	}
}

func New(cfg config.Config, log *slog.Logger) *App {
	return &App{cfg: cfg, log: log}
}

// Plan computes the read-only plan from the file configuration; the plan
// command validates the file before reaching this point.
func (a *App) Plan(ctx context.Context) (reconcile.Plan, error) {
	return a.newEngine(a.cfg, nil).Plan(ctx)
}

// DeleteResource removes a single managed resource from the cluster. It resolves
// the resource from the desired state, so a delete only ever targets something
// the repo declares; the resource reappears on the next reconcile if still
// declared. Implements server.ResourceDeleter.
func (a *App) DeleteResource(ctx context.Context, kind, name string) error {
	cfg, err := a.effectiveConfig(ctx)
	if err != nil {
		return err
	}
	if !cfg.Complete() {
		return server.ErrResourceNotFound
	}
	desired, err := source.New(cfg.Source).Desired(ctx)
	if err != nil {
		return err
	}
	return deleteManaged(ctx, desired, proxmox.New(cfg.Cluster), kind, name)
}

// clusterDeleter is the subset of the Proxmox client that deletes resources.
type clusterDeleter interface {
	DeleteISO(ctx context.Context, node, storage, filename string) error
	DeleteGuest(ctx context.Context, obj proxmox.Object) error
}

// deleteManaged finds the desired resource by kind and name and deletes it from
// the cluster. ISO and VirtualMachine are supported; other kinds return
// ErrDeleteUnsupported.
func deleteManaged(ctx context.Context, desired []manifest.Resource, del clusterDeleter, kind, name string) error {
	for _, res := range desired {
		if string(res.GetTypeMeta().Kind) != kind || res.GetObjectMeta().Name != name {
			continue
		}
		switch r := res.(type) {
		case manifest.Iso:
			return del.DeleteISO(ctx, r.Spec.Node, r.Spec.Storage, r.Filename())
		case manifest.VirtualMachine:
			return del.DeleteGuest(ctx, proxmox.Object{
				Kind: proxmox.KindVirtualMachine,
				Node: r.Metadata.Node,
				VMID: r.Spec.VMID,
			})
		default:
			return server.ErrDeleteUnsupported
		}
	}
	return server.ErrResourceNotFound
}

func (a *App) Run(ctx context.Context, addr string) error {
	st, err := store.Open(a.cfg.Server.DatabasePath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()

	key, err := crypt.LoadOrCreateKey(a.cfg.Server.KeyPath)
	if err != nil {
		return fmt.Errorf("load encryption key: %w", err)
	}
	a.set = settings.New(st, key)
	a.events = &eventSink{store: st, log: a.log}

	authSvc := auth.New(st, a.log)
	if err := authSvc.Init(ctx); err != nil {
		return fmt.Errorf("init auth: %w", err)
	}

	statusStore := status.NewStore()
	srv := server.New(server.Options{
		Addr:         addr,
		Auth:         authSvc,
		Status:       statusStore,
		Settings:     a.set,
		Deleter:      a,
		Events:       st,
		CookieSecure: a.cfg.Server.CookieSecure,
	}, a.log)

	errc := make(chan error, 2)
	go func() { errc <- srv.Start() }()
	go func() { errc <- a.reconcileLoop(ctx, statusStore) }()

	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-errc:
		if runErr != nil {
			a.log.Error("component failed", "err", runErr)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	return runErr
}

// reconcileLoop drives reconciliation. It rebuilds the engine from the current
// settings on every pass, so configuration saved from the web UI applies
// without restarting the daemon; the file configuration serves as fallback
// until settings exist.
func (a *App) reconcileLoop(ctx context.Context, statusStore *status.Store) error {
	// The dispatcher applies actions in the background and outlives each pass, so
	// its in-flight registry survives the per-pass engine rebuild. It is created
	// on the first configured pass, sized by the reconcile concurrency then; a
	// later change to that limit takes effect on restart.
	var dispatcher *reconcile.Dispatcher
	defer func() {
		if dispatcher != nil {
			dispatcher.Wait()
		}
	}()

	var wasComplete bool
	var lastCfgErr string
	for {
		cfg, cfgErr := a.effectiveConfig(ctx)
		if cfgErr != nil {
			// Log on state change only, so a broken settings row does not
			// spam the log on every pass.
			if cfgErr.Error() != lastCfgErr {
				a.log.Error("cannot read settings; falling back to file configuration", "err", cfgErr)
				lastCfgErr = cfgErr.Error()
			}
		} else {
			lastCfgErr = ""
		}

		complete := cfg.Complete()
		if complete != wasComplete {
			wasComplete = complete
			if complete {
				a.log.Info("daemon configured; reconciliation started")
			} else {
				a.log.Info("daemon not configured yet; waiting for settings from the web UI")
			}
		}

		if complete {
			if dispatcher == nil {
				dispatcher = reconcile.NewDispatcher(cfg.Reconcile.Concurrency, a.log)
				dispatcher.SetEventSink(a.events)
			}
			engine := a.newEngine(cfg, statusStore)
			engine.SetEventSink(a.events)
			if err := engine.Scan(ctx, dispatcher); err != nil && !errors.Is(err, context.Canceled) {
				a.log.Error("reconcile failed", "err", err)
			}
		} else {
			statusStore.Set(status.Snapshot{
				UpdatedAt:  time.Now(),
				Configured: false,
				Error:      "daemon not configured: fill in the cluster and repository settings",
			})
		}

		interval := cfg.Reconcile.Interval
		if interval <= 0 {
			interval = time.Minute
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(interval):
		}
	}
}

// effectiveConfig returns the settings saved from the web UI, falling back to
// the file configuration when none exist. A read or decrypt failure is
// reported to the caller: a silent fallback would hide a broken settings row.
func (a *App) effectiveConfig(ctx context.Context) (config.Config, error) {
	st, err := a.set.Get(ctx)
	if errors.Is(err, settings.ErrNotConfigured) {
		return a.cfg, nil
	}
	if err != nil {
		return a.cfg, err
	}
	return st.Config(), nil
}

func (a *App) newEngine(cfg config.Config, statusStore *status.Store) *reconcile.Engine {
	client := proxmox.New(cfg.Cluster)
	src := source.New(cfg.Source)
	reconcilers := []reconcile.Reconciler{
		reconcile.NewTemplateReconciler(client),
		reconcile.NewGuestReconciler(client),
		reconcile.NewIsoReconciler(client),
	}
	opts := reconcile.Options{
		AutoSync: cfg.Reconcile.AutoSync,
		DryRun:   cfg.Reconcile.DryRun,
		Prune:    cfg.Reconcile.Prune,
	}
	return reconcile.NewEngine(src, reconcilers, opts, a.log, statusStore)
}
