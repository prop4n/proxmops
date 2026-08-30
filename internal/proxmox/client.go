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
// modified or deleted. Proxmox tags forbid ':', so the marker uses hyphens.
const ManagedTag = "managed-by-proxmops"

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

// Object is the observed state of a guest (VM or container). Cores, MemoryMB and
// Running come from the cluster resource listing and back drift detection.
type Object struct {
	Kind     Kind
	Name     string
	Node     string
	ID       string
	VMID     int
	Cores    int
	MemoryMB int
	Running  bool
	Tags     []string
}

// Owned reports whether the object carries the proxmops ownership tag.
func (o Object) Owned() bool {
	return slices.Contains(o.Tags, ManagedTag)
}

// GuestDisk is a VM's primary disk.
type GuestDisk struct {
	Storage string
	Size    string
}

// GuestNIC is a VM's primary network interface.
type GuestNIC struct {
	Bridge string
	Model  string
}

// GuestSpec is the desired configuration for creating a guest. It is a flat
// value so the reconciler does not leak manifest types into this package.
type GuestSpec struct {
	Kind     Kind
	Node     string
	VMID     int
	Name     string
	Cores    int
	MemoryMB int
	Disk     GuestDisk
	NIC      GuestNIC
	ISO      string
	Running  bool
	Tags     []string
}

// GuestUpdate carries the safe, non-destructive drift corrections applied to an
// existing guest: cores, memory, and power state.
type GuestUpdate struct {
	Node     string
	VMID     int
	Cores    int
	MemoryMB int
	Running  bool
}

// GuestStore reads and mutates QEMU guests and LXC containers. Ownership is
// tracked with ManagedTag, so untagged guests are never mutated.
type GuestStore interface {
	// ListGuests returns every VM and container visible in the cluster.
	ListGuests(ctx context.Context) ([]Object, error)
	// CreateGuest provisions a guest to match spec.
	CreateGuest(ctx context.Context, spec GuestSpec) error
	// DeleteGuest removes a guest proxmops owns.
	DeleteGuest(ctx context.Context, obj Object) error
	// UpdateGuest applies safe config drift corrections to an existing guest.
	UpdateGuest(ctx context.Context, upd GuestUpdate) error
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
	// DeleteISO removes an ISO file from node/storage.
	DeleteISO(ctx context.Context, node, storage, filename string) error
}
