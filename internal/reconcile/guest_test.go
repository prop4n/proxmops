package reconcile

import (
	"context"
	"slices"
	"testing"

	"github.com/prop4n/proxmops/internal/manifest"
	"github.com/prop4n/proxmops/internal/proxmox"
)

// fakeGuestStore is an in-memory GuestStore for tests.
type fakeGuestStore struct {
	guests    []proxmox.Object
	created   []proxmox.GuestSpec
	updated   []proxmox.GuestUpdate
	deleted   []proxmox.Object
	rebooted  []int
	converted []int
}

func (f *fakeGuestStore) ListGuests(context.Context) ([]proxmox.Object, error) {
	return f.guests, nil
}

func (f *fakeGuestStore) CreateGuest(_ context.Context, s proxmox.GuestSpec) error {
	f.created = append(f.created, s)
	return nil
}

func (f *fakeGuestStore) UpdateGuest(_ context.Context, u proxmox.GuestUpdate) error {
	f.updated = append(f.updated, u)
	return nil
}

func (f *fakeGuestStore) DeleteGuest(_ context.Context, o proxmox.Object) error {
	f.deleted = append(f.deleted, o)
	return nil
}

func (f *fakeGuestStore) RebootGuest(_ context.Context, _ string, vmid int) error {
	f.rebooted = append(f.rebooted, vmid)
	return nil
}

func (f *fakeGuestStore) ConvertToTemplate(_ context.Context, _ string, vmid int) error {
	f.converted = append(f.converted, vmid)
	return nil
}

// vmid derives a stable, non-zero VMID from a name so tests can match desired
// and observed guests by identity without repeating numbers.
func vmid(name string) int {
	n := 0
	for _, r := range name {
		n = n*31 + int(r)
	}
	if n < 0 {
		n = -n
	}
	return 100 + n%9000
}

func vmResource(name string) manifest.VirtualMachine {
	return manifest.VirtualMachine{
		TypeMeta: manifest.TypeMeta{APIVersion: manifest.APIVersion, Kind: manifest.KindVirtualMachine},
		Metadata: manifest.ObjectMeta{Name: name, Node: "pve"},
		Spec:     manifest.VirtualMachineSpec{VMID: vmid(name)},
	}
}

func ownedGuest(name string) proxmox.Object {
	return proxmox.Object{Kind: proxmox.KindVirtualMachine, Name: name, VMID: vmid(name), Node: "pve", Tags: []string{proxmox.ManagedTag}}
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
	if len(store.created) != 1 || !slices.Contains(store.created[0].Tags, proxmox.ManagedTag) {
		t.Fatalf("created guest must carry the managed tag, got %+v", store.created)
	}
}

func TestGuestCreateCarriesCloudInit(t *testing.T) {
	store := &fakeGuestStore{}
	vm := vmResource("web-01")
	vm.Spec.CPU = "x86-64-v2"
	vm.Spec.Disks = []manifest.Disk{{Storage: "local-lvm", Size: "20G"}}
	vm.Spec.Image = &manifest.Image{Source: "https://ex/d/debian-12.qcow2"}
	vm.Spec.CloudInit = &manifest.CloudInit{User: "debian", SSHKeys: []string{"ssh-ed25519 AAAA"}, IP: "dhcp"}

	plan, _ := NewGuestReconciler(store).Plan(context.Background(), []manifest.Resource{vm})
	if err := plan.Actions[0].Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.created) != 1 {
		t.Fatalf("want one create, got %d", len(store.created))
	}
	got := store.created[0]
	if got.CPU != "x86-64-v2" {
		t.Errorf("cpu not propagated: %q", got.CPU)
	}
	if got.Image == nil || got.Image.Filename != "debian-12.qcow2" {
		t.Fatalf("image not propagated: %+v", got.Image)
	}
	if got.CloudInit == nil || got.CloudInit.User != "debian" || got.CloudInit.IP != "dhcp" {
		t.Fatalf("cloud-init not propagated: %+v", got.CloudInit)
	}
	if len(got.CloudInit.SSHKeys) != 1 {
		t.Errorf("ssh keys not propagated: %+v", got.CloudInit.SSHKeys)
	}
}

