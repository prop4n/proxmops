package proxmox

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	diskfs "github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/filesystem/iso9660"
)

func TestParseCidataHash(t *testing.T) {
	cases := map[string]string{
		"local:iso/proxmops-cidata-9100-a1b2c3d4.iso,media=cdrom": "a1b2c3d4",
		"local:iso/alpine.iso,media=cdrom":                        "",
		"":                                                        "",
		"none,media=cdrom":                                        "",
	}
	for in, want := range cases {
		if got := parseCidataHash(in); got != want {
			t.Errorf("parseCidataHash(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseISOVolume(t *testing.T) {
	s, f := parseISOVolume("local:iso/proxmops-cidata-9100-a1b2c3d4.iso,media=cdrom")
	if s != "local" || f != "proxmops-cidata-9100-a1b2c3d4.iso" {
		t.Fatalf("got %q,%q", s, f)
	}
	if s, f := parseISOVolume("local-lvm:vm-9100-disk-0"); s != "" || f != "" {
		t.Fatalf("non-iso volume should be empty, got %q,%q", s, f)
	}
}

func TestBuildCidataISO(t *testing.T) {
	userData := "#cloud-config\n((system-file . \"systems/web01.scm\"))\n"
	iso, hash, err := buildCidataISO("web01", userData)
	if err != nil {
		t.Fatalf("buildCidataISO: %v", err)
	}
	if len(iso) == 0 {
		t.Fatal("empty iso")
	}
	if len(hash) != 8 {
		t.Fatalf("hash = %q, want 8 hex chars", hash)
	}

	// Deterministic: same input -> same hash.
	_, hash2, _ := buildCidataISO("web01", userData)
	if hash != hash2 {
		t.Fatalf("hash not deterministic: %q vs %q", hash, hash2)
	}
	// Different user-data -> different hash.
	_, hash3, _ := buildCidataISO("web01", userData+"x")
	if hash == hash3 {
		t.Fatal("hash did not change with user-data")
	}

	// Read the ISO back and confirm the label and user-data payload.
	path := filepath.Join(t.TempDir(), "c.iso")
	if err := os.WriteFile(path, iso, 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := diskfs.Open(path)
	if err != nil {
		t.Fatalf("open iso: %v", err)
	}
	fs, err := d.GetFilesystem(0)
	if err != nil {
		t.Fatalf("get fs: %v", err)
	}
	if isofs, ok := fs.(*iso9660.FileSystem); ok {
		// ISO9660 pads the volume id to 32 chars with NULs.
		if got := strings.Trim(isofs.Label(), "\x00 "); got != "CIDATA" {
			t.Errorf("label = %q, want CIDATA", got)
		}
	}
	f, err := fs.OpenFile("/user-data", os.O_RDONLY)
	if err != nil {
		t.Fatalf("open user-data: %v", err)
	}
	got, _ := io.ReadAll(f)
	if !bytes.Contains(got, []byte("system-file")) {
		t.Fatalf("user-data content = %q, want the payload", got)
	}
}
