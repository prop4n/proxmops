// Package manifest defines the declarative desired-state resources that
// proxmops reads from a Git repository, together with a loader that decodes
// them from a filesystem tree.
package manifest

import "fmt"

// APIVersion is the schema version understood by this build.
const APIVersion = "proxmops.dev/v1"

// Kind identifies the type of a resource manifest.
type Kind string

// Supported resource kinds.
const (
	KindVirtualMachine Kind = "VirtualMachine"
	KindContainer      Kind = "Container"
	KindIso            Kind = "Iso"
	KindNetwork        Kind = "Network"
)

// TypeMeta describes the schema version and kind of a manifest. It is embedded
// in every concrete resource type.
type TypeMeta struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       Kind   `yaml:"kind"`
}

// ObjectMeta holds the identifying metadata common to every resource.
type ObjectMeta struct {
	Name string   `yaml:"name"`
	Node string   `yaml:"node,omitempty"`
	Tags []string `yaml:"tags,omitempty"`
}

// Resource is implemented by every declarative resource kind. It exposes the
// shared metadata and self-validation so callers can treat kinds uniformly.
type Resource interface {
	GetTypeMeta() TypeMeta
	GetObjectMeta() ObjectMeta
	Validate() error
}

// validateMeta checks the fields shared by all resources.
func validateMeta(tm TypeMeta, om ObjectMeta) error {
	if tm.APIVersion != APIVersion {
		return fmt.Errorf("unsupported apiVersion %q, want %q", tm.APIVersion, APIVersion)
	}
	if om.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	return nil
}
