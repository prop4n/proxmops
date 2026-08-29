package proxmox

import (
	"context"
	"fmt"
	"strings"

	pve "github.com/luthermonson/go-proxmox"

	"github.com/prop4n/proxmops/internal/config"
)

// pveClient adapts the go-proxmox SDK to the Client interface.
type pveClient struct {
	api *pve.Client
}

// New returns a Client for the given cluster configuration, authenticating with
// a Proxmox API token.
func New(cluster config.Cluster) Client {
	opts := []pve.Option{pve.WithAPIToken(cluster.TokenID, cluster.TokenSecret)}
	if cluster.InsecureSkipVerify {
		opts = append(opts, pve.WithInsecureSkipVerify())
	}
	return &pveClient{api: pve.NewClient(cluster.Endpoint, opts...)}
}

// List reports the QEMU guests and LXC containers visible in the cluster. ISOs
// and networks are not enumerated yet; they arrive with their respective
// reconciliation phases.
func (c *pveClient) List(ctx context.Context) ([]Object, error) {
	cluster, err := c.api.Cluster(ctx)
	if err != nil {
		return nil, fmt.Errorf("get cluster: %w", err)
	}
	resources, err := cluster.Resources(ctx, "vm")
	if err != nil {
		return nil, fmt.Errorf("list resources: %w", err)
	}

	objects := make([]Object, 0, len(resources))
	for _, r := range resources {
		kind, ok := kindFromType(r.Type)
		if !ok {
			continue
		}
		objects = append(objects, Object{
			Kind: kind,
			Name: r.Name,
			Node: r.Node,
			ID:   r.ID,
			Tags: parseTags(r.Tags),
		})
	}
	return objects, nil
}

// Apply is not implemented yet; mutation lands with Phase 1.
func (c *pveClient) Apply(context.Context, Object) error { return ErrNotImplemented }

// Delete is not implemented yet; mutation lands with Phase 1.
func (c *pveClient) Delete(context.Context, Object) error { return ErrNotImplemented }

// kindFromType maps a Proxmox cluster resource type to a proxmops Kind.
func kindFromType(t string) (Kind, bool) {
	switch t {
	case "qemu":
		return KindVirtualMachine, true
	case "lxc":
		return KindContainer, true
	default:
		return "", false
	}
}

// parseTags splits the Proxmox tag string (semicolon-separated) into a slice,
// dropping empty entries.
func parseTags(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ";")
	tags := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			tags = append(tags, p)
		}
	}
	return tags
}
