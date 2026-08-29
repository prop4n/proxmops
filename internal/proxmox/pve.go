package proxmox

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	pve "github.com/luthermonson/go-proxmox"

	"github.com/prop4n/proxmops/internal/config"
)

// downloadTimeout bounds how long to wait for an ISO download task.
const downloadTimeout = 30 * time.Minute

// isoContent is the Proxmox storage content type for ISO images.
const isoContent = "iso"

// PVE adapts the go-proxmox SDK to the store interfaces. A single value
// satisfies both GuestStore and IsoStore.
type PVE struct {
	api *pve.Client
}

// compile-time checks that PVE implements the store interfaces.
var (
	_ GuestStore = (*PVE)(nil)
	_ IsoStore   = (*PVE)(nil)
)

// New returns a PVE client for the given cluster configuration, authenticating
// with a Proxmox API token.
func New(cluster config.Cluster) *PVE {
	opts := []pve.Option{pve.WithAPIToken(cluster.TokenID, cluster.TokenSecret)}
	if cluster.InsecureSkipVerify {
		opts = append(opts, pve.WithInsecureSkipVerify())
	}
	return &PVE{api: pve.NewClient(cluster.Endpoint, opts...)}
}

// Ping verifies the endpoint is reachable and the API token works by fetching
// the cluster version. It backs the web UI connection test.
func (c *PVE) Ping(ctx context.Context) error {
	if _, err := c.api.Version(ctx); err != nil {
		return fmt.Errorf("get version: %w", err)
	}
	return nil
}

// ListGuests reports the QEMU guests and LXC containers visible in the cluster.
func (c *PVE) ListGuests(ctx context.Context) ([]Object, error) {
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

// CreateGuest is not implemented yet; guest provisioning lands with the VM phase.
func (c *PVE) CreateGuest(context.Context, Object) error { return ErrNotImplemented }

// DeleteGuest is not implemented yet; guest deletion lands with the VM phase.
func (c *PVE) DeleteGuest(context.Context, Object) error { return ErrNotImplemented }

// ListISOs returns the filenames of the ISOs present on node/storage.
func (c *PVE) ListISOs(ctx context.Context, node, storageName string) ([]string, error) {
	storage, err := c.storage(ctx, node, storageName)
	if err != nil {
		return nil, err
	}
	content, err := storage.GetContent(ctx)
	if err != nil {
		return nil, fmt.Errorf("get content of %s/%s: %w", node, storageName, err)
	}

	var names []string
	for _, item := range content {
		if name, ok := isoFilename(item.Volid); ok {
			names = append(names, name)
		}
	}
	return names, nil
}

// DownloadISO fetches an ISO onto a storage and waits for the task to complete.
func (c *PVE) DownloadISO(ctx context.Context, req IsoDownload) error {
	storage, err := c.storage(ctx, req.Node, req.Storage)
	if err != nil {
		return err
	}

	var task *pve.Task
	if req.Checksum != "" {
		task, err = storage.DownloadURLWithHash(ctx, isoContent, req.Filename, req.URL, req.Checksum, req.ChecksumAlgo)
	} else {
		task, err = storage.DownloadURL(ctx, isoContent, req.Filename, req.URL)
	}
	if err != nil {
		return fmt.Errorf("download %s: %w", req.Filename, err)
	}
	if err := task.Wait(ctx, 2*time.Second, downloadTimeout); err != nil {
		return fmt.Errorf("download %s: %w", req.Filename, err)
	}
	return nil
}

// storage resolves a storage handle on a node.
func (c *PVE) storage(ctx context.Context, node, name string) (*pve.Storage, error) {
	n, err := c.api.Node(ctx, node)
	if err != nil {
		return nil, fmt.Errorf("get node %s: %w", node, err)
	}
	s, err := n.Storage(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("get storage %s on %s: %w", name, node, err)
	}
	return s, nil
}

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

// isoFilename extracts the filename from an ISO volid such as
// "local:iso/debian-12.iso", reporting whether the volid is an ISO.
func isoFilename(volid string) (string, bool) {
	_, after, ok := strings.Cut(volid, ":"+isoContent+"/")
	if !ok {
		return "", false
	}
	return path.Base(after), true
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
