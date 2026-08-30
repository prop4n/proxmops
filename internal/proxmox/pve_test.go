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
