package reconcile

import (
	"context"
	"testing"

	"github.com/prop4n/proxmops/internal/manifest"
	"github.com/prop4n/proxmops/internal/proxmox"
)

func tplResource(name string, vmid int) manifest.Template {
	return manifest.Template{
		TypeMeta: manifest.TypeMeta{APIVersion: manifest.APIVersion, Kind: manifest.KindTemplate},
		Metadata: manifest.ObjectMeta{Name: name, Node: "pve"},
		Spec: manifest.TemplateSpec{
			VMID:  vmid,
			Image: &manifest.Image{Source: "https://ex/d/debian-12.qcow2"},
			Disks: []manifest.Disk{{Storage: "local-lvm", Size: "10G"}},
		},
	}
}

func ownedTemplate(name string, vmid int) proxmox.Object {
	return proxmox.Object{
		Kind: proxmox.KindVirtualMachine, Name: name, VMID: vmid, Node: "pve",
		IsTemplate: true, Tags: []string{proxmox.ManagedTag},
	}
}

func TestTemplateCarriesDiskBus(t *testing.T) {
	store := &fakeGuestStore{}
	tpl := tplResource("deb", 9000)
	tpl.Spec.Disks = []manifest.Disk{{Storage: "local-lvm", Size: "10G", Bus: "virtio"}}
	plan, _ := NewTemplateReconciler(store).Plan(context.Background(), []manifest.Resource{tpl})
	if err := plan.Actions[0].Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.created[0].Disk.Bus != "virtio" {
		t.Fatalf("disk bus not propagated: %q", store.created[0].Disk.Bus)
	}
}

func TestTemplateBuildsAndConverts(t *testing.T) {
	store := &fakeGuestStore{}
	plan, err := NewTemplateReconciler(store).Plan(context.Background(), []manifest.Resource{tplResource("deb", 9000)})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Type != ActionCreate {
		t.Fatalf("want one create, got %+v", plan.Actions)
	}
	if err := plan.Actions[0].Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.created) != 1 || !store.created[0].AsTemplate {
		t.Fatalf("template must be built AsTemplate, got %+v", store.created)
	}
	if len(store.converted) != 1 || store.converted[0] != 9000 {
		t.Fatalf("converted = %v, want [9000]", store.converted)
	}
}

func TestTemplateConvertsInterruptedBuild(t *testing.T) {
	// An owned VM at the template's vmid that is NOT yet a template: finish it.
	obs := proxmox.Object{Kind: proxmox.KindVirtualMachine, Name: "deb", VMID: 9000, Node: "pve", Tags: []string{proxmox.ManagedTag}}
	store := &fakeGuestStore{guests: []proxmox.Object{obs}}

	plan, _ := NewTemplateReconciler(store).Plan(context.Background(), []manifest.Resource{tplResource("deb", 9000)})
	if len(plan.Actions) != 1 || plan.Actions[0].Type != ActionUpdate {
		t.Fatalf("want one convert action, got %+v", plan.Actions)
	}
	if err := plan.Actions[0].Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.converted) != 1 || store.converted[0] != 9000 {
		t.Fatalf("converted = %v, want [9000]", store.converted)
	}
}

func TestTemplateInSyncWhenPresent(t *testing.T) {
	store := &fakeGuestStore{guests: []proxmox.Object{ownedTemplate("deb", 9000)}}
	plan, _ := NewTemplateReconciler(store).Plan(context.Background(), []manifest.Resource{tplResource("deb", 9000)})
	if !plan.Empty() {
		t.Fatalf("want empty plan, got %+v", plan.Actions)
	}
}

func TestTemplatePrunesOwnedOrphan(t *testing.T) {
	store := &fakeGuestStore{guests: []proxmox.Object{ownedTemplate("old", 9000)}}
	plan, _ := NewTemplateReconciler(store).Plan(context.Background(), nil)
	if len(plan.Actions) != 1 || plan.Actions[0].Type != ActionDelete {
		t.Fatalf("want one delete, got %+v", plan.Actions)
	}
}

func TestTemplateIgnoresPlainVM(t *testing.T) {
	// A regular owned VM (not a template) must not be touched by this reconciler.
	vm := ownedGuest("web")
	store := &fakeGuestStore{guests: []proxmox.Object{vm}}
	plan, _ := NewTemplateReconciler(store).Plan(context.Background(), nil)
	if !plan.Empty() {
		t.Fatalf("template reconciler must ignore plain VMs, got %+v", plan.Actions)
	}
}
