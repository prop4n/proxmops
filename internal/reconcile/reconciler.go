package reconcile

import (
	"context"
	"slices"

	"github.com/prop4n/proxmops/internal/manifest"
)

// Reconciler plans the changes for one resource family. It receives the full
// set of desired resources and filters the kinds it owns, so the Engine stays
// agnostic of how any family maps to the cluster.
type Reconciler interface {
	// Plan compares the desired resources it owns against the cluster and
	// returns the actions needed to converge.
	Plan(ctx context.Context, desired []manifest.Resource) (Plan, error)
}

// filterKinds returns the desired resources whose kind is in kinds.
func filterKinds(desired []manifest.Resource, kinds ...manifest.Kind) []manifest.Resource {
	var out []manifest.Resource
	for _, r := range desired {
		if slices.Contains(kinds, r.GetTypeMeta().Kind) {
			out = append(out, r)
		}
	}
	return out
}
