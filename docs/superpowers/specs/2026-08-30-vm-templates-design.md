# VM templates and local images

**Date:** 2026-08-30
**Status:** approved, implementing increment A

Delivered in two increments:
- **A (this spec):** reference a local (already-on-storage) image, and a new
  `Template` kind that builds a golden image and converts it to a template.
- **B (later):** a VM that clones a Template, overriding cloud-init per clone.

## Increment A

### 1. Local image reference

`spec.image.source` accepts two forms, distinguished by `://`:

- `https://...` — downloaded via download-url (current behaviour).
- `local:import/debian-12.qcow2` — an existing Proxmox volume; the download is
  skipped and `import-from` uses it directly.

A helper `isVolumeRef(s)` returns true when `s` has no `://`. When it is a volume
ref, `provisionCloudImage` skips `downloadImport` and derives the import storage
and volume directly from the ref. No new manifest field; backward compatible.

### 2. Template kind

A template is a VM built from an image, then converted to a template
(`ConvertToTemplate`). It is "bare": hardware + image only, no cloud-init (that is
set per clone in increment B).

```yaml
apiVersion: proxmops.dev/v1
kind: Template
metadata:
  name: debian-12-tpl
  node: pve
spec:
  vmid: 9000
  image:
    source: https://.../debian-12-genericcloud-amd64.qcow2   # or local:import/...
  cores: 2
  memory: 2048
  cpu: x86-64-v2            # optional
  disks:
    - storage: local-lvm
      size: 10G
  net:
    - bridge: vmbr0
```

**Manifest** (`manifest/template.go`): `Template` with `TemplateSpec` (VMID,
Image, Cores, Memory, CPU, Disks, Net). Validate requires node, positive vmid,
and image.source. Add `KindTemplate` and a loader case.

**Proxmox layer:**
- `proxmox.ConvertToTemplate(ctx, node, vmid)` wraps the SDK call and waits.
- `GuestSpec` is reused for the build (no Running, no CloudInit). Add an
  `AsTemplate bool` so `CreateGuest` skips the cloud-init drive and never starts.
- `provisionCloudImage` gains the local-volume branch (see §1).
- `Object` already carries enough for ownership; add an observed `IsTemplate`
  flag (from the cluster resource `Template` field) so the reconciler can tell a
  finished template from a half-built VM.

**Reconciler** (`reconcile/template.go`, tag-owned like guests, keyed by VMID):
- absent → build the VM from the image (reuse the create path with
  `AsTemplate`), then convert to template.
- present and already a template → in sync.
- present but not yet a template (interrupted build) → convert to template.
- owned template dropped from the repo → delete (prune-gated).
- no spec-level drift in A.

**Engine:** register the template reconciler; report `Template` in status kinds.

### Scope A

Build + convert + delete templates; image by URL or local volume. No clone (B),
no template spec drift, no containers.

## Increment B (later, not now)

A VM with `spec.fromTemplate: <name-or-vmid>` clones the template
(`Clone`), then applies cloud-init overrides. Cloud-init values live on the
cloning VM, not the template.

## Non-goals

- Clone (increment B).
- Template spec drift correction.
- LXC.

## Risks

- Distinguishing a finished template from a mid-build VM: rely on the observed
  `IsTemplate` flag, and make convert idempotent (skip if already a template).
- A template must not be started; ensure the create path honours `AsTemplate`
  (no start, no cloud-init drive) so conversion doesn't fail on a running VM.
