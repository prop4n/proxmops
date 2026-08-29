// Package proxmox is the client for the Proxmox VE API. It defines the
// capability interfaces the reconcilers depend on, the data types that
// represent observed cluster state, and an adapter over
// github.com/luthermonson/go-proxmox.
package proxmox

import (
	"context"
	"errors"
	"slices"
)

// ErrNotImplemented is returned by operations that are not built yet.
var ErrNotImplemented = errors.New("proxmox: not implemented")

// ManagedTag marks the guests proxmops owns. Only tagged guests are ever
// modified or deleted.
const ManagedTag = "managed-by:proxmops"

// Kind identifies the type of an observed resource. The values mirror
// manifest.Kind so desired and observed objects compare directly, without this
// package importing the manifest package.
type Kind string

// Observed resource kinds.
const (
	KindVirtualMachine Kind = "VirtualMachine"
	KindContainer      Kind = "Container"
	KindIso            Kind = "Iso"
	KindNetwork        Kind = "Network"
)

// Object is the observed state of a guest (VM or container).
type Object struct {
	Kind Kind
	Name string
	Node string
	ID   string
	Tags []string
}

// Owned reports whether the object carries the proxmops ownership tag.
func (o Object) Owned() bool {
	return slices.Contains(o.Tags, ManagedTag)
}

// GuestStore reads and mutates QEMU guests and LXC containers. Ownership is
// tracked with ManagedTag, so untagged guests are never mutated.
type GuestStore interface {
	// ListGuests returns every VM and container visible in the cluster.
	ListGuests(ctx context.Context) ([]Object, error)
	// CreateGuest provisions a guest to match obj.
	CreateGuest(ctx context.Context, obj Object) error
	// DeleteGuest removes a guest proxmops owns.
	DeleteGuest(ctx context.Context, obj Object) error
}

// IsoDownload describes an ISO to fetch onto a storage.
type IsoDownload struct {
	Node         string
	Storage      string
	Filename     string
	URL          string
	Checksum     string
	ChecksumAlgo string
}

// IsoStore reads and downloads ISO images. ISOs carry no ownership tag, so this
// store never deletes: reconciliation is download-if-absent only.
type IsoStore interface {
	// ListISOs returns the filenames of the ISOs present on node/storage.
	ListISOs(ctx context.Context, node, storage string) ([]string, error)
	// DownloadISO fetches an ISO onto a storage, verifying its checksum.
	DownloadISO(ctx context.Context, req IsoDownload) error
}
