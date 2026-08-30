package manifest

import (
	"strings"
	"testing"
)

func baseVM() VirtualMachine {
	return VirtualMachine{
		TypeMeta: TypeMeta{APIVersion: APIVersion, Kind: KindVirtualMachine},
		Metadata: ObjectMeta{Name: "web", Node: "pve"},
		Spec:     VirtualMachineSpec{VMID: 101},
	}
}

func TestVMValidateOK(t *testing.T) {
	if err := baseVM().Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestVMValidateImageRequiresSource(t *testing.T) {
	vm := baseVM()
	vm.Spec.Image = &Image{}
	if err := vm.Validate(); err == nil || !strings.Contains(err.Error(), "image.source") {
		t.Fatalf("want image.source error, got %v", err)
	}
}

func TestVMValidateCloudInitRequiresImage(t *testing.T) {
	vm := baseVM()
	vm.Spec.CloudInit = &CloudInit{User: "debian"}
	if err := vm.Validate(); err == nil || !strings.Contains(err.Error(), "cloudInit requires") {
		t.Fatalf("want cloudInit-requires-image error, got %v", err)
	}
}

func TestImageFilename(t *testing.T) {
	cases := map[string]string{
		"https://ex/d/debian-12-genericcloud.qcow2":        "debian-12-genericcloud.qcow2",
		"https://ex/d/cirros.img?token=abc":                "cirros.img",
		"https://cloud.debian.org/images/x/debian.qcow2#z": "debian.qcow2",
	}
	for src, want := range cases {
		if got := (Image{Source: src}).Filename(); got != want {
			t.Errorf("Filename(%q) = %q, want %q", src, got, want)
		}
	}
}
