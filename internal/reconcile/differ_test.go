package reconcile

import (
	"testing"

	"github.com/prop4n/proxmops/internal/manifest"
	"github.com/prop4n/proxmops/internal/proxmox"
)

func vm(name string) manifest.VirtualMachine {
	return manifest.VirtualMachine{
		TypeMeta: manifest.TypeMeta{APIVersion: manifest.APIVersion, Kind: manifest.KindVirtualMachine},
		Metadata: manifest.ObjectMeta{Name: name, Node: "pve"},
	}
}

func ownedVM(name string) proxmox.Object {
	return proxmox.Object{
		Kind: proxmox.Kind(manifest.KindVirtualMachine),
		Name: name,
		Tags: []string{proxmox.ManagedTag},
	}
}

func TestDiffCreatesMissing(t *testing.T) {
	plan := Diff([]manifest.Resource{vm("web-01")}, nil)
	if len(plan.Actions) != 1 || plan.Actions[0].Type != ActionCreate {
		t.Fatalf("want one create, got %+v", plan.Actions)
	}
}

func TestDiffPrunesOwnedOrphan(t *testing.T) {
	plan := Diff(nil, []proxmox.Object{ownedVM("old-01")})
	if len(plan.Actions) != 1 || plan.Actions[0].Type != ActionDelete {
		t.Fatalf("want one delete, got %+v", plan.Actions)
	}
}

func TestDiffIgnoresUnownedOrphan(t *testing.T) {
	unowned := proxmox.Object{Kind: proxmox.Kind(manifest.KindVirtualMachine), Name: "manual-01"}
	plan := Diff(nil, []proxmox.Object{unowned})
	if !plan.Empty() {
		t.Fatalf("want empty plan, got %+v", plan.Actions)
	}
}

func TestDiffOwnedPresentIsInSync(t *testing.T) {
	plan := Diff([]manifest.Resource{vm("web-01")}, []proxmox.Object{ownedVM("web-01")})
	if !plan.Empty() {
		t.Fatalf("want empty plan, got %+v", plan.Actions)
	}
}
