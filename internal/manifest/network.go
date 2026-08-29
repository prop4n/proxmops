package manifest

import "fmt"

// Network is the desired state of a node network device such as a bridge.
type Network struct {
	TypeMeta `yaml:",inline"`
	Metadata ObjectMeta  `yaml:"metadata"`
	Spec     NetworkSpec `yaml:"spec"`
}

// NetworkSpec describes a network device.
type NetworkSpec struct {
	Type      string   `yaml:"type"`
	Ports     []string `yaml:"ports,omitempty"`
	VLANAware bool     `yaml:"vlanAware,omitempty"`
	CIDR      string   `yaml:"cidr,omitempty"`
}

// GetTypeMeta implements Resource.
func (n Network) GetTypeMeta() TypeMeta { return n.TypeMeta }

// GetObjectMeta implements Resource.
func (n Network) GetObjectMeta() ObjectMeta { return n.Metadata }

// Validate reports whether the manifest is well-formed.
func (n Network) Validate() error {
	if err := validateMeta(n.TypeMeta, n.Metadata); err != nil {
		return err
	}
	if n.Metadata.Node == "" {
		return fmt.Errorf("metadata.node is required for a Network")
	}
	if n.Spec.Type == "" {
		return fmt.Errorf("spec.type is required")
	}
	return nil
}
