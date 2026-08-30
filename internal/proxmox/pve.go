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

// deleteTimeout bounds how long to wait for an ISO delete task.
const deleteTimeout = 2 * time.Minute

// guestTimeout bounds how long to wait for a VM create/config/power/delete task.
const guestTimeout = 5 * time.Minute

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
		o := Object{
			Kind:    kind,
			Name:    r.Name,
			Node:    r.Node,
			ID:      r.ID,
			VMID:    int(r.VMID),
			Running: r.Status == "running",
			Tags:    parseTags(r.Tags),
		}
		// Configured cores/memory come from the VM config (authoritative for
		// drift); the cluster listing only carries the live, running values.
		// When they differ on a running VM, a change is pending a restart.
		liveCores, liveMemMB := int(r.MaxCPU), int(r.MaxMem/(1024*1024))
		o.Cores, o.MemoryMB = liveCores, liveMemMB
		if kind == KindVirtualMachine {
			if cfgCores, cfgMemMB, err := c.vmConfig(ctx, r.Node, int(r.VMID)); err == nil {
				o.Cores, o.MemoryMB = cfgCores, cfgMemMB
				o.RebootPending = o.Running && (cfgCores != liveCores || cfgMemMB != liveMemMB)
			}
		}
		objects = append(objects, o)
	}
	return objects, nil
}

// vmConfig reads a VM's configured total vCPUs (sockets×cores) and memory in MB.
func (c *PVE) vmConfig(ctx context.Context, node string, vmid int) (int, int, error) {
	vm, err := c.vm(ctx, node, vmid)
	if err != nil {
		return 0, 0, err
	}
	cfg := vm.VirtualMachineConfig
	cores := derefOr(cfg.Cores, 1) * derefOr(cfg.Sockets, 1)
	return cores, int(cfg.Memory), nil
}

// derefOr returns *p, or def when p is nil or zero.
func derefOr(p *int, def int) int {
	if p == nil || *p == 0 {
		return def
	}
	return *p
}

// CreateGuest provisions a QEMU VM from spec, tags it, and starts it when
// requested. Container creation is not built yet.
func (c *PVE) CreateGuest(ctx context.Context, spec GuestSpec) error {
	if spec.Kind != KindVirtualMachine {
		return ErrNotImplemented
	}
	node, err := c.api.Node(ctx, spec.Node)
	if err != nil {
		return fmt.Errorf("get node %s: %w", spec.Node, err)
	}

	opts := []pve.VirtualMachineOption{
		{Name: "name", Value: spec.Name},
		{Name: "cores", Value: spec.Cores},
		{Name: "memory", Value: spec.MemoryMB},
		{Name: "scsihw", Value: "virtio-scsi-single"},
	}
	if spec.Disk.Storage != "" {
		opts = append(opts, pve.VirtualMachineOption{
			Name:  "scsi0",
			Value: fmt.Sprintf("%s:%d", spec.Disk.Storage, parseSizeGB(spec.Disk.Size)),
		})
	}
	if spec.NIC.Bridge != "" {
		model := spec.NIC.Model
		if model == "" {
			model = "virtio"
		}
		opts = append(opts, pve.VirtualMachineOption{
			Name:  "net0",
			Value: fmt.Sprintf("%s,bridge=%s", model, spec.NIC.Bridge),
		})
	}
	if len(spec.Tags) > 0 {
		opts = append(opts, pve.VirtualMachineOption{Name: "tags", Value: strings.Join(spec.Tags, ";")})
	}

	task, err := node.NewVirtualMachine(ctx, spec.VMID, opts...)
	if err != nil {
		return fmt.Errorf("create vm %d: %w", spec.VMID, err)
	}
	if err := task.Wait(ctx, time.Second, guestTimeout); err != nil {
		return fmt.Errorf("create vm %d: %w", spec.VMID, err)
	}

	if spec.Running {
		return c.setPower(ctx, spec.Node, spec.VMID, true)
	}
	return nil
}

