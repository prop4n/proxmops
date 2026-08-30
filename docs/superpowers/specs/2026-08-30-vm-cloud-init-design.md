# VM cloud-init from a cloud image

**Date:** 2026-08-30
**Status:** approved, implementing (increment 1)

## Goal

Create a usable VM from a cloud image URL with cloud-init: SSH keys, user, IP,
DNS. The mechanism is API-only, so it works whether proxmops runs on a node or
off it (single code path). Templates and clone come in later increments.

## Verified mechanism (spike on live PVE 9.2)

1. `download-url` with **content=import** fetches the cloud image to
   `<storage>:import/<file>` (a directory storage with the `import` content type,
   e.g. `local`).
2. Create the VM.
3. `scsiN = <images-storage>:0,import-from=<storage>:import/<file>` imports the
   image as the VM's disk (async task).
4. `ide2 = <images-storage>:cloudinit` adds the cloud-init drive.
5. Cloud-init params are native config options: `ciuser`, `cipassword`,
   `sshkeys` (via `EncodeSSHKeys`), `ipconfig0`, `nameserver`, `searchdomain`.
6. `boot = order=scsiN`, optional disk resize, then start.

## Manifest

Extend `VirtualMachineSpec`:

```yaml
spec:
  vmid: 9001
  cores: 2
  memory: 2048
  image:
    source: https://.../debian-12-genericcloud.qcow2
    importStorage: local        # optional; a storage with `import` content
  disks:
    - storage: local-lvm        # target for the imported disk
      size: 20G                 # optional resize after import
  net:
    - bridge: vmbr0
  cloudInit:
    user: debian
    password: "..."             # optional
    sshKeys:
      - ssh-ed25519 AAAA...
    ip: dhcp                     # or "ip=10.0.0.5/24,gw=10.0.0.1"
    nameserver: 1.1.1.1         # optional
    searchDomain: lan           # optional
  state: running
```

`image.importStorage` defaults to the first directory storage advertising the
`import` content type; if none, creation fails with a clear error.

## Design

- **manifest**: add `Image` (Source, ImportStorage) and `CloudInit` (User,
  Password, SSHKeys, IP, Nameserver, SearchDomain) to `VirtualMachineSpec`.
- **proxmox.GuestSpec**: add the image source/import-storage and a flat
  `CloudInit` struct. New `GuestStore` capability isn't needed beyond the
  existing `CreateGuest`, which branches on whether an image source is set:
  - image set: download-import (dedup by filename), create, import-from, add
    cloud-init drive, set params, resize if larger, start if running.
  - image unset: current blank-VM path.
- **filename derivation**: reuse the URL-basename logic (shared with ISO).
- **download dedup**: skip the import download when the target VM already exists
  (create only runs when absent) and when the import volume is already present.
- **reconcile/guest.go**: build the richer `GuestSpec` from the manifest. Create
  when absent as today. Cloud-init/image fields are create-time only in v1 (no
  drift correction on them yet); cores/memory/state drift unchanged.
- **ssh keys**: joined and encoded with the SDK's `EncodeSSHKeys`.

## Non-goals (later)

- Templates (declare + convert) and clone-from-template.
- Drift correction on cloud-init params (rotate SSH key, change IP).
- Cleanup/GC of the downloaded import volume.
- LXC containers.

## Risks

- Import storage discovery: the target for `import-from` source must have the
  `import` content type. If the user has none, report a clear error rather than
  guessing.
- Disk resize must never shrink (Proxmox refuses); only grow when the requested
  size exceeds the imported image.
