package reconcile

import (
	"context"
	"testing"

	"github.com/prop4n/proxmops/internal/manifest"
	"github.com/prop4n/proxmops/internal/proxmox"
)

// fakeIsoStore is an in-memory IsoStore for tests.
type fakeIsoStore struct {
	present   map[string][]string // keyed by "node/storage"
	downloads []proxmox.IsoDownload
}

func (f *fakeIsoStore) ListISOs(_ context.Context, node, storage string) ([]string, error) {
	return f.present[node+"/"+storage], nil
}

func (f *fakeIsoStore) DownloadISO(_ context.Context, req proxmox.IsoDownload) error {
	f.downloads = append(f.downloads, req)
	return nil
}

func isoResource(name, source string) manifest.Iso {
	return manifest.Iso{
		TypeMeta: manifest.TypeMeta{APIVersion: manifest.APIVersion, Kind: manifest.KindIso},
		Metadata: manifest.ObjectMeta{Name: name},
		Spec: manifest.IsoSpec{
			Source:   source,
			Node:     "pve",
			Storage:  "local",
			Checksum: manifest.Checksum{Algo: "sha256", Value: "abc"},
		},
	}
}

func TestIsoDownloadsMissing(t *testing.T) {
	store := &fakeIsoStore{present: map[string][]string{}}
	iso := isoResource("debian-12", "https://example.com/debian-12.iso")

	plan, err := NewIsoReconciler(store).Plan(context.Background(), []manifest.Resource{iso})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Type != ActionCreate {
		t.Fatalf("want one create, got %+v", plan.Actions)
	}

	if err := plan.Actions[0].Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.downloads) != 1 || store.downloads[0].Filename != "debian-12.iso" {
		t.Fatalf("want download of debian-12.iso, got %+v", store.downloads)
	}
	if store.downloads[0].Checksum != "abc" {
		t.Errorf("checksum not propagated: %+v", store.downloads[0])
	}
}

func TestIsoInSyncWhenPresent(t *testing.T) {
	store := &fakeIsoStore{present: map[string][]string{"pve/local": {"debian-12.iso"}}}
	iso := isoResource("debian-12", "https://example.com/debian-12.iso")

	plan, _ := NewIsoReconciler(store).Plan(context.Background(), []manifest.Resource{iso})
	if !plan.Empty() {
		t.Fatalf("want empty plan, got %+v", plan.Actions)
	}
}

func TestIsoNeverDeletes(t *testing.T) {
	// An extra ISO on the storage that no manifest declares must be left alone.
	store := &fakeIsoStore{present: map[string][]string{"pve/local": {"stale.iso"}}}
	iso := isoResource("debian-12", "https://example.com/debian-12.iso")

	plan, _ := NewIsoReconciler(store).Plan(context.Background(), []manifest.Resource{iso})
	for _, a := range plan.Actions {
		if a.Type == ActionDelete {
			t.Fatalf("iso reconciler must never delete, got %+v", a)
		}
	}
}
