package manifest

import (
	"testing"
	"testing/fstest"
)

func TestLoadVirtualMachine(t *testing.T) {
	fsys := fstest.MapFS{
		"vms/web-01.yaml": &fstest.MapFile{Data: []byte(`
apiVersion: proxmops.dev/v1
kind: VirtualMachine
metadata:
  name: web-01
  node: pve-node1
spec:
  vmid: 101
  cores: 2
  memory: 2048
`)},
	}
	res, err := Load(fsys, ".")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("got %d resources, want 1", len(res))
	}
	if got := res[0].GetObjectMeta().Name; got != "web-01" {
		t.Errorf("name = %q, want web-01", got)
	}
	if got := res[0].GetTypeMeta().Kind; got != KindVirtualMachine {
		t.Errorf("kind = %q, want %q", got, KindVirtualMachine)
	}
}

func TestLoadUnknownKind(t *testing.T) {
	fsys := fstest.MapFS{
		"bad.yaml": &fstest.MapFile{Data: []byte("apiVersion: proxmops.dev/v1\nkind: Bogus\nmetadata:\n  name: x\n")},
	}
	if _, err := Load(fsys, "."); err == nil {
		t.Fatal("want error for unknown kind")
	}
}

func TestLoadInvalidResource(t *testing.T) {
	fsys := fstest.MapFS{
		"vms/bad.yaml": &fstest.MapFile{Data: []byte(`
apiVersion: proxmops.dev/v1
kind: VirtualMachine
metadata:
  name: no-node
spec:
  vmid: 1
`)},
	}
	if _, err := Load(fsys, "."); err == nil {
		t.Fatal("want validation error for missing node")
	}
}
