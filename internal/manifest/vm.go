package manifest

import (
	"fmt"
	"net/url"
	"path"

	"gopkg.in/yaml.v3"
)

// VirtualMachine is the desired state of a QEMU guest.
type VirtualMachine struct {
	TypeMeta `yaml:",inline"`
	Metadata ObjectMeta         `yaml:"metadata"`
	Spec     VirtualMachineSpec `yaml:"spec"`
}

// GetVMID implements VMIDer.
func (vm VirtualMachine) GetVMID() int { return vm.Spec.VMID }

// VirtualMachineSpec describes the configuration of a QEMU guest.
type VirtualMachineSpec struct {
	VMID   int `yaml:"vmid"`
	Cores  int `yaml:"cores"`
	Memory int `yaml:"memory"`
	// CPU is the processor type (e.g. "x86-64-v2", "host"). Empty keeps the
	// Proxmox default.
	CPU   string `yaml:"cpu,omitempty"`
	Disks []Disk `yaml:"disks,omitempty"`
	Net   []NIC  `yaml:"net,omitempty"`
	// ISO attaches an existing image as a cdrom drive, given as a Proxmox volume
	// reference (e.g. "local:iso/seed.iso"). Useful for a NoCloud cidata disk
	// carrying custom user-data. Independent of cloudInit.
	ISO   string     `yaml:"iso,omitempty"`
	State PowerState `yaml:"state,omitempty"`
	// Image, when set, provisions the VM from a cloud image instead of a blank
	// disk, enabling cloud-init.
	Image *Image `yaml:"image,omitempty"`
	// FromTemplate, when set, clones an existing Template instead of building
	// from an image. Mutually exclusive with Image.
	FromTemplate *FromTemplate `yaml:"fromTemplate,omitempty"`
	// CloudInit configures the cloud-init drive; meaningful with Image or
	// FromTemplate.
	CloudInit *CloudInit `yaml:"cloudInit,omitempty"`
	// ApplyMode controls how config changes that need a restart (cores, memory)
	// are applied to a running VM. Empty reports "reboot required" and leaves the
	// VM running; "reboot" lets proxmops restart the VM to apply them.
	ApplyMode ApplyMode `yaml:"applyMode,omitempty"`
}

// Image is a cloud image the VM disk is imported from.
type Image struct {
	// Source is the URL of the cloud image (.qcow2/.img).
	Source string `yaml:"source"`
	// ImportStorage is the directory storage (with the "import" content type)
	// the image is downloaded to. Empty auto-selects one.
	ImportStorage string `yaml:"importStorage,omitempty"`
}

// FromTemplate clones an existing Template. It accepts either a scalar template
// name (a full clone) or a mapping with an optional linked flag.
type FromTemplate struct {
	Name string `yaml:"name"`
	// Linked makes a copy-on-write clone tied to the template. Default is a full,
	// independent clone.
	Linked bool `yaml:"linked,omitempty"`
}

// UnmarshalYAML accepts either a scalar name ("debian-12-tpl") or a mapping
// ({name: ..., linked: true}).
func (f *FromTemplate) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		return node.Decode(&f.Name)
	}
	type raw FromTemplate
	var r raw
	if err := node.Decode(&r); err != nil {
		return err
	}
	*f = FromTemplate(r)
	return nil
}

// CloudInit holds the cloud-init settings injected into the VM.
type CloudInit struct {
	User     string   `yaml:"user,omitempty"`
	Password string   `yaml:"password,omitempty"`
	SSHKeys  []string `yaml:"sshKeys,omitempty"`
	// IP is a Proxmox ipconfig0 value: "dhcp" or "ip=10.0.0.5/24,gw=10.0.0.1".
	IP           string `yaml:"ip,omitempty"`
	Nameserver   string `yaml:"nameserver,omitempty"`
	SearchDomain string `yaml:"searchDomain,omitempty"`
}

// ApplyMode selects how restart-requiring changes reach a running VM.
type ApplyMode string

// Recognised apply modes. The zero value reports drift without restarting.
const ApplyModeReboot ApplyMode = "reboot"

// Disk is a virtual disk backed by a storage.
type Disk struct {
	Storage string `yaml:"storage"`
	Size    string `yaml:"size"`
}

// NIC is a virtual network interface. IP and GW apply to containers, whose
// addresses are managed declaratively.
type NIC struct {
	Bridge string `yaml:"bridge"`
	Model  string `yaml:"model,omitempty"`
	IP     string `yaml:"ip,omitempty"`
	GW     string `yaml:"gw,omitempty"`
}

// PowerState is the desired runtime state of a guest.
type PowerState string

// Recognised power states.
const (
	StateRunning PowerState = "running"
	StateStopped PowerState = "stopped"
)

// GetTypeMeta implements Resource.
func (vm VirtualMachine) GetTypeMeta() TypeMeta { return vm.TypeMeta }

// GetObjectMeta implements Resource.
func (vm VirtualMachine) GetObjectMeta() ObjectMeta { return vm.Metadata }

// Validate reports whether the manifest is well-formed.
func (vm VirtualMachine) Validate() error {
	if err := validateMeta(vm.TypeMeta, vm.Metadata); err != nil {
		return err
	}
	if vm.Metadata.Node == "" {
		return fmt.Errorf("metadata.node is required for a VirtualMachine")
	}
	if vm.Spec.VMID <= 0 {
		return fmt.Errorf("spec.vmid must be positive")
	}
	if vm.Spec.Image != nil && vm.Spec.Image.Source == "" {
		return fmt.Errorf("spec.image.source is required when image is set")
	}
	if vm.Spec.Image != nil && vm.Spec.FromTemplate != nil {
		return fmt.Errorf("spec.image and spec.fromTemplate are mutually exclusive")
	}
	if vm.Spec.FromTemplate != nil && vm.Spec.FromTemplate.Name == "" {
		return fmt.Errorf("spec.fromTemplate.name is required")
	}
	if vm.Spec.CloudInit != nil && vm.Spec.Image == nil && vm.Spec.FromTemplate == nil {
		return fmt.Errorf("spec.cloudInit requires spec.image or spec.fromTemplate")
	}
	return nil
}

// Filename derives the storage filename from the image source URL, ignoring any
// query string. Falls back to the raw base when the URL does not parse.
func (i Image) Filename() string {
	if u, err := url.Parse(i.Source); err == nil && u.Path != "" {
		return path.Base(u.Path)
	}
	return path.Base(i.Source)
}
