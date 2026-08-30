package manifest

import "fmt"

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
	VMID   int        `yaml:"vmid"`
	Cores  int        `yaml:"cores"`
	Memory int        `yaml:"memory"`
	Disks  []Disk     `yaml:"disks,omitempty"`
	Net    []NIC      `yaml:"net,omitempty"`
	ISO    string     `yaml:"iso,omitempty"`
	State  PowerState `yaml:"state,omitempty"`
	// ApplyMode controls how config changes that need a restart (cores, memory)
	// are applied to a running VM. Empty reports "reboot required" and leaves the
	// VM running; "reboot" lets proxmops restart the VM to apply them.
	ApplyMode ApplyMode `yaml:"applyMode,omitempty"`
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
	return nil
}
