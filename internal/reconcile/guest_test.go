package reconcile

import (
	"context"
	"testing"

	"github.com/prop4n/proxmops/internal/manifest"
	"github.com/prop4n/proxmops/internal/proxmox"
)

// fakeGuestStore is an in-memory GuestStore for tests.
type fakeGuestStore struct {
	guests  []proxmox.Object
	created []proxmox.Object
	deleted []proxmox.Object
}

func (f *fakeGuestStore) ListGuests(context.Context) ([]proxmox.Object, error) {
	return f.guests, nil
}

func (f *fakeGuestStore) CreateGuest(_ context.Context, o proxmox.Object) error {
	f.created = append(f.created, o)
	return nil
}

func (f *fakeGuestStore) DeleteGuest(_ context.Context, o proxmox.Object) error {
	f.deleted = append(f.deleted, o)
	return nil
}

func vmResource(name string) manifest.VirtualMachine {
	return manifest.VirtualMachine{
		TypeMeta: manifest.TypeMeta{APIVersion: manifest.APIVersion, Kind: manifest.KindVirtualMachine},
		Metadata: manifest.ObjectMeta{Name: name, Node: "pve"},
	}
}

func ownedGuest(name string) proxmox.Object {
	return proxmox.Object{Kind: proxmox.KindVirtualMachine, Name: name, Tags: []string{proxmox.ManagedTag}}
}

func TestGuestCreatesMissing(t *testing.T) {
	store := &fakeGuestStore{}
	plan, err := NewGuestReconciler(store).Plan(context.Background(), []manifest.Resource{vmResource("web-01")})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Type != ActionCreate {
		t.Fatalf("want one create, got %+v", plan.Actions)
	}
}

func TestGuestCreateCarriesManagedTag(t *testing.T) {
	store := &fakeGuestStore{}
	plan, _ := NewGuestReconciler(store).Plan(context.Background(), []manifest.Resource{vmResource("web-01")})
	if err := plan.Actions[0].Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.created) != 1 || !store.created[0].Owned() {
		t.Fatalf("created guest must carry the managed tag, got %+v", store.created)
	}
}

func TestGuestPrunesOwnedOrphan(t *testing.T) {
	store := &fakeGuestStore{guests: []proxmox.Object{ownedGuest("old-01")}}
	plan, _ := NewGuestReconciler(store).Plan(context.Background(), nil)
	if len(plan.Actions) != 1 || plan.Actions[0].Type != ActionDelete {
		t.Fatalf("want one delete, got %+v", plan.Actions)
	}
}

func TestGuestIgnoresUnownedOrphan(t *testing.T) {
	unowned := proxmox.Object{Kind: proxmox.KindVirtualMachine, Name: "manual-01"}
	store := &fakeGuestStore{guests: []proxmox.Object{unowned}}
	plan, _ := NewGuestReconciler(store).Plan(context.Background(), nil)
	if !plan.Empty() {
		t.Fatalf("want empty plan, got %+v", plan.Actions)
	}
}

func TestGuestInSyncWhenPresent(t *testing.T) {
	store := &fakeGuestStore{guests: []proxmox.Object{ownedGuest("web-01")}}
	plan, _ := NewGuestReconciler(store).Plan(context.Background(), []manifest.Resource{vmResource("web-01")})
	if !plan.Empty() {
		t.Fatalf("want empty plan, got %+v", plan.Actions)
	}
}
