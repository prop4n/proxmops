package source

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/prop4n/proxmops/internal/config"
)

const vmManifest = `
apiVersion: proxmops.dev/v1
kind: VirtualMachine
metadata:
  name: web-01
  node: pve
spec:
  vmid: 101
`

// initRepo creates a git repository containing proxmox/vms/web-01.yaml and
// returns its path. The default branch is "master" (go-git's default).
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	sub := filepath.Join(dir, "proxmox", "vms")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "web-01.yaml"), []byte(vmManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if err := wt.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		t.Fatal(err)
	}
	_, err = wt.Commit("init", &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@example.com", When: time.Now()},
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	return dir
}

func TestGitSourceClonesAndLoads(t *testing.T) {
	repoDir := initRepo(t)

	src := New(config.Source{
		RepoURL:  "file://" + repoDir,
		Path:     "proxmox",
		Revision: "master",
		CacheDir: filepath.Join(t.TempDir(), "clone"),
	})
	if _, ok := src.(*Git); !ok {
		t.Fatalf("expected a Git source, got %T", src)
	}

	res, err := src.Desired(context.Background())
	if err != nil {
		t.Fatalf("Desired: %v", err)
	}
	if len(res) != 1 || res[0].GetObjectMeta().Name != "web-01" {
		t.Fatalf("want web-01, got %+v", res)
	}
}

func TestGitSourceSyncsOnSecondCall(t *testing.T) {
	repoDir := initRepo(t)
	src := New(config.Source{
		RepoURL:  "file://" + repoDir,
		Path:     "proxmox",
		Revision: "master",
		CacheDir: filepath.Join(t.TempDir(), "clone"),
	})

	// First call clones, second call must fetch/checkout an existing clone.
	if _, err := src.Desired(context.Background()); err != nil {
		t.Fatalf("first Desired: %v", err)
	}
	if _, err := src.Desired(context.Background()); err != nil {
		t.Fatalf("second Desired: %v", err)
	}
}

func TestNewSelectsDirForLocalPath(t *testing.T) {
	if _, ok := New(config.Source{RepoURL: "local", Path: "."}).(*Dir); !ok {
		t.Error("local repoURL should select a Dir source")
	}
}

func TestIsRemote(t *testing.T) {
	remote := []string{"https://github.com/x/y.git", "ssh://git@host/x.git", "git@host:x/y.git"}
	local := []string{"local", "", "./manifests", "/etc/proxmops"}
	for _, u := range remote {
		if !isRemote(u) {
			t.Errorf("isRemote(%q) = false, want true", u)
		}
	}
	for _, u := range local {
		if isRemote(u) {
			t.Errorf("isRemote(%q) = true, want false", u)
		}
	}
}
