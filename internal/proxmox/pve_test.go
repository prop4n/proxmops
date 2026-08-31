package proxmox

import (
	"slices"
	"testing"
)

func TestParseTags(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"prod", []string{"prod"}},
		{"prod;web", []string{"prod", "web"}},
		{" prod ; ; web ", []string{"prod", "web"}},
		{"managed-by-proxmops;prod", []string{"managed-by-proxmops", "prod"}},
	}
	for _, tt := range tests {
		if got := parseTags(tt.in); !slices.Equal(got, tt.want) {
			t.Errorf("parseTags(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestIsoCdromOption(t *testing.T) {
	if _, ok := isoCdromOption(""); ok {
		t.Error("empty iso should not produce an option")
	}
	opt, ok := isoCdromOption("local:iso/seed.iso")
	if !ok || opt.Name != "ide0" || opt.Value != "local:iso/seed.iso,media=cdrom" {
		t.Fatalf("got %+v, %v; want ide0=local:iso/seed.iso,media=cdrom", opt, ok)
	}
}

func TestDiskSlot(t *testing.T) {
	if diskSlot("virtio") != "virtio0" {
		t.Errorf("virtio -> %q, want virtio0", diskSlot("virtio"))
	}
	for _, b := range []string{"", "scsi", "anything"} {
		if diskSlot(b) != "scsi0" {
			t.Errorf("diskSlot(%q) = %q, want scsi0", b, diskSlot(b))
		}
	}
}

func TestKindFromType(t *testing.T) {
	tests := []struct {
		in    string
		want  Kind
		valid bool
	}{
		{"qemu", KindVirtualMachine, true},
		{"lxc", KindContainer, true},
		{"storage", "", false},
		{"node", "", false},
	}
	for _, tt := range tests {
		got, ok := kindFromType(tt.in)
		if ok != tt.valid || got != tt.want {
			t.Errorf("kindFromType(%q) = %q,%v, want %q,%v", tt.in, got, ok, tt.want, tt.valid)
		}
	}
}
