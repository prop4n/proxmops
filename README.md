# proxmops

Declarative GitOps for Proxmox VE. Point it at a Git repo, and it keeps your
cluster matching what the repo says. Think ArgoCD, for Proxmox.

> Status: early work in progress. Not affiliated with or endorsed by Proxmox.

## The problem

Proxmox clusters drift. VMs get resized by hand, ISOs are uploaded to one node
and forgotten on another, tags rot, and six months later nobody knows why a
guest is configured the way it is. There is no single source of truth, no review
before a change lands, and no clean way to roll back.

## The idea

Describe the desired state of your cluster as YAML in a Git repo. proxmops reads
that repo and reconciles the live cluster to match, continuously. Git becomes the
source of truth. Changes go through pull requests. Rollback is a revert.

```
Git repo (desired state)  ->  proxmops  ->  Proxmox cluster (observed state)
        ^                                              |
        |________________ reconcile loop ______________|
```

## Core concepts

- **Desired vs observed state.** The repo holds desired state. The Proxmox API
  reports observed state. proxmops computes the difference and closes the gap.
- **Reconciliation loop.** A daemon watches the repo and the cluster and
  reconciles on an interval (and, later, on Git webhook).
- **Ownership by tag.** proxmops only manages resources it owns, marked with a
  `managed-by:proxmops` tag. Anything it did not create is left alone.
- **Soft prune.** Inside the owned scope, removing a manifest from the repo
  removes the resource from the cluster. Outside that scope, nothing is ever
  deleted. Pre-existing, hand-made VMs are never touched.
- **Adoption.** An existing resource can be brought under management explicitly,
  by matching its id and letting proxmops tag it. Nothing is adopted silently.
- **Drift correction.** If someone changes an owned VM by hand, proxmops reports
  it as out of sync and, in auto-sync mode, restores the desired state.

## Manifests

One file per resource, Kubernetes-style. Four kinds are planned.

A QEMU virtual machine:

```yaml
apiVersion: proxmops.dev/v1
kind: VirtualMachine
metadata:
  name: web-01
  node: pve-node1
  tags: [prod, web]
spec:
  vmid: 101
  cores: 4
  memory: 8192
  disks:
    - storage: local-lvm
      size: 40G
  net:
    - bridge: vmbr0
      model: virtio
  iso: debian-12.iso
  state: running
```

An LXC container:

```yaml
apiVersion: proxmops.dev/v1
kind: Container
metadata:
  name: dns-01
  node: pve-node1
  tags: [infra, dns]
spec:
  vmid: 201
  cores: 1
  memory: 512
  rootfs:
    storage: local-lvm
    size: 8G
  net:
    - bridge: vmbr0
      ip: 10.0.0.53/24
      gw: 10.0.0.1
  template: debian-12-standard
  state: running
```

An ISO or template synced onto a storage:

```yaml
apiVersion: proxmops.dev/v1
kind: Iso
metadata:
  name: debian-12
spec:
  source: https://cdimage.debian.org/.../debian-12.iso
  node: pve-node1
  storage: local
  checksum:
    algo: sha256
    value: 0123abcd...
```

A network bridge:

```yaml
apiVersion: proxmops.dev/v1
kind: Network
metadata:
  name: vmbr0
  node: pve-node1
spec:
  type: bridge
  ports: [eno1]
  vlanAware: true
  cidr: 10.0.0.1/24
```

## Target repo layout

proxmops reads manifests from a directory tree. The recommended layout groups
resources by kind, but any nesting under the configured path is scanned
recursively.

```
your-repo/
  proxmox/                 <- the path proxmops watches
    vms/
      web-01.yaml
      web-02.yaml
    lxc/
      dns-01.yaml
    isos/
      debian-12.yaml
    networks/
      vmbr0.yaml
  ...other unrelated content, ignored...
```

The watched path is configurable, so proxmops can live inside a monorepo next to
unrelated files. Everything outside that path is ignored.

## Configuration

proxmops is configured with a small file describing where the cluster is and
where the desired state lives.

