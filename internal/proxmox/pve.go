package proxmox

import (
	"context"
	"fmt"
	"path"
	"slices"
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
			Kind:       kind,
			Name:       r.Name,
			Node:       r.Node,
			ID:         r.ID,
			VMID:       int(r.VMID),
			Running:    r.Status == "running",
			IsTemplate: r.Template == 1,
			Tags:       parseTags(r.Tags),
		}
		// Configured cores/memory come from the VM config (authoritative for
		// drift); the cluster listing only carries the live, running values.
		// When they differ on a running VM, a change is pending a restart.
		liveCores, liveMemMB := int(r.MaxCPU), int(r.MaxMem/(1024*1024))
		o.Cores, o.MemoryMB = liveCores, liveMemMB
		if kind == KindVirtualMachine {
			if cfg, err := c.vmConfig(ctx, r.Node, int(r.VMID)); err == nil {
				o.Cores, o.MemoryMB, o.CPU = cfg.cores, cfg.memMB, cfg.cpu
				o.CIUser, o.IP, o.Nameserver, o.SearchDomain = cfg.ciUser, cfg.ip, cfg.nameserver, cfg.searchDomain
				o.RebootPending = o.Running && (cfg.cores != liveCores || cfg.memMB != liveMemMB)
			}
		}
		objects = append(objects, o)
	}
	return objects, nil
}

// vmObservedConfig is a VM's configured values relevant to drift.
type vmObservedConfig struct {
	cores        int
	memMB        int
	cpu          string
	ciUser       string
	ip           string
	nameserver   string
	searchDomain string
}

