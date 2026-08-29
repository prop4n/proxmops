package manifest

import "fmt"

// Container is the desired state of an LXC container.
type Container struct {
	TypeMeta `yaml:",inline"`
	Metadata ObjectMeta    `yaml:"metadata"`
	Spec     ContainerSpec `yaml:"spec"`
}

// ContainerSpec describes the configuration of an LXC container.
type ContainerSpec struct {
	VMID     int        `yaml:"vmid"`
	Cores    int        `yaml:"cores"`
	Memory   int        `yaml:"memory"`
	RootFS   Disk       `yaml:"rootfs"`
	Net      []NIC      `yaml:"net,omitempty"`
	Template string     `yaml:"template,omitempty"`
	State    PowerState `yaml:"state,omitempty"`
}

// GetTypeMeta implements Resource.
func (c Container) GetTypeMeta() TypeMeta { return c.TypeMeta }

// GetObjectMeta implements Resource.
func (c Container) GetObjectMeta() ObjectMeta { return c.Metadata }

// Validate reports whether the manifest is well-formed.
func (c Container) Validate() error {
	if err := validateMeta(c.TypeMeta, c.Metadata); err != nil {
		return err
	}
	if c.Metadata.Node == "" {
		return fmt.Errorf("metadata.node is required for a Container")
	}
	if c.Spec.VMID <= 0 {
		return fmt.Errorf("spec.vmid must be positive")
	}
	return nil
}