func TestGuestUpdatesDriftedConfig(t *testing.T) {
	// Owned VM present with 1 core / 512MB; the manifest wants 4 cores / 2048MB.
	obs := ownedGuest("web-01")
	obs.Cores, obs.MemoryMB = 1, 512
	store := &fakeGuestStore{guests: []proxmox.Object{obs}}

	vm := vmResource("web-01")
	vm.Spec.Cores, vm.Spec.Memory = 4, 2048

	plan, _ := NewGuestReconciler(store).Plan(context.Background(), []manifest.Resource{vm})
	if len(plan.Actions) != 1 || plan.Actions[0].Type != ActionUpdate {
		t.Fatalf("want one update, got %+v", plan.Actions)
	}
	if err := plan.Actions[0].Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.updated) != 1 || store.updated[0].Cores != 4 || store.updated[0].MemoryMB != 2048 {
		t.Fatalf("update = %+v, want cores 4 / mem 2048", store.updated)
	}
	if store.updated[0].VMID != vmid("web-01") {
		t.Errorf("update vmid = %d, want %d", store.updated[0].VMID, vmid("web-01"))
	}
}

func TestGuestReportsRebootRequiredByDefault(t *testing.T) {
	// Config already matches desired, but the running VM awaits a restart.
	obs := ownedGuest("web-01")
	obs.Cores, obs.MemoryMB, obs.Running, obs.RebootPending = 4, 2048, true, true
	store := &fakeGuestStore{guests: []proxmox.Object{obs}}

	vm := vmResource("web-01")
	vm.Spec.Cores, vm.Spec.Memory = 4, 2048

	plan, _ := NewGuestReconciler(store).Plan(context.Background(), []manifest.Resource{vm})
	if len(plan.Actions) != 1 || !plan.Actions[0].Informational {
		t.Fatalf("want one informational action, got %+v", plan.Actions)
	}
	// Informational actions are never applied.
	for _, a := range plan.Actions {
		if a.Apply != nil {
			t.Error("informational action must have no Apply")
		}
	}
}

func TestGuestRebootsWhenApplyModeReboot(t *testing.T) {
	obs := ownedGuest("web-01")
	obs.Cores, obs.MemoryMB, obs.Running, obs.RebootPending = 4, 2048, true, true
	store := &fakeGuestStore{guests: []proxmox.Object{obs}}

	vm := vmResource("web-01")
	vm.Spec.Cores, vm.Spec.Memory = 4, 2048
	vm.Spec.ApplyMode = manifest.ApplyModeReboot

	plan, _ := NewGuestReconciler(store).Plan(context.Background(), []manifest.Resource{vm})
	if len(plan.Actions) != 1 || plan.Actions[0].Informational {
		t.Fatalf("want one real reboot action, got %+v", plan.Actions)
	}
	if err := plan.Actions[0].Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.rebooted) != 1 || store.rebooted[0] != vmid("web-01") {
		t.Fatalf("rebooted = %v, want [%d]", store.rebooted, vmid("web-01"))
	}
}

func TestGuestUpdatesDriftedCPU(t *testing.T) {
	obs := ownedGuest("web-01")
	obs.Cores, obs.MemoryMB, obs.CPU = 2, 2048, "kvm64"
	store := &fakeGuestStore{guests: []proxmox.Object{obs}}

	vm := vmResource("web-01")
	vm.Spec.Cores, vm.Spec.Memory, vm.Spec.CPU = 2, 2048, "x86-64-v2"

	plan, _ := NewGuestReconciler(store).Plan(context.Background(), []manifest.Resource{vm})
	if len(plan.Actions) != 1 || plan.Actions[0].Type != ActionUpdate {
		t.Fatalf("want one update, got %+v", plan.Actions)
	}
	if err := plan.Actions[0].Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.updated) != 1 || store.updated[0].CPU != "x86-64-v2" {
		t.Fatalf("update = %+v, want cpu x86-64-v2", store.updated)
	}
}

