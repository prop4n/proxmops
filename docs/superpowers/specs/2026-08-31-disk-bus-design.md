# Configurable disk bus (scsi / virtio)

**Date:** 2026-08-31
**Status:** approved, implementing

## Problem

proxmops always attaches a VM's primary disk on the virtio-scsi controller
(`scsi0`), so the guest sees it as `/dev/sda`. Images that expect virtio-blk
(`/dev/vda`) — e.g. a `guix system image` qcow2 whose bootloader targets
`/dev/vda` — fail to install their bootloader on reconfigure. The bus must be
selectable.

## Manifest

`disks[].bus`, values `scsi` (default) or `virtio`:

```yaml
disks:
  - storage: local-lvm
    size: 20G
    bus: virtio        # guest sees /dev/vda; default scsi -> /dev/sda
```

Applies to VirtualMachine (blank and image) and Template. A clone inherits the
disk (and its bus) from the template, so the clone need not repeat it.

## Code

- **manifest:** `Disk.Bus string`. Validate rejects a value other than "",
  "scsi", "virtio".
- **proxmox:** `GuestDisk.Bus`. A helper `diskSlot(bus) -> "virtio0" | "scsi0"`.
  - Blank create: inline disk on `diskSlot`.
  - `provisionCloudImage`: import-from targets `diskSlot`; boot order
    `order=<slot>`; resize `<slot>`.
  - `cloneFromTemplate`: the cloned disk keeps the template's slot, so detect it
    from the cloned config (`virtio0` present -> virtio0, else scsi0) and use that
    slot for the cloud-init storage lookup and the resize.
- **reconcile:** thread `Bus` through `desiredSpec` and `templateSpec`.

## Non-goals

- Changing the bus of an existing disk (would require detach/reattach); the bus
  is set at build time and inherited by clones.
- Multiple disks (still only disks[0]).

## Risks

- The clone path currently hardcodes `scsi0`; it must detect the slot or a
  virtio-built template's clone breaks. Covered by slot detection.
