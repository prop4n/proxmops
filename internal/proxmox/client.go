// Package proxmox is the client for the Proxmox VE API. It defines the
// interface the reconciliation engine depends on, the data types that represent
// observed cluster state, and an adapter over github.com/luthermonson/go-proxmox.
package proxmox

import (
	"context"
	"errors"
	"slices"
)

// ErrNotImplemented is returned by operations that are not built yet.
var ErrNotImplemented = errors.New("proxmox: not implemented")

// ManagedTag marks the resources proxmops owns. Only tagged resources are ever
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

// Object is the observed state of a single cluster resource.
type Object struct {
	Kind Kind
	Name string
	Node string
	ID   string
	Tags []string
	// Spec holds kind-specific observed fields used for diffing.
	Spec map[string]any
}

// Owned reports whether the object carries the proxmops ownership tag.
func (o Object) Owned() bool {
	return slices.Contains(o.Tags, ManagedTag)
}

// Client reads and mutates Proxmox cluster state. Implementations must be safe
// for concurrent use.
type Client interface {
	// List returns the observed state of every resource proxmops can see.
	List(ctx context.Context) ([]Object, error)
	// Apply creates or updates a resource to match the desired object.
	Apply(ctx context.Context, obj Object) error
	// Delete removes a resource that proxmops owns.
	Delete(ctx context.Context, obj Object) error
}
