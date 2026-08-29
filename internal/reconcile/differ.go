package reconcile

import (
	"github.com/prop4n/proxmops/internal/manifest"
	"github.com/prop4n/proxmops/internal/proxmox"
)

// Diff computes the plan that reconciles observed cluster state toward the
// desired manifests. Ownership rules apply: unowned observed resources are
// never scheduled for update or deletion, so hand-made resources are left
// untouched.
func Diff(desired []manifest.Resource, observed []proxmox.Object) Plan {
	desiredByKey := make(map[objectKey]proxmox.Object, len(desired))
	order := make([]objectKey, 0, len(desired))
	for _, r := range desired {
		obj := toObject(r)
		k := keyOf(obj)
		desiredByKey[k] = obj
		order = append(order, k)
	}

	observedByKey := make(map[objectKey]proxmox.Object, len(observed))
	for _, o := range observed {
		observedByKey[keyOf(o)] = o
	}

	var plan Plan

	// Creates, in manifest order. A present, owned resource is considered in
	// sync for now; real spec comparison lands with the API client.
	for _, k := range order {
		if _, ok := observedByKey[k]; ok {
			continue
		}
		plan.Actions = append(plan.Actions, Action{
			Type:   ActionCreate,
			Object: desiredByKey[k],
			Reason: "not present in cluster",
		})
	}

	// Deletes: owned observed resources that are absent from the desired set.
	for _, o := range observed {
		if !o.Owned() {
			continue
		}
		if _, ok := desiredByKey[keyOf(o)]; !ok {
			plan.Actions = append(plan.Actions, Action{
				Type:   ActionDelete,
				Object: o,
				Reason: "removed from repository",
			})
		}
	}

	return plan
}

// objectKey uniquely identifies a resource across the desired and observed sets.
type objectKey struct {
	kind proxmox.Kind
	name string
}

func keyOf(o proxmox.Object) objectKey {
	return objectKey{kind: o.Kind, name: o.Name}
}

// toObject projects a manifest resource onto the observed-state shape used for
// diffing.
func toObject(r manifest.Resource) proxmox.Object {
	tm := r.GetTypeMeta()
	om := r.GetObjectMeta()
	return proxmox.Object{
		Kind: proxmox.Kind(tm.Kind),
		Name: om.Name,
		Node: om.Node,
		Tags: om.Tags,
	}
}
