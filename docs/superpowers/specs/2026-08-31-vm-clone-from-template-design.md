# Clone a VM from a template (templates increment B)

**Date:** 2026-08-31
**Status:** approved, implementing

Increment A gave us templates (build a bare golden image, convert to a Proxmox
template). This increment lets a VM be created by cloning one of those templates
and then applying its own cloud-init, so a single template produces many VMs.

## Manifest

A new `fromTemplate` field on `VirtualMachine`, accepting a scalar or an object:

```yaml
spec:
  fromTemplate: debian-12-tpl          # shorthand: full clone by name
```

```yaml
spec:
  fromTemplate:
    name: debian-12-tpl
    linked: true                       # optional, default is a full clone
```

A custom `UnmarshalYAML` on the `FromTemplate` type accepts either a string
(name, full clone) or a mapping (`name`, `linked`).

Rules:
- `fromTemplate` and `image` are mutually exclusive (build from an image, or
  clone a template, not both). Validation rejects setting both.
- `cloudInit` is allowed with `fromTemplate`; that is the point of cloning.
- `fromTemplate.name` is required when `fromTemplate` is set.

## Template resolution

`fromTemplate.name` refers to a `Template` declared in the same repository. The
guest reconciler builds a `name -> vmid` map from the desired `Template`
manifests and resolves the clone source at plan time. If the name is not declared,
the create action fails with a clear error ("template %q is not declared"). If it
is declared but not yet built on the cluster, the clone fails and is retried on
the next pass; the template reconciler runs first, so the template is built first.

## Proxmox layer

Add to `proxmox`:
- `CloneSpec{Node, TemplateVMID, NewVMID, Name, Full bool, Storage string}`.
- `CloneGuest(ctx, CloneSpec) error`: calls the SDK `Clone` with
  `VirtualMachineCloneOptions{NewID, Full, Name, Storage}`, waits for the task,
  and surfaces a failed exit status (via the existing `waitTask`).
- After the clone, the new VM needs cloud-init wiring the bare template lacks:
  add the cloud-init drive (`ide2: <storage>:cloudinit`), apply the cloud-init
  scalars, override cores/memory/cpu when set, resize `scsi0` when a larger size
  is given, and start when requested. This reuses the same option-building helpers
  as `provisionCloudImage`; factor the cloud-init option assembly into a shared
  helper so both paths stay in sync.

The clone target storage comes from `spec.disks[0].storage` when set, otherwise
the template's storage (omit the option and let Proxmox default).

## Reconciler

In `guestReconciler.Plan`, when a desired VM is absent and has `fromTemplate`
set, plan a create whose apply clones the template and finishes the VM, instead of
the build-from-image or blank path. The `GuestSpec` gains the resolved clone
source and clone mode. Everything after creation is unchanged: the VM is a normal
owned guest and the existing drift path applies on later passes.

## Out of scope (YAGNI)

- Re-cloning or reconciling which template an existing VM came from.
- Linked-clone storage-capability checks (Proxmox reports its own error if the
  storage cannot do it, surfaced via `waitTask`).
- LXC.

## Testing

- Manifest: `fromTemplate` scalar and object forms unmarshal; `fromTemplate`
  plus `image` is rejected; `fromTemplate` without a name is rejected.
- Reconciler: an absent VM with `fromTemplate` plans a clone with the resolved
  source vmid and the right mode; an undeclared template name yields a failing
  action; a present VM is not re-cloned.
- End-to-end on the live cluster: clone `debian-12-tpl` into a VM with cloud-init,
  confirm it boots and is reachable, then confirm a full clone survives deleting
  the template. Throwaway, not committed.

## Risks

- Cloud-init drive on a cloned template: the template is bare, so the clone has no
  `ide2` cloudinit drive; the create path must add it before setting ci params, or
  the settings are silently dropped.
- Ordering: a clone can only succeed once the template exists on the cluster;
  eventual retries cover the gap, and the template reconciler runs first.
