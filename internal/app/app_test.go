package app

import (
	"context"
	"errors"
	"testing"

	"github.com/prop4n/proxmops/internal/config"
	"github.com/prop4n/proxmops/internal/crypt"
	"github.com/prop4n/proxmops/internal/manifest"
	"github.com/prop4n/proxmops/internal/proxmox"
	"github.com/prop4n/proxmops/internal/server"
	"github.com/prop4n/proxmops/internal/settings"
	"github.com/prop4n/proxmops/internal/store"
)

// fakeIsoStore records ISO deletions.
type fakeIsoStore struct{ deleted []string }

func (f *fakeIsoStore) ListISOs(context.Context, string, string) ([]string, error) {
	return nil, nil
}
func (f *fakeIsoStore) DownloadISO(context.Context, proxmox.IsoDownload) error { return nil }
func (f *fakeIsoStore) DeleteISO(_ context.Context, node, storage, filename string) error {
	f.deleted = append(f.deleted, node+"/"+storage+"/"+filename)
	return nil
}

func isoRes(name, src string) manifest.Iso {
	return manifest.Iso{
		TypeMeta: manifest.TypeMeta{APIVersion: manifest.APIVersion, Kind: manifest.KindIso},
		Metadata: manifest.ObjectMeta{Name: name},
		Spec:     manifest.IsoSpec{Source: src, Node: "pve", Storage: "local"},
	}
}

func TestDeleteManagedDeletesISO(t *testing.T) {
	store := &fakeIsoStore{}
	desired := []manifest.Resource{isoRes("nix", "https://ex/path/nixos.iso?token=x")}

	if err := deleteManaged(context.Background(), desired, store, "Iso", "nix"); err != nil {
		t.Fatalf("deleteManaged: %v", err)
	}
	if len(store.deleted) != 1 || store.deleted[0] != "pve/local/nixos.iso" {
		t.Fatalf("deleted = %v; want [pve/local/nixos.iso]", store.deleted)
	}
}

func TestDeleteManagedNotFound(t *testing.T) {
	desired := []manifest.Resource{isoRes("nix", "https://ex/nixos.iso")}
	err := deleteManaged(context.Background(), desired, &fakeIsoStore{}, "Iso", "missing")
	if !errors.Is(err, server.ErrResourceNotFound) {
		t.Fatalf("err = %v; want ErrResourceNotFound", err)
	}
}

func TestDeleteManagedUnsupportedKind(t *testing.T) {
	vm := manifest.VirtualMachine{
		TypeMeta: manifest.TypeMeta{APIVersion: manifest.APIVersion, Kind: manifest.KindVirtualMachine},
		Metadata: manifest.ObjectMeta{Name: "web", Node: "pve"},
		Spec:     manifest.VirtualMachineSpec{VMID: 100},
	}
	err := deleteManaged(context.Background(), []manifest.Resource{vm}, &fakeIsoStore{}, "VirtualMachine", "web")
	if !errors.Is(err, server.ErrDeleteUnsupported) {
		t.Fatalf("err = %v; want ErrDeleteUnsupported", err)
	}
}

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
