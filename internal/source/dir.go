package source

import (
	"context"
	"os"

	"github.com/prop4n/proxmops/internal/manifest"
)

// Dir loads manifests from a local directory tree.
type Dir struct {
	Root string
}

// Desired reads and decodes every manifest under Root.
func (d *Dir) Desired(context.Context) ([]manifest.Resource, error) {
	return manifest.Load(os.DirFS(d.Root), ".")
}
