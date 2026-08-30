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

// fakeDeleter records ISO and guest deletions.
type fakeDeleter struct {
	deletedISOs   []string
	deletedGuests []int
}

func (f *fakeDeleter) DeleteISO(_ context.Context, node, storage, filename string) error {
	f.deletedISOs = append(f.deletedISOs, node+"/"+storage+"/"+filename)
	return nil
}

func (f *fakeDeleter) DeleteGuest(_ context.Context, obj proxmox.Object) error {
	f.deletedGuests = append(f.deletedGuests, obj.VMID)
	return nil
}

func isoRes(name, src string) manifest.Iso {
	return manifest.Iso{
		TypeMeta: manifest.TypeMeta{APIVersion: manifest.APIVersion, Kind: manifest.KindIso},
		Metadata: manifest.ObjectMeta{Name: name},
		Spec:     manifest.IsoSpec{Source: src, Node: "pve", Storage: "local"},
	}
}

func vmRes(name string, vmid int) manifest.VirtualMachine {
	return manifest.VirtualMachine{
		TypeMeta: manifest.TypeMeta{APIVersion: manifest.APIVersion, Kind: manifest.KindVirtualMachine},
		Metadata: manifest.ObjectMeta{Name: name, Node: "pve"},
		Spec:     manifest.VirtualMachineSpec{VMID: vmid},
	}
}

// fakeObserver implements clusterObserver for detail tests.
type fakeObserver struct {
	guests []proxmox.Object
	isos   map[string][]string
}

func (f *fakeObserver) ListGuests(context.Context) ([]proxmox.Object, error) { return f.guests, nil }
func (f *fakeObserver) ListISOs(_ context.Context, node, storage string) ([]string, error) {
	return f.isos[node+"/"+storage], nil
}

func TestObserveGuestByVMID(t *testing.T) {
	obs := &fakeObserver{guests: []proxmox.Object{
		{Kind: proxmox.KindVirtualMachine, VMID: 101, Cores: 4, MemoryMB: 2048, CPU: "host", Running: true},
	}}
	got := observeResource(context.Background(), obs, vmRes("web", 101))
	if got == nil || !got.Present || got.Cores != 4 || got.MemoryMB != 2048 || !got.Running {
		t.Fatalf("observed = %+v, want present 4c/2048/running", got)
	}
}

func TestObserveGuestAbsent(t *testing.T) {
	got := observeResource(context.Background(), &fakeObserver{}, vmRes("web", 999))
	if got == nil || got.Present {
		t.Fatalf("observed = %+v, want present=false", got)
	}
}

func TestObserveISOPresence(t *testing.T) {
	obs := &fakeObserver{isos: map[string][]string{"pve/local": {"nixos.iso"}}}
	got := observeResource(context.Background(), obs, isoRes("nix", "https://ex/nixos.iso"))
	if got == nil || !got.Present {
		t.Fatalf("observed = %+v, want present ISO", got)
	}
}

func TestDeleteManagedDeletesISO(t *testing.T) {
	del := &fakeDeleter{}
	desired := []manifest.Resource{isoRes("nix", "https://ex/path/nixos.iso?token=x")}

	if err := deleteManaged(context.Background(), desired, del, "Iso", "nix"); err != nil {
		t.Fatalf("deleteManaged: %v", err)
	}
	if len(del.deletedISOs) != 1 || del.deletedISOs[0] != "pve/local/nixos.iso" {
		t.Fatalf("deleted = %v; want [pve/local/nixos.iso]", del.deletedISOs)
	}
}

func TestDeleteManagedDeletesVM(t *testing.T) {
	del := &fakeDeleter{}
	desired := []manifest.Resource{vmRes("web", 101)}

	if err := deleteManaged(context.Background(), desired, del, "VirtualMachine", "web"); err != nil {
		t.Fatalf("deleteManaged: %v", err)
	}
	if len(del.deletedGuests) != 1 || del.deletedGuests[0] != 101 {
		t.Fatalf("deleted guests = %v; want [101]", del.deletedGuests)
	}
}

func TestDeleteManagedNotFound(t *testing.T) {
	desired := []manifest.Resource{isoRes("nix", "https://ex/nixos.iso")}
	err := deleteManaged(context.Background(), desired, &fakeDeleter{}, "Iso", "missing")
	if !errors.Is(err, server.ErrResourceNotFound) {
		t.Fatalf("err = %v; want ErrResourceNotFound", err)
	}
}

func TestDeleteManagedUnsupportedKind(t *testing.T) {
	net := manifest.Network{
		TypeMeta: manifest.TypeMeta{APIVersion: manifest.APIVersion, Kind: manifest.KindNetwork},
		Metadata: manifest.ObjectMeta{Name: "vmbr0", Node: "pve"},
	}
	err := deleteManaged(context.Background(), []manifest.Resource{net}, &fakeDeleter{}, "Network", "vmbr0")
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
	repo := &fakeRepo{} // no data saved -> not configured
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