// vmConfig reads the VM config values proxmops reconciles: cpu/cores/memory and
// the cloud-init scalars.
func (c *PVE) vmConfig(ctx context.Context, node string, vmid int) (vmObservedConfig, error) {
	vm, err := c.vm(ctx, node, vmid)
	if err != nil {
		return vmObservedConfig{}, err
	}
	cfg := vm.VirtualMachineConfig
	return vmObservedConfig{
		cores:        derefOr(cfg.Cores, 1) * derefOr(cfg.Sockets, 1),
		memMB:        int(cfg.Memory),
		cpu:          cfg.CPU,
		ciUser:       cfg.CIUser,
		ip:           cfg.IPConfigs["ipconfig0"],
		nameserver:   cfg.Nameserver,
		searchDomain: cfg.Searchdomain,
	}, nil
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
	if spec.CPU != "" {
		opts = append(opts, pve.VirtualMachineOption{Name: "cpu", Value: spec.CPU})
	}
	// A blank disk is created inline; an image-backed disk is imported after
	// create (it needs the VM to exist first).
	if spec.Image == nil && spec.Disk.Storage != "" {
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

	if spec.Image != nil {
		if err := c.provisionCloudImage(ctx, node, spec); err != nil {
			return err
		}
	}

	if spec.Running {
		return c.setPower(ctx, spec.Node, spec.VMID, true)
	}
	return nil
}

// provisionCloudImage imports the cloud image as scsi0, adds the cloud-init
// drive, applies the cloud-init settings, and resizes the disk if requested. The
// VM must already exist. This path is API-only, so it works on or off the node.
func (c *PVE) provisionCloudImage(ctx context.Context, node *pve.Node, spec GuestSpec) error {
	if spec.Disk.Storage == "" {
		return fmt.Errorf("vm %d: disks[0].storage is required for a cloud image", spec.VMID)
	}

	// The image is either a remote URL (download to an import storage first) or a
	// reference to a volume already on a storage (use it directly).
	var importVolume string
	if isVolumeRef(spec.Image.Source) {
		importVolume = spec.Image.Source
	} else {
		importStorage, err := c.resolveImportStorage(ctx, node, spec.Image.ImportStorage)
		if err != nil {
			return err
		}
		if err := c.downloadImport(ctx, node, importStorage, spec.Image.Filename, spec.Image.Source); err != nil {
			return err
		}
		importVolume = importStorage + ":import/" + spec.Image.Filename
	}

	vm, err := node.VirtualMachine(ctx, spec.VMID)
	if err != nil {
		return fmt.Errorf("get vm %d: %w", spec.VMID, err)
	}

	// Import the image as scsi0.
	importFrom := fmt.Sprintf("%s:0,import-from=%s", spec.Disk.Storage, importVolume)
	if err := c.configWait(ctx, vm, pve.VirtualMachineOption{Name: "scsi0", Value: importFrom}); err != nil {
		return fmt.Errorf("import disk for vm %d: %w", spec.VMID, err)
	}

	// Boot order + serial console. Cloud images boot with console=ttyS0, so a
	// serial device is required or init dies early; ostype l26 sets the right
	// Linux defaults. A template is bare: no cloud-init drive.
	opts := []pve.VirtualMachineOption{
		{Name: "boot", Value: "order=scsi0"},
		{Name: "serial0", Value: "socket"},
		{Name: "vga", Value: "serial0"},
		{Name: "ostype", Value: "l26"},
	}
	if !spec.AsTemplate {
		opts = append(opts, pve.VirtualMachineOption{Name: "ide2", Value: spec.Disk.Storage + ":cloudinit"})
	}
	ci := spec.CloudInit
	if ci != nil {
		if ci.User != "" {
			opts = append(opts, pve.VirtualMachineOption{Name: "ciuser", Value: ci.User})
		}
		if ci.Password != "" {
			opts = append(opts, pve.VirtualMachineOption{Name: "cipassword", Value: ci.Password})
		}
		if len(ci.SSHKeys) > 0 {
			opts = append(opts, pve.VirtualMachineOption{Name: "sshkeys", Value: pve.EncodeSSHKeys(ci.SSHKeys...)})
		}
		if ci.IP != "" {
			// Proxmox wants key=value ("ip=dhcp"); accept a bare mode like "dhcp".
			ip := ci.IP
			if !strings.Contains(ip, "=") {
				ip = "ip=" + ip
			}
			opts = append(opts, pve.VirtualMachineOption{Name: "ipconfig0", Value: ip})
		}
		if ci.Nameserver != "" {
			opts = append(opts, pve.VirtualMachineOption{Name: "nameserver", Value: ci.Nameserver})
		}
		if ci.SearchDomain != "" {
			opts = append(opts, pve.VirtualMachineOption{Name: "searchdomain", Value: ci.SearchDomain})
		}
	}
	if err := c.configWait(ctx, vm, opts...); err != nil {
		return fmt.Errorf("cloud-init config for vm %d: %w", spec.VMID, err)
	}

	// Grow the imported disk if a larger size is requested. Proxmox refuses to
	// shrink, so a smaller value surfaces as an error rather than data loss.
	if spec.Disk.Size != "" {
		task, err := vm.ResizeDisk(ctx, "scsi0", spec.Disk.Size)
		if err != nil {
			return fmt.Errorf("resize disk for vm %d: %w", spec.VMID, err)
		}
		if err := task.Wait(ctx, time.Second, guestTimeout); err != nil {
			return fmt.Errorf("resize disk for vm %d: %w", spec.VMID, err)
		}
	}
	return nil
}

// isVolumeRef reports whether an image source is an existing Proxmox volume
// (e.g. "local:import/debian.qcow2") rather than a URL to download.
func isVolumeRef(source string) bool {
	return !strings.Contains(source, "://")
}

// ConvertToTemplate turns a built VM into a template. It is idempotent: a VM
// that is already a template is left as is.
func (c *PVE) ConvertToTemplate(ctx context.Context, node string, vmid int) error {
	vm, err := c.vm(ctx, node, vmid)
	if err != nil {
		return err
	}
	if vm.Template {
		return nil
	}
	task, err := vm.ConvertToTemplate(ctx)
	if err != nil {
		return fmt.Errorf("convert vm %d to template: %w", vmid, err)
	}
	if err := task.Wait(ctx, time.Second, guestTimeout); err != nil {
		return fmt.Errorf("convert vm %d to template: %w", vmid, err)
	}
	return nil
}

// configWait applies config options and waits for the resulting task.
func (c *PVE) configWait(ctx context.Context, vm *pve.VirtualMachine, opts ...pve.VirtualMachineOption) error {
	task, err := vm.Config(ctx, opts...)
	if err != nil {
		return err
	}
	return task.Wait(ctx, time.Second, guestTimeout)
}

// resolveImportStorage returns the requested import storage, or the first one on
// the node advertising the "import" content type.
func (c *PVE) resolveImportStorage(ctx context.Context, node *pve.Node, want string) (string, error) {
	if want != "" {
		return want, nil
	}
	storages, err := node.Storages(ctx)
	if err != nil {
		return "", fmt.Errorf("list storages: %w", err)
	}
	for _, s := range storages {
		if slices.Contains(strings.Split(s.Content, ","), "import") {
			return s.Name, nil
		}
	}
	return "", fmt.Errorf("no storage with the 'import' content type; set spec.image.importStorage")
}

// downloadImport fetches the cloud image into the import storage unless it is
// already present, so re-creating a VM does not re-download.
func (c *PVE) downloadImport(ctx context.Context, node *pve.Node, storageName, filename, url string) error {
	storage, err := node.Storage(ctx, storageName)
	if err != nil {
		return fmt.Errorf("get storage %s: %w", storageName, err)
	}
	content, err := storage.GetContent(ctx)
	if err != nil {
		return fmt.Errorf("get content of %s: %w", storageName, err)
	}
	want := storageName + ":import/" + filename
	for _, item := range content {
		if item.Volid == want {
			return nil // already downloaded
		}
	}
	task, err := storage.DownloadURL(ctx, "import", filename, url)
	if err != nil {
		return fmt.Errorf("download %s: %w", filename, err)
	}
	if err := task.Wait(ctx, 2*time.Second, downloadTimeout); err != nil {
		return fmt.Errorf("download %s: %w", filename, err)
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
	opts := []pve.VirtualMachineOption{
		{Name: "cores", Value: upd.Cores},
		{Name: "memory", Value: upd.MemoryMB},
	}
	if upd.CPU != "" {
		opts = append(opts, pve.VirtualMachineOption{Name: "cpu", Value: upd.CPU})
	}
	if upd.CIUser != "" {
		opts = append(opts, pve.VirtualMachineOption{Name: "ciuser", Value: upd.CIUser})
	}
	if upd.IP != "" {
		opts = append(opts, pve.VirtualMachineOption{Name: "ipconfig0", Value: upd.IP})
	}
	if upd.Nameserver != "" {
		opts = append(opts, pve.VirtualMachineOption{Name: "nameserver", Value: upd.Nameserver})
	}
	if upd.SearchDomain != "" {
		opts = append(opts, pve.VirtualMachineOption{Name: "searchdomain", Value: upd.SearchDomain})
	}
	task, err := vm.Config(ctx, opts...)
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
