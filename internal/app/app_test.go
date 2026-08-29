package app

import (
	"context"
	"errors"
	"testing"

	"github.com/prop4n/proxmops/internal/config"
	"github.com/prop4n/proxmops/internal/crypt"
	"github.com/prop4n/proxmops/internal/settings"
	"github.com/prop4n/proxmops/internal/store"
)

// In-memory settings.Repository; nil data means "not configured".
type fakeRepo struct {
	data []byte
	err  error
}

func (f *fakeRepo) LoadSettings(context.Context) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.data == nil {
		return nil, store.ErrNotFound
	}
	return f.data, nil
}

func (f *fakeRepo) SaveSettings(_ context.Context, data []byte) error {
	f.data = data
	return nil
}

func fileConfig() config.Config {
	return config.Config{
		Cluster: config.Cluster{Endpoint: "https://file", TokenID: "file@pve", TokenSecret: "sec"},
		Source:  config.Source{RepoURL: "https://file/repo"},
	}
}

func TestEffectiveConfigFallsBackToFileWhenUnconfigured(t *testing.T) {
	repo := &fakeRepo{} // no data saved → not configured
	a := &App{cfg: fileConfig(), set: settings.New(repo, crypt.Key{})}

	got, err := a.effectiveConfig(context.Background())
	if err != nil {
		t.Fatalf("effectiveConfig: unexpected err %v", err)
	}
	if got.Cluster.Endpoint != "https://file" {
		t.Fatalf("endpoint = %q; want the file fallback", got.Cluster.Endpoint)
	}
}

func TestEffectiveConfigReportsReadError(t *testing.T) {
	boom := errors.New("boom")
	repo := &fakeRepo{err: boom}
	a := &App{cfg: fileConfig(), set: settings.New(repo, crypt.Key{})}

	got, err := a.effectiveConfig(context.Background())
	if !errors.Is(err, boom) {
		t.Fatalf("effectiveConfig err = %v; want boom surfaced", err)
	}
	// A read error still falls back to the file config, not a blank target.
	if got.Cluster.Endpoint != "https://file" {
		t.Fatalf("endpoint = %q; want the file fallback on error", got.Cluster.Endpoint)
	}
}

func TestEffectiveConfigPrefersSavedSettings(t *testing.T) {
	repo := &fakeRepo{}
	set := settings.New(repo, crypt.Key{})
	if err := set.Save(context.Background(), settings.Settings{
		Cluster: settings.ClusterSettings{Endpoint: "https://saved", TokenID: "saved@pve", TokenSecret: "s"},
		Source:  settings.SourceSettings{RepoURL: "https://saved/repo"},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	a := &App{cfg: fileConfig(), set: set}
	got, err := a.effectiveConfig(context.Background())
	if err != nil {
		t.Fatalf("effectiveConfig: %v", err)
	}
	if got.Cluster.Endpoint != "https://saved" {
		t.Fatalf("endpoint = %q; want the saved settings to win", got.Cluster.Endpoint)
	}
}
