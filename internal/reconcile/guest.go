package reconcile

import (
	"context"

	"github.com/prop4n/proxmops/internal/manifest"
	"github.com/prop4n/proxmops/internal/proxmox"
)

// guestReconciler reconciles QEMU guests and LXC containers using tag-based
// ownership: only guests carrying proxmox.ManagedTag are updated or deleted, so
// hand-made guests are left untouched.
type guestReconciler struct {
	store proxmox.GuestStore
}

// NewGuestReconciler returns a Reconciler for VMs and containers.
func NewGuestReconciler(store proxmox.GuestStore) Reconciler {
	return &guestReconciler{store: store}
}

// Plan diffs desired guests against the cluster within the owned scope.
func (g *guestReconciler) Plan(ctx context.Context, desired []manifest.Resource) (Plan, error) {
	want := filterKinds(desired, manifest.KindVirtualMachine, manifest.KindContainer)

	observed, err := g.store.ListGuests(ctx)
	if err != nil {
		return Plan{}, err
	}

	desiredByKey := make(map[objectKey]proxmox.Object, len(want))
	order := make([]objectKey, 0, len(want))
	for _, r := range want {
		obj := desiredObject(r)
		k := keyOf(obj)
		desiredByKey[k] = obj
		order = append(order, k)
	}

	observedByKey := make(map[objectKey]proxmox.Object, len(observed))
	for _, o := range observed {
		observedByKey[keyOf(o)] = o
	}

	var plan Plan

	// Creates, in manifest order. A present guest is treated as in sync for
	// now; spec comparison arrives with the VM phase.
	for _, k := range order {
		if _, ok := observedByKey[k]; ok {
			continue
		}
		obj := desiredByKey[k]
		plan.Actions = append(plan.Actions, Action{
			Type:   ActionCreate,
			Kind:   manifest.Kind(obj.Kind),
			Name:   obj.Name,
			Reason: "not present in cluster",
			Apply:  func(ctx context.Context) error { return g.store.CreateGuest(ctx, obj) },
		})
	}

	// Deletes: owned guests absent from the desired set.
	for _, o := range observed {
		if !o.Owned() {
			continue
		}
		if _, ok := desiredByKey[keyOf(o)]; ok {
			continue
		}
		plan.Actions = append(plan.Actions, Action{
			Type:   ActionDelete,
			Kind:   manifest.Kind(o.Kind),
			Name:   o.Name,
			Reason: "removed from repository",
			Apply:  func(ctx context.Context) error { return g.store.DeleteGuest(ctx, o) },
		})
	}

	return plan, nil
}

// objectKey uniquely identifies a guest across the desired and observed sets.
type objectKey struct {
	kind proxmox.Kind
	name string
}

func keyOf(o proxmox.Object) objectKey {
	return objectKey{kind: o.Kind, name: o.Name}
}

// desiredObject projects a manifest guest onto the observed-state shape, adding
// the ownership tag that a created guest must carry.
func desiredObject(r manifest.Resource) proxmox.Object {
	om := r.GetObjectMeta()
	tags := append([]string{}, om.Tags...)
	tags = append(tags, proxmox.ManagedTag)
	return proxmox.Object{
		Kind: proxmox.Kind(r.GetTypeMeta().Kind),
		Name: om.Name,
		Node: om.Node,
		Tags: tags,
	}
}
