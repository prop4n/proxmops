# VM lifecycle: create / update / delete

**Date:** 2026-08-30
**Status:** approved, implementing (increment 1)

## Goal

Make QEMU VMs a first-class reconciled resource: proxmops creates them from the
manifest, deletes owned ones dropped from the repo (or via the UI button), and
corrects safe config drift. No cloud-init yet — this is the plain-VM base that
cloud-init and fine-grained params will build on later.

## Scope (increment 1)

**Create** a VM on `metadata.node` from `VirtualMachineSpec`:
- `vmid`, `cores`, `memory`
- one disk (`disks[0]`: storage + size)
- one NIC (`net[0]`: bridge + model)
- boot ISO mounted when `spec.iso` is set
- tag `managed-by:proxmops`
- start it when `spec.state: running`

**Delete** (`DeleteGuest`): stop if running, then destroy. Owned VMs only. This
also enables the currently-disabled UI delete button for VMs.

**Update on drift** — split by safety:
- **Auto-corrected** (safe, via `qm set` / config update): `cores`, `memory`,
  and power `state` (start/stop). Non-destructive.
- **Detected but NOT auto-applied** in v1: disk resize/add/remove and NIC
  changes. Reported as drift only — too destructive to automate yet.

## Design

- **`proxmox` package**
  - `Object` gains observed VM config for diffing: `Cores`, `Memory`, `Status`
    (running/stopped). Populated by `ListGuests` from the cluster resource list.
  - `GuestStore`:
    - `CreateGuest(ctx, GuestSpec)` — create + configure + tag + optional start.
    - `DeleteGuest(ctx, Object)` — stop + destroy.
    - `UpdateGuest(ctx, GuestUpdate)` — apply cores/memory; start/stop for state.
  - A new `GuestSpec` carries the create parameters (kind, node, vmid, name,
    cores, memory, disk, nic, iso, running, tags) so the reconciler passes a
    flat value, not a manifest type (keeps proxmox free of the manifest import).

- **`reconcile/guest.go`**
  - Build the desired `GuestSpec` from the manifest.
  - Create when absent; delete owned orphans (prune-gated as today).
  - When present and owned: diff safe fields (cores, memory, state) against the
    observed `Object`; emit an `update` action listing what changed. Disk/NIC
    differences are surfaced in the reason but not applied.

- **`manifest`**: current `VirtualMachineSpec` is sufficient.

- **status/UI**: an `update` action shows as OutOfSync with a reason; the VM
  delete button lights up once `DeleteGuest` exists.

## Non-goals (later increments)

- cloud-init (clone template / import cloud image, ssh keys, IP config).
- LXC containers.
- Disk and NIC drift correction.
- Live migration, snapshots.

## Risks

- go-proxmox create/config/start API shape — verify the exact calls and task
  waiting against the SDK and the live cluster before finalising.
- vmid collisions with pre-existing, unmanaged guests: never touch a guest that
  lacks the ownership tag; a desired vmid already used by an unowned guest is a
  reported error, not an overwrite.
