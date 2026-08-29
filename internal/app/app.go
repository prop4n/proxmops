package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/prop4n/proxmops/internal/auth"
	"github.com/prop4n/proxmops/internal/config"
	"github.com/prop4n/proxmops/internal/proxmox"
	"github.com/prop4n/proxmops/internal/reconcile"
	"github.com/prop4n/proxmops/internal/server"
	"github.com/prop4n/proxmops/internal/source"
	"github.com/prop4n/proxmops/internal/status"
	"github.com/prop4n/proxmops/internal/store"
)

const shutdownTimeout = 10 * time.Second

type App struct {
	cfg config.Config
	log *slog.Logger
}

func New(cfg config.Config, log *slog.Logger) *App {
	return &App{cfg: cfg, log: log}
}

func (a *App) Plan(ctx context.Context) (reconcile.Plan, error) {
	return a.newEngine(nil).Plan(ctx)
}

func (a *App) Run(ctx context.Context, addr string) error {
	st, err := store.Open(a.cfg.Server.DatabasePath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()

	authSvc := auth.New(st, a.log)
	if err := authSvc.Init(ctx); err != nil {
		return fmt.Errorf("init auth: %w", err)
	}

	statusStore := status.NewStore()
	engine := a.newEngine(statusStore)
	srv := server.New(server.Options{
		Addr:         addr,
		Auth:         authSvc,
		Status:       statusStore,
		CookieSecure: a.cfg.Server.CookieSecure,
	}, a.log)

	errc := make(chan error, 2)
	go func() { errc <- srv.Start() }()
	go func() { errc <- engine.Run(ctx, a.cfg.Reconcile.Interval) }()

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

func (a *App) newEngine(statusStore *status.Store) *reconcile.Engine {
	client := proxmox.New(a.cfg.Cluster)
	src := source.New(a.cfg.Source)
	reconcilers := []reconcile.Reconciler{
		reconcile.NewGuestReconciler(client),
		reconcile.NewIsoReconciler(client),
	}
	opts := reconcile.Options{
		AutoSync: a.cfg.Reconcile.AutoSync,
		DryRun:   a.cfg.Reconcile.DryRun,
		Prune:    a.cfg.Reconcile.Prune,
	}
	return reconcile.NewEngine(src, reconcilers, opts, a.log, statusStore)
}
