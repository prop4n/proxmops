package settings

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/prop4n/proxmops/internal/crypt"
	"github.com/prop4n/proxmops/internal/store"
)

// testKey returns a throwaway encryption key.
func testKey(t *testing.T) crypt.Key {
	t.Helper()
	key, err := crypt.LoadOrCreateKey(filepath.Join(t.TempDir(), "key"))
	if err != nil {
		t.Fatalf("test key: %v", err)
	}
	return key
}

// fakeRepo is an in-memory Repository.
type fakeRepo struct {
	data []byte
	err  error // returned by LoadSettings
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

func testService(t *testing.T) (*Service, *fakeRepo) {
	t.Helper()
	repo := &fakeRepo{}
	return New(repo, testKey(t)), repo
}

func sampleSettings() Settings {
	return Settings{
		Cluster: ClusterSettings{
			Endpoint:    "https://pve:8006/api2/json",
			TokenID:     "proxmops@pve!gitops",
			TokenSecret: "cluster-secret",
		},
		Source: SourceSettings{
			RepoURL:  "https://github.com/me/homelab.git",
			Path:     "proxmox",
			Revision: "main",
			Token:    "git-token",
		},
		Reconcile: ReconcileSettings{IntervalSeconds: 30, AutoSync: true, Prune: true},
	}
}

func TestSaveGetRoundtrip(t *testing.T) {
	svc, _ := testService(t)
	ctx := context.Background()

	want := sampleSettings()
	if err := svc.Save(ctx, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := svc.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Cluster.Endpoint != want.Cluster.Endpoint ||
		got.Cluster.TokenID != want.Cluster.TokenID ||
		got.Cluster.TokenSecret != want.Cluster.TokenSecret ||
		got.Cluster.InsecureSkipVerify != want.Cluster.InsecureSkipVerify {
		t.Errorf("cluster = %+v, want %+v", got.Cluster, want.Cluster)
	}
	if got.Source != want.Source {
		t.Errorf("source = %+v, want %+v", got.Source, want.Source)
	}
	if got.Reconcile != want.Reconcile {
		t.Errorf("reconcile = %+v, want %+v", got.Reconcile, want.Reconcile)
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt not set")
	}
}

func TestSavedBlobHasNoClearTextSecrets(t *testing.T) {
	svc, repo := testService(t)

	if err := svc.Save(context.Background(), sampleSettings()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	blob := string(repo.data)
	if strings.Contains(blob, "cluster-secret") || strings.Contains(blob, "git-token") {
		t.Fatal("clear-text secret found in stored blob")
	}
}

func TestGetNotConfigured(t *testing.T) {
	svc, _ := testService(t)
	if _, err := svc.Get(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("Get err = %v, want ErrNotConfigured", err)
	}
}

func TestGetPropagatesStoreErrors(t *testing.T) {
	repo := &fakeRepo{err: errors.New("boom")}
	svc := New(repo, testKey(t))
	if _, err := svc.Get(context.Background()); err == nil || errors.Is(err, ErrNotConfigured) {
		t.Fatalf("Get err = %v, want propagated error", err)
	}
}

func TestGetPropagatesStoreNotFound(t *testing.T) {
	repo := &fakeRepo{err: store.ErrNotFound}
	svc := New(repo, testKey(t))
	if _, err := svc.Get(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("Get err = %v, want ErrNotConfigured", err)
	}
}

func TestSaveRejectsInvalid(t *testing.T) {
	svc, _ := testService(t)
	st := sampleSettings()
	st.Cluster.Endpoint = ""
	if err := svc.Save(context.Background(), st); err == nil {
		t.Fatal("Save with missing endpoint succeeded")
	}
}

func TestMaskedHidesSecrets(t *testing.T) {
	st := sampleSettings()
	m := st.Masked()
	if !m.Configured || !m.Cluster.TokenSecretSet || !m.Source.TokenSet {
		t.Fatalf("masked flags wrong: %+v", m)
	}
	if m.Cluster.TokenSecret != "" || m.Source.Token != "" {
		t.Fatal("masked view leaks secrets")
	}
}

func TestMaskedSettingsKeepsStoredSecrets(t *testing.T) {
	current := sampleSettings()
	update := current.Masked()

	merged := update.Settings(current)
	if merged.Cluster.TokenSecret != current.Cluster.TokenSecret {
		t.Error("cluster secret not kept")
	}
	if merged.Source.Token != current.Source.Token {
		t.Error("git token not kept")
	}
	if merged.Cluster.Endpoint != current.Cluster.Endpoint {
		t.Error("endpoint not updated")
	}
}

func TestMaskedSettingsAcceptsNewSecrets(t *testing.T) {
	current := sampleSettings()
	update := current.Masked()
	update.Cluster.TokenSecret = "new-secret"
	update.Source.Token = "new-token"

	merged := update.Settings(current)
	if merged.Cluster.TokenSecret != "new-secret" || merged.Source.Token != "new-token" {
		t.Errorf("secrets not replaced: %+v", merged)
	}
}

func TestConfigFillsDefaults(t *testing.T) {
	st := Settings{
		Cluster:   ClusterSettings{Endpoint: "https://pve:8006/api2/json", TokenID: "id", TokenSecret: "secret"},
		Source:    SourceSettings{RepoURL: "https://example.com/repo.git"},
		Reconcile: ReconcileSettings{},
	}
	cfg := st.Config()
	if cfg.Source.Revision != "main" {
		t.Errorf("revision = %q, want main", cfg.Source.Revision)
	}
	if cfg.Reconcile.Interval != time.Minute {
		t.Errorf("interval = %v, want 1m", cfg.Reconcile.Interval)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}