```yaml
cluster:
  endpoint: https://pve.example.com:8006/api2/json
  tokenId: proxmops@pve!gitops
  tokenSecret: ${PROXMOPS_TOKEN}     # read from env
  insecureSkipVerify: false

source:
  repoURL: https://github.com/you/homelab.git
  path: proxmox               # sub-directory watched inside the repo
  revision: main

reconcile:
  interval: 60s
  autoSync: true              # apply automatically; false = detect only
  prune: true                 # allow deleting owned resources dropped from repo
  dryRun: false
```

Authentication uses a Proxmox API token, so proxmops runs with a scoped,
revocable identity rather than a root login.

## Architecture

proxmops is a reconciliation engine wrapped in a control loop.

```
        +-------------------+
        |  Git repo (path)  |
        +-------------------+
                  |
                  v
        +-------------------+     loads and validates manifests
        |  Loader / Parser  |
        +-------------------+
                  |
                  v   desired state
        +-------------------+     +-----------------------+
        |     Differ /      |<----|  Proxmox API client   |  observed state
        |     Planner       |     +-----------------------+
        +-------------------+
                  |   plan (create / update / delete)
                  v
        +-------------------+
        |     Executor      | ----> Proxmox cluster
        +-------------------+
                  |
                  v
        +-------------------+       +-------------------+
        |  Control loop     |------>|  Web UI / API     |
        |  (interval,       |       |  (status, dry-run,|
        |   status, retries)|       |   PR impact)      |
        +-------------------+       +-------------------+
```

- **Loader / Parser** reads the watched path, decodes manifests, validates them.
- **Proxmox API client** reads live state and applies changes.
- **Differ / Planner** compares desired and observed state within the owned
  scope and produces an ordered plan.
- **Executor** applies the plan, respecting ownership and prune settings.
- **Control loop** drives all of the above on an interval and tracks sync status.

The engine (loader, differ, executor) has no knowledge of the loop, so the same
core powers both the daemon and the read-only `plan` command.

## Usage modes

- **Daemon.** Runs continuously, reconciling on the configured interval. This is
  the primary mode.
- **plan.** A read-only command that prints the diff between repo and cluster
  without changing anything. Useful in CI and before enabling auto-sync.
- **Web UI.** A dashboard served by the daemon for supervising the whole thing.

## Web UI

The daemon serves a web dashboard to watch and understand what proxmops is doing,
in the spirit of the ArgoCD UI.

- **Overview.** Every managed resource with its sync status: in sync, out of
  sync, or unmanaged. Filter by node, kind, or tag.
- **Dry-run preview.** For any resource or the whole cluster, see the plan
  proxmops would apply (what gets created, changed, or deleted) before anything
  runs.
- **PR impact.** Point the UI at a proposed revision or branch to see what
  merging that pull request would do to the cluster, so review happens against
  real consequences, not just a YAML diff.
- **Drift.** When an owned resource is changed by hand, the UI shows the drift
  and what reconciliation will do about it.

The UI is read-only for viewing state and plans. Applying still follows the
configured sync policy.

## Safety

Managing infrastructure declaratively is powerful and therefore dangerous. The
defaults lean cautious.

- **Ownership isolation.** Only tagged, proxmops-owned resources are ever
  modified or deleted.
- **Opt-in prune.** Deletion of resources dropped from the repo is only done when
  prune is enabled.
- **Dry run.** Every apply can be previewed without side effects.
- **Explicit adoption.** Existing resources are never taken over silently.

## Roadmap

- [ ] Phase 1: QEMU VM reconciliation (create, update, drift, prune)
- [ ] Phase 2: LXC containers
- [ ] Phase 3: ISOs and templates with checksum verification
- [ ] Phase 4: networks and storage
- [ ] Web UI: sync overview, dry-run preview, PR impact preview, drift view
- Later: Git webhook triggers, health checks, multi-cluster

## Status

Early and incomplete. Interfaces and manifest schema will change. Not affiliated
with or endorsed by Proxmox Server Solutions GmbH.
