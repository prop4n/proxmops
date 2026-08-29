// Package source provides desired-state loaders. A remote repoURL is cloned
// over Git; any other value is read from the local filesystem. Both satisfy the
// Desired contract the reconciliation engine depends on.
package source

import (
	"context"
	"fmt"
	"os"
	"strings"

	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"

	"github.com/prop4n/proxmops/internal/config"
	"github.com/prop4n/proxmops/internal/manifest"
)

// Source loads the desired state.
type Source interface {
	Desired(ctx context.Context) ([]manifest.Resource, error)
}

// New builds the appropriate Source for the configuration: Git for a remote
// URL, otherwise a local directory.
func New(cfg config.Source) Source {
	if isRemote(cfg.RepoURL) {
		return newGit(cfg)
	}
	root := cfg.Path
	if root == "" {
		root = "."
	}
	return &Dir{Root: root}
}

// CheckURL verifies that the configured source is reachable without loading
// any manifests: a remote repository is probed with a Git reference list, a
// local path is checked for existence. It backs the web UI connection test.
func CheckURL(ctx context.Context, cfg config.Source) error {
	if !isRemote(cfg.RepoURL) {
		root := cfg.Path
		if root == "" {
			root = "."
		}
		if _, err := os.Stat(root); err != nil {
			return fmt.Errorf("stat %s: %w", root, err)
		}
		return nil
	}

	var auth transport.AuthMethod
	if cfg.Token != "" {
		username := cfg.Username
		if username == "" {
			username = defaultGitUsername
		}
		auth = &githttp.BasicAuth{Username: username, Password: cfg.Token}
	}
	remote := git.NewRemote(memory.NewStorage(), &gitconfig.RemoteConfig{URLs: []string{cfg.RepoURL}})
	if _, err := remote.ListContext(ctx, &git.ListOptions{Auth: auth}); err != nil {
		return fmt.Errorf("list %s: %w", cfg.RepoURL, err)
	}
	return nil
}

// isRemote reports whether repoURL denotes a remote Git repository.
func isRemote(repoURL string) bool {
	return strings.Contains(repoURL, "://") || strings.HasPrefix(repoURL, "git@")
}
