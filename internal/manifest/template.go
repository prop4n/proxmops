package manifest

import "fmt"

// Template is a golden VM image converted to a Proxmox template, cloned by VMs.
// It is bare: hardware and image only, with no cloud-init (that is set per clone).
type Template struct {
	TypeMeta `yaml:",inline"`
	Metadata ObjectMeta   `yaml:"metadata"`
	Spec     TemplateSpec `yaml:"spec"`
}

// TemplateSpec describes the template to build and convert.
type TemplateSpec struct {
	VMID   int    `yaml:"vmid"`
	Image  *Image `yaml:"image"`
	Cores  int    `yaml:"cores,omitempty"`
	Memory int    `yaml:"memory,omitempty"`
	CPU    string `yaml:"cpu,omitempty"`
	Disks  []Disk `yaml:"disks,omitempty"`
	Net    []NIC  `yaml:"net,omitempty"`
}

// GetVMID implements VMIDer.
func (t Template) GetVMID() int { return t.Spec.VMID }

// GetTypeMeta implements Resource.
func (t Template) GetTypeMeta() TypeMeta { return t.TypeMeta }

// GetObjectMeta implements Resource.
func (t Template) GetObjectMeta() ObjectMeta { return t.Metadata }

// Validate reports whether the manifest is well-formed.
func (t Template) Validate() error {
	if err := validateMeta(t.TypeMeta, t.Metadata); err != nil {
		return err
	}
	if t.Metadata.Node == "" {
		return fmt.Errorf("metadata.node is required for a Template")
	}
	if t.Spec.VMID <= 0 {
		return fmt.Errorf("spec.vmid must be positive")
	}
	if t.Spec.Image == nil || t.Spec.Image.Source == "" {
		return fmt.Errorf("spec.image.source is required for a Template")
	}
	for _, d := range t.Spec.Disks {
		if err := validateBus(d.Bus); err != nil {
			return err
		}
	}
	return nil
}
