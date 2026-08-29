package source

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"

	"github.com/prop4n/proxmops/internal/config"
	"github.com/prop4n/proxmops/internal/manifest"
)

// defaultGitUsername is used for HTTPS basic auth when none is configured. Git
// hosts accept any non-empty username alongside a token password.
const defaultGitUsername = "git"

// Git loads manifests from a Git repository, keeping a local clone in CacheDir
// up to date on every call.
type Git struct {
	repoURL  string
	path     string
	revision string
	cacheDir string
	auth     transport.AuthMethod
}

// newGit builds a Git source from configuration.
func newGit(cfg config.Source) *Git {
	cacheDir := cfg.CacheDir
	if cacheDir == "" {
		cacheDir = filepath.Join(os.TempDir(), "proxmops-repo")
	}
	revision := cfg.Revision
	if revision == "" {
		revision = "main"
	}

	var auth transport.AuthMethod
	if cfg.Token != "" {
		username := cfg.Username
		if username == "" {
			username = defaultGitUsername
		}
		auth = &githttp.BasicAuth{Username: username, Password: cfg.Token}
	}

	return &Git{
		repoURL:  cfg.RepoURL,
		path:     cfg.Path,
		revision: revision,
		cacheDir: cacheDir,
		auth:     auth,
	}
}

// Desired syncs the clone to the configured revision and loads the manifests
// from the configured sub-path.
func (g *Git) Desired(ctx context.Context) ([]manifest.Resource, error) {
	if err := g.sync(ctx); err != nil {
		return nil, err
	}
	root := filepath.Join(g.cacheDir, g.path)
	return manifest.Load(os.DirFS(root), ".")
}

// sync clones the repository if needed, fetches updates, and checks out the
// target revision, discarding any local state.
func (g *Git) sync(ctx context.Context) error {
	repo, err := git.PlainOpen(g.cacheDir)
	if errors.Is(err, git.ErrRepositoryNotExists) {
		repo, err = git.PlainCloneContext(ctx, g.cacheDir, false, &git.CloneOptions{
			URL:  g.repoURL,
			Auth: g.auth,
		})
	}
	if err != nil {
		return fmt.Errorf("open/clone %s: %w", g.repoURL, err)
	}

	err = repo.FetchContext(ctx, &git.FetchOptions{Auth: g.auth, Force: true})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("fetch %s: %w", g.repoURL, err)
	}

	hash, err := g.resolve(repo)
	if err != nil {
		return fmt.Errorf("resolve revision %q: %w", g.revision, err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return err
	}
	if err := wt.Checkout(&git.CheckoutOptions{Hash: *hash, Force: true}); err != nil {
		return fmt.Errorf("checkout %s: %w", hash, err)
	}
	return nil
}

// resolve turns the configured revision into a commit hash, preferring the
// remote-tracking branch so a branch name follows upstream after a fetch.
func (g *Git) resolve(repo *git.Repository) (*plumbing.Hash, error) {
	candidates := []plumbing.Revision{
		plumbing.Revision("refs/remotes/origin/" + g.revision),
		plumbing.Revision(g.revision),
	}
	var lastErr error
	for _, rev := range candidates {
		hash, err := repo.ResolveRevision(rev)
		if err == nil {
			return hash, nil
		}
		lastErr = err
	}
	return nil, lastErr
}
