package proxmox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"

	diskfs "github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/filesystem"
	"github.com/diskfs/go-diskfs/filesystem/iso9660"
	pve "github.com/luthermonson/go-proxmox"
)

// cidataVolumeLabel is the filesystem label a NoCloud datasource carries; cloud
// consumers (cloud-init, guix-metadata) mount the disk by this label.
const cidataVolumeLabel = "CIDATA"

// buildCidataISO builds a NoCloud seed ISO holding user-data and meta-data,
// labelled CIDATA. It returns the ISO bytes and a short hex hash of the payload,
// so callers can name the volume by content and detect changes. instanceID seeds
// the meta-data instance-id and hostname.
func buildCidataISO(instanceID, userData string) ([]byte, string, error) {
	metaData := fmt.Sprintf("instance-id: %s\nlocal-hostname: %s\n", instanceID, instanceID)

	tmp, err := os.CreateTemp("", "proxmops-cidata-*.iso")
	if err != nil {
		return nil, "", err
	}
	path := tmp.Name()
	tmp.Close()
	// diskfs.Create needs to create the device file itself.
	os.Remove(path)
	defer os.Remove(path)

	// ISO9660 requires a 2048-byte sector; 4 MiB is ample for a seed.
	d, err := diskfs.Create(path, 4*1024*1024, diskfs.SectorSize(2048))
	if err != nil {
		return nil, "", fmt.Errorf("create iso: %w", err)
	}
	fs, err := d.CreateFilesystem(disk.FilesystemSpec{Partition: 0, FSType: filesystem.TypeISO9660, VolumeLabel: cidataVolumeLabel})
	if err != nil {
		return nil, "", fmt.Errorf("create iso fs: %w", err)
	}
	for name, content := range map[string]string{"/user-data": userData, "/meta-data": metaData} {
		f, err := fs.OpenFile(name, os.O_CREATE|os.O_RDWR)
		if err != nil {
			return nil, "", fmt.Errorf("write %s: %w", name, err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			return nil, "", fmt.Errorf("write %s: %w", name, err)
		}
	}
	isofs, ok := fs.(*iso9660.FileSystem)
	if !ok {
		return nil, "", fmt.Errorf("unexpected filesystem type")
	}
	if err := isofs.Finalize(iso9660.FinalizeOptions{RockRidge: true, Joliet: true, VolumeIdentifier: cidataVolumeLabel}); err != nil {
		return nil, "", fmt.Errorf("finalize iso: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	return data, CidataHash(userData), nil
}

// CidataHash is the short content hash used to name the ISO and detect drift.
func CidataHash(userData string) string {
	sum := sha256.Sum256([]byte(userData))
	return hex.EncodeToString(sum[:])[:8]
}

// cidataFilename is the deterministic ISO name for a VM's user-data: the VMID
// plus the content hash, so a change produces a new name the reconciler can spot.
func cidataFilename(vmid int, hash string) string {
	return fmt.Sprintf("proxmops-cidata-%d-%s.iso", vmid, hash)
}

// provisionCidata builds the CIDATA seed ISO from spec.UserData, uploads it, and
// attaches it as the ide0 cdrom. A no-op when no user-data is set.
func (c *PVE) provisionCidata(ctx context.Context, node *pve.Node, spec GuestSpec) error {
	if spec.UserData == "" {
		return nil
	}
	iso, hash, err := buildCidataISO(spec.Name, spec.UserData)
	if err != nil {
		return fmt.Errorf("build cidata for vm %d: %w", spec.VMID, err)
	}
	storage, err := c.resolveISOStorage(ctx, node, "")
	if err != nil {
		return err
	}
	filename := cidataFilename(spec.VMID, hash)
	if err := c.uploadISO(ctx, spec.Node, storage, filename, iso); err != nil {
		return err
	}
	vm, err := node.VirtualMachine(ctx, spec.VMID)
	if err != nil {
		return fmt.Errorf("get vm %d: %w", spec.VMID, err)
	}
	volid := fmt.Sprintf("%s:iso/%s,media=cdrom", storage, filename)
	if err := c.configWait(ctx, vm, pve.VirtualMachineOption{Name: "ide0", Value: volid}); err != nil {
		return fmt.Errorf("attach cidata for vm %d: %w", spec.VMID, err)
	}
	return nil
}

// SyncUserData re-provisions a VM's cidata ISO to match spec.UserData: it uploads
// and attaches the new seed ISO, then deletes the previous one. The caller reboots
// the VM to make it re-read the disk.
func (c *PVE) SyncUserData(ctx context.Context, spec GuestSpec) error {
	node, err := c.api.Node(ctx, spec.Node)
	if err != nil {
		return fmt.Errorf("get node %s: %w", spec.Node, err)
	}
	oldStorage, oldFile := "", ""
	if vm, err := node.VirtualMachine(ctx, spec.VMID); err == nil {
		oldStorage, oldFile = parseISOVolume(vm.VirtualMachineConfig.IDEs["ide0"])
	}
	if err := c.provisionCidata(ctx, node, spec); err != nil {
		return err
	}
	// Remove the previous seed once the new one is attached, so ISOs don't pile
	// up. Only proxmops-generated seeds are touched.
	newFile := cidataFilename(spec.VMID, CidataHash(spec.UserData))
	if oldFile != "" && oldFile != newFile && strings.HasPrefix(oldFile, "proxmops-cidata-") {
		_ = c.DeleteISO(ctx, spec.Node, oldStorage, oldFile)
	}
	return nil
}

// parseISOVolume splits an ide cdrom value like
// "local:iso/foo.iso,media=cdrom" into its storage and filename, empty when it is
// not a storage ISO reference.
func parseISOVolume(ide string) (storage, filename string) {
	ide, _, _ = strings.Cut(ide, ",")
	storage, rest, ok := strings.Cut(ide, ":iso/")
	if !ok {
		return "", ""
	}
	return storage, rest
}

// resolveISOStorage returns the requested storage, or the first one on the node
// advertising the "iso" content type.
func (c *PVE) resolveISOStorage(ctx context.Context, node *pve.Node, want string) (string, error) {
	if want != "" {
		return want, nil
	}
	s, err := node.StorageISO(ctx)
	if err != nil {
		return "", fmt.Errorf("no iso-content storage on node %s: %w", node.Name, err)
	}
	return s.Name, nil
}

// uploadISO uploads ISO bytes to a storage via the Proxmox upload endpoint. The
// go-proxmox SDK's Upload mis-formats the multipart body, so this posts it
// directly (verified against the API), then waits for the resulting task.
func (c *PVE) uploadISO(ctx context.Context, node, storage, filename string, data []byte) error {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if err := w.WriteField("content", isoContent); err != nil {
		return err
	}
	part, err := w.CreateFormFile("filename", filename)
	if err != nil {
		return err
	}
	if _, err := part.Write(data); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/nodes/%s/storage/%s/upload", c.cluster.Endpoint, node, storage)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s=%s", c.cluster.TokenID, c.cluster.TokenSecret))

	client := &http.Client{Timeout: downloadTimeout}
	if c.cluster.InsecureSkipVerify {
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} //nolint:gosec // opt-in for self-signed PVE
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("upload iso: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upload iso: %s: %s", resp.Status, string(raw))
	}

	// The endpoint returns the UPID of an imgcopy task; wait for it.
	var out struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.Data == "" {
		return nil // no task to wait on; the upload already returned 200
	}
	task := pve.NewTask(pve.UPID(out.Data), c.api)
	return waitTask(ctx, task, time.Second, downloadTimeout)
}
