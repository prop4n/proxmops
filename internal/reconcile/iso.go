package reconcile

import (
	"context"
	"path"
	"slices"

	"github.com/prop4n/proxmops/internal/manifest"
	"github.com/prop4n/proxmops/internal/proxmox"
)

// isoReconciler reconciles ISO images conservatively. ISOs carry no ownership
// tag, so it only downloads images that are absent; it never deletes.
type isoReconciler struct {
	store proxmox.IsoStore
}

// NewIsoReconciler returns a Reconciler for ISO images.
func NewIsoReconciler(store proxmox.IsoStore) Reconciler {
	return &isoReconciler{store: store}
}

// Plan downloads desired ISOs that are missing from their target storage.
func (r *isoReconciler) Plan(ctx context.Context, desired []manifest.Resource) (Plan, error) {
	want := filterKinds(desired, manifest.KindIso)

	// Cache the content listing per node/storage to avoid repeat calls.
	present := make(map[storageKey][]string)

	var plan Plan
	for _, res := range want {
		iso, ok := res.(manifest.Iso)
		if !ok {
			continue
		}
		key := storageKey{node: iso.Spec.Node, storage: iso.Spec.Storage}
		names, ok := present[key]
		if !ok {
			var err error
			names, err = r.store.ListISOs(ctx, key.node, key.storage)
			if err != nil {
				return Plan{}, err
			}
			present[key] = names
		}

		filename := isoFilename(iso.Spec.Source)
		if slices.Contains(names, filename) {
			continue
		}

		req := proxmox.IsoDownload{
			Node:         iso.Spec.Node,
			Storage:      iso.Spec.Storage,
			Filename:     filename,
			URL:          iso.Spec.Source,
			Checksum:     iso.Spec.Checksum.Value,
			ChecksumAlgo: iso.Spec.Checksum.Algo,
		}
		plan.Actions = append(plan.Actions, Action{
			Type:   ActionCreate,
			Kind:   manifest.KindIso,
			Name:   iso.Metadata.Name,
			Reason: "not present on storage",
			Apply:  func(ctx context.Context) error { return r.store.DownloadISO(ctx, req) },
		})
	}
	return plan, nil
}

// storageKey identifies a storage on a node.
type storageKey struct {
	node    string
	storage string
}

// isoFilename derives the storage filename from a source URL.
func isoFilename(source string) string {
	return path.Base(source)
}
