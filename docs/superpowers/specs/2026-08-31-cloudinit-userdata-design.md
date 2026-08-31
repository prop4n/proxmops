# Arbitrary cloud-init user-data via a generated NoCloud ISO

**Date:** 2026-08-31
**Status:** approved (scope B), implementing

## Problem

Proxmox's cloud-init options only set native fields (ciuser, sshkeys, ipconfig0,
nameserver). They cannot carry arbitrary NoCloud `user-data`. Consumers that read
the raw user-data (guix-metadata reading `((system-file . "..."))`, or any
NoCloud-aware image) have no way in.

Proxmox's only native path for custom user-data is `cicustom` pointing at a
snippet file, and snippets cannot be written through the API (no upload endpoint);
they need filesystem/SSH access to the node. proxmops is API-only and stays that
way.

## Approach (proven by spike)

Deliver arbitrary user-data as a **generated NoCloud seed ISO**, 100% over the
API, no SSH:

1. Generate an ISO9660 labelled `CIDATA` holding `user-data` (the payload) and
   `meta-data` (instance-id/hostname), with go-diskfs.
2. Upload it to an iso-content storage via the Proxmox upload endpoint (a raw
   multipart POST; the go-proxmox SDK's Upload helper is broken).
3. Attach it as a cdrom (`ide0`). A NoCloud consumer mounts the `CIDATA` disk and
   reads `user-data` at boot.

The spike verified both hard parts on the live cluster: go-diskfs produced a
mountable CIDATA ISO, and the upload returned HTTP 200 with the file landing on
`local`.

## Manifest

A top-level `spec.userData` (a raw NoCloud user-data string), separate from
`cloudInit`:

```yaml
spec:
  fromTemplate: guix-homelab-template
  userData: |
    #cloud-config
    ((system-file . "systems/web01.scm"))
  applyMode: reboot        # so a userData change is applied by a reboot
  state: running
```

**Why top-level, not `cloudInit.userData`:** `cloudInit` (user/ip/sshkeys) renders
the Proxmox cloud-init drive, itself a `CIDATA` disk. A generated `userData` ISO is
also a `CIDATA` disk. Two `CIDATA` disks conflict, so the two mechanisms are
mutually exclusive; a separate top-level field makes that explicit rather than
nesting a field that silently disables its siblings.

**Validation:**
- `userData` and `cloudInit` are mutually exclusive.
- `userData` and `iso` are mutually exclusive (both attach the ide0 cdrom;
  `userData` generates that ISO, `iso` supplies a ready one).

## Drift and re-sync (scope B)

The user-data is a first-boot input, but a change should still be applied:

- The generated ISO is named with a short content hash:
  `proxmops-cidata-<vmid>-<hash8>.iso`. The attached `ide0` volume therefore
  encodes which user-data is live.
- On each pass, the reconciler compares the desired user-data hash against the
  hash in the observed `ide0` volume. On a mismatch it regenerates + uploads the
  new ISO, re-attaches `ide0`, and — because a NoCloud disk is only read at boot —
  reboots the VM. The reboot is gated by `applyMode: reboot`; otherwise it reports
  "reboot required to apply new user-data".
- The old ISO is deleted after the new one is attached; the VM's ISO is also
  deleted when the VM is deleted, so no orphans accumulate.

## Code

- **manifest/vm.go:** `VirtualMachineSpec.UserData string`. Validation for the two
  mutual-exclusions above.
- **proxmox/cidata.go (new):** `buildCidataISO(userData) (bytes, hash)` with
  go-diskfs; `uploadISO(ctx, node, storage, filename, bytes)` via raw multipart;
  `resolveISOStorage(ctx, node)` (first storage with iso content); a helper to
  delete an ISO volume.
- **proxmox/client.go:** `GuestSpec.UserData`; observed `Object.CidataHash` (parsed
  from ide0). `GuestUpdate` carries a re-provision/reboot intent for user-data.
- **proxmox/pve.go:** on create (blank/image/clone), when UserData set, build +
  upload + attach ide0, and skip the Proxmox cloud-init drive. Read ide0 into the
  observed config so drift can compare the hash.
- **reconcile/guest.go:** thread UserData through desiredSpec; add a drift branch
  that fires when the desired hash differs from observed, producing a reboot
  action (or an informational "reboot required").
- **app/app.go:** on resource delete, also delete the VM's cidata ISO.
- **go.mod:** promote `github.com/diskfs/go-diskfs` to a direct dependency.

## Non-goals

- Snippets / cicustom (needs SSH; rejected).
- meta-data beyond instance-id/hostname.
- Editing user-data without a reboot (a NoCloud disk is read at boot).

## Risks

- The SDK upload is unusable; the raw multipart must match what Proxmox expects
  (verified working with curl: field `content=iso`, file field `filename`).
- Storage selection: needs an iso-content storage on the node; resolve it, error
  clearly when none exists.
- Two CIDATA disks: prevented by making userData and cloudInit mutually exclusive
  and skipping the Proxmox cloud-init drive when userData is set.