// DeleteGuest stops a VM proxmops owns, then destroys it and its disks.
func (c *PVE) DeleteGuest(ctx context.Context, obj Object) error {
	if obj.Kind != KindVirtualMachine {
		return ErrNotImplemented
	}
	vm, err := c.vm(ctx, obj.Node, obj.VMID)
	if err != nil {
		return err
	}
	if vm.IsRunning() {
		if err := c.setPower(ctx, obj.Node, obj.VMID, false); err != nil {
			return err
		}
	}
	task, err := vm.Delete(ctx, &pve.VirtualMachineDeleteOptions{Purge: true, DestroyUnreferencedDisks: true})
	if err != nil {
		return fmt.Errorf("delete vm %d: %w", obj.VMID, err)
	}
	if err := task.Wait(ctx, time.Second, guestTimeout); err != nil {
		return fmt.Errorf("delete vm %d: %w", obj.VMID, err)
	}
	return nil
}

// UpdateGuest applies safe drift corrections (cores, memory) and reconciles the
// power state.
func (c *PVE) UpdateGuest(ctx context.Context, upd GuestUpdate) error {
	vm, err := c.vm(ctx, upd.Node, upd.VMID)
	if err != nil {
		return err
	}
	task, err := vm.Config(ctx,
		pve.VirtualMachineOption{Name: "cores", Value: upd.Cores},
		pve.VirtualMachineOption{Name: "memory", Value: upd.MemoryMB},
	)
	if err != nil {
		return fmt.Errorf("configure vm %d: %w", upd.VMID, err)
	}
	if err := task.Wait(ctx, time.Second, guestTimeout); err != nil {
		return fmt.Errorf("configure vm %d: %w", upd.VMID, err)
	}
	if upd.Running != vm.IsRunning() {
		return c.setPower(ctx, upd.Node, upd.VMID, upd.Running)
	}
	return nil
}

// RebootGuest restarts a VM so pending config changes take effect. Proxmox
// applies pending config when the QEMU process is recreated, so this cold-cycles
// the VM (stop then start) rather than issuing an in-guest ACPI reboot.
func (c *PVE) RebootGuest(ctx context.Context, node string, vmid int) error {
	if err := c.setPower(ctx, node, vmid, false); err != nil {
		return err
	}
	return c.setPower(ctx, node, vmid, true)
}

// vm resolves a VirtualMachine handle by node and vmid.
func (c *PVE) vm(ctx context.Context, node string, vmid int) (*pve.VirtualMachine, error) {
	n, err := c.api.Node(ctx, node)
	if err != nil {
		return nil, fmt.Errorf("get node %s: %w", node, err)
	}
	vm, err := n.VirtualMachine(ctx, vmid)
	if err != nil {
		return nil, fmt.Errorf("get vm %d: %w", vmid, err)
	}
	return vm, nil
}

// setPower starts or stops a VM and waits for the task.
func (c *PVE) setPower(ctx context.Context, node string, vmid int, running bool) error {
	vm, err := c.vm(ctx, node, vmid)
	if err != nil {
		return err
	}
	var task *pve.Task
	verb := "start"
	if running {
		task, err = vm.Start(ctx)
	} else {
		verb = "stop"
		task, err = vm.Stop(ctx)
	}
	if err != nil {
		return fmt.Errorf("%s vm %d: %w", verb, vmid, err)
	}
	if err := task.Wait(ctx, time.Second, guestTimeout); err != nil {
		return fmt.Errorf("%s vm %d: %w", verb, vmid, err)
	}
	return nil
}

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

// DeleteISO removes an ISO from a storage and waits for the task to complete.
func (c *PVE) DeleteISO(ctx context.Context, node, storageName, filename string) error {
	storage, err := c.storage(ctx, node, storageName)
	if err != nil {
		return err
	}
	iso, err := storage.ISO(ctx, filename)
	if err != nil {
		return fmt.Errorf("get iso %s: %w", filename, err)
	}
	task, err := iso.Delete(ctx)
	if err != nil {
		return fmt.Errorf("delete %s: %w", filename, err)
	}
	if err := task.Wait(ctx, time.Second, deleteTimeout); err != nil {
		return fmt.Errorf("delete %s: %w", filename, err)
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

// parseSizeGB reads a leading integer from a disk size such as "40G" or "40GB"
// and returns it as gibibytes. It falls back to a small default when the value
// is empty or unparseable, so a create never asks Proxmox for a 0-sized disk.
func parseSizeGB(size string) int {
	n := 0
	for _, r := range size {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	if n <= 0 {
		return 8
	}
	return n
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
