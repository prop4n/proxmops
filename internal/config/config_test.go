package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "proxmops.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

const validConfig = `
cluster:
  endpoint: https://pve:8006/api2/json
  tokenId: id
  tokenSecret: secret
source:
  repoURL: https://example.com/repo.git
  path: proxmox
reconcile:
  interval: 30s
  autoSync: true
`

func TestLoadValid(t *testing.T) {
	cfg, err := Load(writeConfig(t, validConfig))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Reconcile.Interval != 30*time.Second {
		t.Errorf("interval = %v, want 30s", cfg.Reconcile.Interval)
	}
	if !cfg.Reconcile.AutoSync {
		t.Error("autoSync = false, want true")
	}
}

func TestLoadDefaultInterval(t *testing.T) {
	const noInterval = `
cluster:
  endpoint: https://pve:8006/api2/json
  tokenId: id
  tokenSecret: secret
source:
  repoURL: https://example.com/repo.git
`
	cfg, err := Load(writeConfig(t, noInterval))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Reconcile.Interval != time.Minute {
		t.Errorf("interval = %v, want default 1m", cfg.Reconcile.Interval)
	}
	if cfg.Reconcile.Concurrency != 4 {
		t.Errorf("concurrency = %d, want default 4", cfg.Reconcile.Concurrency)
	}
}

func TestLoadEnvOverridesFile(t *testing.T) {
	const noSecret = `
cluster:
  endpoint: https://pve:8006/api2/json
  tokenId: id
source:
  repoURL: https://example.com/repo.git
`
	t.Setenv("PROXMOPS_CLUSTER_TOKENSECRET", "from-env")
	cfg, err := Load(writeConfig(t, noSecret))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Cluster.TokenSecret != "from-env" {
		t.Errorf("tokenSecret = %q, want from-env", cfg.Cluster.TokenSecret)
	}
}

// A container or unit can point the database at a state dir via the env.
func TestServerDatabasePathFromEnv(t *testing.T) {
	t.Setenv("PROXMOPS_SERVER_DATABASEPATH", "/var/lib/proxmops/proxmops.db")
	cfg, err := LoadDraft(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil {
		t.Fatalf("LoadDraft: %v", err)
	}
	if cfg.Server.DatabasePath != "/var/lib/proxmops/proxmops.db" {
		t.Errorf("databasePath = %q, want the env value", cfg.Server.DatabasePath)
	}
	if cfg.Server.KeyPath != "/var/lib/proxmops/proxmops.db.key" {
		t.Errorf("keyPath = %q, want derived .key path", cfg.Server.KeyPath)
	}
}

func TestLoadMissingEndpoint(t *testing.T) {
	const noEndpoint = `
cluster:
  tokenId: id
  tokenSecret: secret
source:
  repoURL: https://example.com/repo.git
`
	_, err := Load(writeConfig(t, noEndpoint))
	if err == nil || !strings.Contains(err.Error(), "endpoint") {
		t.Fatalf("want endpoint error, got %v", err)
	}
}
