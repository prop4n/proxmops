package manifest

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// yamlUnmarshal decodes a YAML string into v, for exercising custom unmarshalers.
func yamlUnmarshal(t *testing.T, s string, v any) error {
	t.Helper()
	return yaml.Unmarshal([]byte(s), v)
}

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

func TestVMFromTemplateScalar(t *testing.T) {
	var ft FromTemplate
	if err := yamlUnmarshal(t, "debian-12-tpl", &ft); err != nil {
		t.Fatalf("unmarshal scalar: %v", err)
	}
	if ft.Name != "debian-12-tpl" || ft.Linked {
		t.Fatalf("got %+v, want name debian-12-tpl, full clone", ft)
	}
}

func TestVMFromTemplateObject(t *testing.T) {
	var ft FromTemplate
	if err := yamlUnmarshal(t, "{name: debian-12-tpl, linked: true}", &ft); err != nil {
		t.Fatalf("unmarshal object: %v", err)
	}
	if ft.Name != "debian-12-tpl" || !ft.Linked {
		t.Fatalf("got %+v, want name debian-12-tpl, linked", ft)
	}
}

func TestVMValidateImageAndFromTemplateExclusive(t *testing.T) {
	vm := baseVM()
	vm.Spec.Image = &Image{Source: "https://ex/d.qcow2"}
	vm.Spec.FromTemplate = &FromTemplate{Name: "tpl"}
	if err := vm.Validate(); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("want mutually-exclusive error, got %v", err)
	}
}

func TestVMValidateFromTemplateRequiresName(t *testing.T) {
	vm := baseVM()
	vm.Spec.FromTemplate = &FromTemplate{}
	if err := vm.Validate(); err == nil || !strings.Contains(err.Error(), "fromTemplate.name") {
		t.Fatalf("want fromTemplate.name error, got %v", err)
	}
}

func TestVMValidateCloudInitAllowedWithFromTemplate(t *testing.T) {
	vm := baseVM()
	vm.Spec.FromTemplate = &FromTemplate{Name: "tpl"}
	vm.Spec.CloudInit = &CloudInit{User: "debian"}
	if err := vm.Validate(); err != nil {
		t.Fatalf("cloudInit with fromTemplate should be valid, got %v", err)
	}
}

func TestVMValidateUserDataAndCloudInitExclusive(t *testing.T) {
	vm := baseVM()
	vm.Spec.UserData = "#cloud-config\n"
	vm.Spec.CloudInit = &CloudInit{User: "x"}
	vm.Spec.FromTemplate = &FromTemplate{Name: "tpl"}
	if err := vm.Validate(); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("want userData/cloudInit exclusive error, got %v", err)
	}
}

func TestVMValidateUserDataAndISOExclusive(t *testing.T) {
	vm := baseVM()
	vm.Spec.UserData = "#cloud-config\n"
	vm.Spec.ISO = "local:iso/seed.iso"
	if err := vm.Validate(); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("want userData/iso exclusive error, got %v", err)
	}
}

func TestVMValidateUserDataAlone(t *testing.T) {
	vm := baseVM()
	vm.Spec.FromTemplate = &FromTemplate{Name: "tpl"}
	vm.Spec.UserData = "#cloud-config\n((system-file . \"systems/web01.scm\"))\n"
	if err := vm.Validate(); err != nil {
		t.Fatalf("userData alone should be valid, got %v", err)
	}
}

func TestVMValidateDiskBus(t *testing.T) {
	vm := baseVM()
	vm.Spec.Disks = []Disk{{Storage: "local-lvm", Size: "20G", Bus: "virtio"}}
	if err := vm.Validate(); err != nil {
		t.Fatalf("virtio bus should be valid, got %v", err)
	}
	vm.Spec.Disks[0].Bus = "sata"
	if err := vm.Validate(); err == nil || !strings.Contains(err.Error(), "bus") {
		t.Fatalf("want bus error, got %v", err)
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