func TestGuestUpdatesDriftedNameserver(t *testing.T) {
	obs := ownedGuest("web-01")
	obs.CIUser, obs.IP, obs.Nameserver = "debian", "ip=dhcp", "1.1.1.1"
	store := &fakeGuestStore{guests: []proxmox.Object{obs}}

	vm := vmResource("web-01")
	vm.Spec.Image = &manifest.Image{Source: "https://ex/d.qcow2"}
	vm.Spec.CloudInit = &manifest.CloudInit{User: "debian", IP: "dhcp", Nameserver: "1.1.1.2"}

	plan, _ := NewGuestReconciler(store).Plan(context.Background(), []manifest.Resource{vm})
	if len(plan.Actions) != 1 || plan.Actions[0].Type != ActionUpdate {
		t.Fatalf("want one update, got %+v", plan.Actions)
	}
	if err := plan.Actions[0].Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.updated) != 1 || store.updated[0].Nameserver != "1.1.1.2" {
		t.Fatalf("update = %+v, want nameserver 1.1.1.2", store.updated)
	}
}

func TestGuestNoDriftOnMatchingCloudInit(t *testing.T) {
	// "dhcp" desired must match Proxmox's stored "ip=dhcp" (no perpetual drift).
	obs := ownedGuest("web-01")
	obs.CIUser, obs.IP, obs.Nameserver = "debian", "ip=dhcp", "1.1.1.1"
	store := &fakeGuestStore{guests: []proxmox.Object{obs}}

	vm := vmResource("web-01")
	vm.Spec.Image = &manifest.Image{Source: "https://ex/d.qcow2"}
	vm.Spec.CloudInit = &manifest.CloudInit{User: "debian", IP: "dhcp", Nameserver: "1.1.1.1"}

	plan, _ := NewGuestReconciler(store).Plan(context.Background(), []manifest.Resource{vm})
	if !plan.Empty() {
		t.Fatalf("want empty plan, got %+v", plan.Actions)
	}
}

func TestGuestIgnoresTemplates(t *testing.T) {
	// An owned template must not be pruned by the guest reconciler even when no
	// VM manifest references its vmid.
	tpl := ownedGuest("deb-tpl")
	tpl.IsTemplate = true
	store := &fakeGuestStore{guests: []proxmox.Object{tpl}}
	plan, _ := NewGuestReconciler(store).Plan(context.Background(), nil)
	if !plan.Empty() {
		t.Fatalf("guest reconciler must ignore templates, got %+v", plan.Actions)
	}
}

func TestGuestNoUpdateWhenConfigMatches(t *testing.T) {
	obs := ownedGuest("web-01")
	obs.Cores, obs.MemoryMB = 2, 1024
	store := &fakeGuestStore{guests: []proxmox.Object{obs}}

	vm := vmResource("web-01")
	vm.Spec.Cores, vm.Spec.Memory = 2, 1024

	plan, _ := NewGuestReconciler(store).Plan(context.Background(), []manifest.Resource{vm})
	if !plan.Empty() {
		t.Fatalf("want empty plan, got %+v", plan.Actions)
	}
}

func TestGuestDoesNotUpdateUnownedDrift(t *testing.T) {
	// A hand-made VM (no ownership tag) that differs must be left untouched.
	unowned := proxmox.Object{Kind: proxmox.KindVirtualMachine, Name: "web-01", VMID: vmid("web-01"), Cores: 1}
	store := &fakeGuestStore{guests: []proxmox.Object{unowned}}

	vm := vmResource("web-01")
	vm.Spec.Cores = 8

	plan, _ := NewGuestReconciler(store).Plan(context.Background(), []manifest.Resource{vm})
	if !plan.Empty() {
		t.Fatalf("want empty plan for unowned guest, got %+v", plan.Actions)
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
