package reconcile

import (
	"context"

	"github.com/prop4n/proxmops/internal/manifest"
	"github.com/prop4n/proxmops/internal/proxmox"
)

// templateReconciler builds golden templates: it creates a bare VM from an
// image and converts it to a template. Ownership is tag-based like guests, and
// templates are keyed by VMID. Cloning templates into VMs is a later increment.
type templateReconciler struct {
	store proxmox.GuestStore
}

// NewTemplateReconciler returns a Reconciler for templates.
func NewTemplateReconciler(store proxmox.GuestStore) Reconciler {
	return &templateReconciler{store: store}
}

// Plan diffs desired templates against the cluster within the owned scope.
func (t *templateReconciler) Plan(ctx context.Context, desired []manifest.Resource) (Plan, error) {
	want := filterKinds(desired, manifest.KindTemplate)

	observed, err := t.store.ListGuests(ctx)
	if err != nil {
		return Plan{}, err
	}
	observedByVMID := make(map[int]proxmox.Object, len(observed))
	for _, o := range observed {
		observedByVMID[o.VMID] = o
	}

	desiredByVMID := make(map[int]struct{}, len(want))
	var plan Plan

	for _, r := range want {
		tpl, ok := r.(manifest.Template)
		if !ok {
			continue
		}
		desiredByVMID[tpl.Spec.VMID] = struct{}{}
		obs, present := observedByVMID[tpl.Spec.VMID]

		if !present {
			spec := templateSpec(tpl)
			plan.Actions = append(plan.Actions, Action{
				Type:   ActionCreate,
				Kind:   manifest.KindTemplate,
				Name:   tpl.Metadata.Name,
				Reason: "not present in cluster",
				Apply: func(ctx context.Context) error {
					if err := t.store.CreateGuest(ctx, spec); err != nil {
						return err
					}
					return t.store.ConvertToTemplate(ctx, spec.Node, spec.VMID)
				},
			})
			continue
		}
		// A built-but-not-converted VM (an interrupted build) still needs the
		// conversion step; a finished template is in sync.
		if obs.Owned() && !obs.IsTemplate {
			node, id := obs.Node, obs.VMID
			plan.Actions = append(plan.Actions, Action{
				Type:   ActionUpdate,
				Kind:   manifest.KindTemplate,
				Name:   tpl.Metadata.Name,
				Reason: "converting to template",
				Apply:  func(ctx context.Context) error { return t.store.ConvertToTemplate(ctx, node, id) },
			})
		}
	}

	// Deletes: owned templates dropped from the repo.
	for _, o := range observed {
		if !o.IsTemplate || !o.Owned() {
			continue
		}
		if _, ok := desiredByVMID[o.VMID]; ok {
			continue
		}
		plan.Actions = append(plan.Actions, Action{
			Type:   ActionDelete,
			Kind:   manifest.KindTemplate,
			Name:   o.Name,
			Reason: "removed from repository",
			Apply:  func(ctx context.Context) error { return t.store.DeleteGuest(ctx, o) },
		})
	}

	return plan, nil
}

// templateSpec projects a manifest Template onto the flat build spec, adding the
// ownership tag and marking it as a template (bare: no cloud-init, never started).
func templateSpec(tpl manifest.Template) proxmox.GuestSpec {
	tags := append([]string{}, tpl.Metadata.Tags...)
	tags = append(tags, proxmox.ManagedTag)

	spec := proxmox.GuestSpec{
		Kind:       proxmox.KindVirtualMachine,
		Node:       tpl.Metadata.Node,
		VMID:       tpl.Spec.VMID,
		Name:       tpl.Metadata.Name,
		Cores:      tpl.Spec.Cores,
		MemoryMB:   tpl.Spec.Memory,
		CPU:        tpl.Spec.CPU,
		Tags:       tags,
		AsTemplate: true,
	}
	if tpl.Spec.Image != nil {
		spec.Image = &proxmox.GuestImage{
			Source:        tpl.Spec.Image.Source,
			Filename:      tpl.Spec.Image.Filename(),
			ImportStorage: tpl.Spec.Image.ImportStorage,
		}
	}
	if len(tpl.Spec.Disks) > 0 {
		spec.Disk = proxmox.GuestDisk{Storage: tpl.Spec.Disks[0].Storage, Size: tpl.Spec.Disks[0].Size}
	}
	if len(tpl.Spec.Net) > 0 {
		spec.NIC = proxmox.GuestNIC{Bridge: tpl.Spec.Net[0].Bridge, Model: tpl.Spec.Net[0].Model}
	}
	return spec
}
