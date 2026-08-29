// Package source provides desired-state loaders. A remote repoURL is cloned
// over Git; any other value is read from the local filesystem. Both satisfy the
// Desired contract the reconciliation engine depends on.
package source

import (
	"context"
	"strings"

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

// isRemote reports whether repoURL denotes a remote Git repository.
func isRemote(repoURL string) bool {
	return strings.Contains(repoURL, "://") || strings.HasPrefix(repoURL, "git@")
}
