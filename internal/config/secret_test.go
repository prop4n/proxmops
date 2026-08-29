package config

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveSecretEnvWins(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "secret")
	if err := os.WriteFile(file, []byte("from-file"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROXMOPS_TEST_SECRET", "from-env")

	got, inline, err := resolveSecret("PROXMOPS_TEST_SECRET", file, "from-inline")
	if err != nil {
		t.Fatal(err)
	}
	if got != "from-env" || inline {
		t.Fatalf("got %q inline=%v, want from-env inline=false", got, inline)
	}
}

func TestResolveSecretFileOverInline(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "secret")
	if err := os.WriteFile(file, []byte("  from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, inline, err := resolveSecret("PROXMOPS_ABSENT_SECRET", file, "from-inline")
	if err != nil {
		t.Fatal(err)
	}
	if got != "from-file" {
		t.Errorf("got %q, want from-file (trimmed)", got)
	}
	if inline {
		t.Error("inline should be false when file supplies the value")
	}
}

func TestResolveSecretInlineFlagged(t *testing.T) {
	got, inline, err := resolveSecret("PROXMOPS_ABSENT_SECRET", "", "from-inline")
	if err != nil {
		t.Fatal(err)
	}
	if got != "from-inline" || !inline {
		t.Fatalf("got %q inline=%v, want from-inline inline=true", got, inline)
	}
}

func TestResolveSecretMissingFile(t *testing.T) {
	if _, _, err := resolveSecret("PROXMOPS_ABSENT_SECRET", "/no/such/file", ""); err == nil {
		t.Fatal("want error for missing secret file")
	}
}

func TestResolveSecretsWarnsOnInline(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	cfg := Config{Cluster: Cluster{TokenSecret: "inline-secret"}}
	if err := cfg.resolveSecrets(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "clear text") {
		t.Errorf("expected clear-text warning, got:\n%s", buf.String())
	}
}

func TestLoadTokenSecretFromFile(t *testing.T) {
	dir := t.TempDir()
	secretFile := filepath.Join(dir, "token")
	if err := os.WriteFile(secretFile, []byte("filesecret"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfgFile := writeConfig(t, `
cluster:
  endpoint: https://pve:8006/api2/json
  tokenId: id
  tokenSecretFile: `+secretFile+`
source:
  repoURL: https://example.com/repo.git
`)
	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Cluster.TokenSecret != "filesecret" {
		t.Fatalf("tokenSecret = %q, want filesecret", cfg.Cluster.TokenSecret)
	}
}
