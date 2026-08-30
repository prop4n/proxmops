# proxmops

Declarative GitOps for Proxmox VE. Point it at a Git repo, and it keeps your
cluster matching what the repo says. Think ArgoCD, for Proxmox.

> Status: early work in progress. ISO sync works end to end; VM and container
> apply is the current focus. Not affiliated with or endorsed by Proxmox.

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

## What works today

proxmops is usable now for ISO management and for observing your cluster; guest
provisioning is being built. Concretely:

- **Single binary, web UI embedded.** No runtime dependencies, no database
  server — one static binary that serves the dashboard and runs the loop.
- **Web UI with auth.** First-run setup, login, and sessions. The cluster
  connection and Git source are configured from the UI and stored encrypted at
  rest; the encryption key is generated automatically.
- **Git source.** Clones and pulls a repo (token auth) or reads a local path,
  and watches a configurable sub-directory.
- **Manifests.** Multi-document YAML for `VirtualMachine`, `Container`, `Iso`,
  and `Network`, each validated on load.
- **Reconcile loop with live status.** An ArgoCD-style overview — an interactive
  topology graph and a card view — updates over Server-Sent Events on every pass.
- **ISO sync (applied).** Images are downloaded onto the target storage when
  missing, with checksum verification. This path is complete.
- **VM / container (observed).** Guests are diffed for presence: a manifest with
  no matching guest is planned as a create, an owned guest dropped from the repo
  as a delete. **Applying those actions and comparing guest specs are not wired
  yet** — see the roadmap.
- **`plan` command.** A read-only diff between repo and cluster for CI or a
  pre-flight check.

## Core concepts

- **Desired vs observed state.** The repo holds desired state. The Proxmox API
  reports observed state. proxmops computes the difference and closes the gap.
- **Ownership by tag.** proxmops only manages guests it owns, marked with a
  `managed-by-proxmops` tag. Anything it did not create is left alone.
- **Soft prune.** Inside the owned scope, removing a manifest from the repo
  removes the resource. Outside that scope, nothing is ever deleted — pre-existing,
  hand-made guests are never touched. Prune is opt-in.
- **Drift correction.** When an owned resource diverges from the repo, proxmops
  reports it as out of sync and, in auto-sync mode, restores the desired state.

## Manifests

One file per resource, Kubernetes-style. Four kinds are defined; ISO is applied
today, VM and container are observed, network is schema-only for now.

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

A cloud-init VM built from a cloud image (`.qcow2`/`.raw`/`.vmdk`). proxmops
downloads the image, imports it as the disk, and injects the cloud-init settings:

```yaml
apiVersion: proxmops.dev/v1
kind: VirtualMachine
metadata:
  name: app-01
  node: pve-node1
spec:
  vmid: 120
  cores: 2
  memory: 2048
  cpu: x86-64-v2          # optional; defaults to the Proxmox default
  image:
    source: https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-genericcloud-amd64.qcow2
  disks:
    - storage: local-lvm
      size: 20G            # grows the imported disk
  net:
    - bridge: vmbr0
  cloudInit:
    user: debian
    sshKeys:
      - ssh-ed25519 AAAA...
    ip: dhcp               # or "ip=10.0.0.5/24,gw=10.0.0.1"
    nameserver: 1.1.1.1
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

A network bridge (schema only, not reconciled yet):

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

proxmops reads manifests from a directory tree under the configured path, scanned
recursively. Group them however you like:

```
your-repo/
  proxmox/                 <- the path proxmops watches
    vms/
      web-01.yaml
    lxc/
      dns-01.yaml
    isos/
      debian-12.yaml
  ...other unrelated content, ignored...
```

The watched path is configurable, so proxmops can live inside a monorepo next to
unrelated files. Everything outside that path is ignored.

## Install

proxmops ships as a single static binary with the web UI embedded. It talks to
Proxmox over the API, so it can run on a PVE node, in an LXC/VM, or anywhere with
network access to the cluster.

**One line:**

```sh
curl -fsSL https://raw.githubusercontent.com/prop4n/proxmops/main/packaging/install.sh | sh
```

The script downloads the latest release for your architecture (checksum
verified), installs the binary to `/usr/local/bin`, sets up a hardened `systemd`
service, and starts it. On first start the daemon creates its state directory at
`/var/lib/proxmops` and **generates the secret-encryption key automatically**
(`proxmops.db.key`, `0600`) — you never create or manage it by hand. Open
`http://<host>:8080` to finish setup from the UI.

Pin a version with `PROXMOPS_VERSION=v1.2.3`, and remove everything (keeping your
state) with `... | sh -s -- --uninstall`.

**Docker:**

```sh
docker run -d -p 8080:8080 \
  -v proxmops-data:/data \
  -e PROXMOPS_SERVER_DATABASEPATH=/data/proxmops.db \
  ghcr.io/prop4n/proxmops:latest
```

The key is generated next to the database inside the volume, so it survives
restarts.

## Configuration

The normal path is to configure nothing by hand: start the daemon, open the UI,
and fill in the cluster connection and Git source under **Settings**. Those are
stored encrypted in the database and take precedence over any file, so they apply
without a restart.

A config file (and matching `PROXMOPS_*` environment variables) is still
supported, useful for pre-seeding or air-gapped setups:

```yaml
cluster:
  endpoint: https://pve.example.com:8006/api2/json
  tokenId: proxmops@pve!gitops
  tokenSecret: ${PROXMOPS_CLUSTER_TOKENSECRET}   # from env, never inline
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
revocable identity rather than a root login. Secrets (the cluster token, the Git
token) are read from the environment or a file, never stored inline.

## Web UI

The daemon serves a dashboard to watch what proxmops sees, in the spirit of the
ArgoCD UI.

- **Overview.** Every managed resource with its sync status, in two views: an
  interactive topology graph (source → kind → resource, pan/zoom, drift filter)
  and a grouped card view. Status streams live over SSE.
- **Settings.** Configure and test the cluster connection and the Git source
  from the browser; secrets are write-only and never sent back.

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
        |  (interval,       |       |  (status over SSE)|
        |   status)         |       +-------------------+
        +-------------------+
```

The engine (loader, differ, executor) has no knowledge of the loop, so the same
core powers both the daemon and the read-only `plan` command.

## Safety

Managing infrastructure declaratively is powerful and therefore dangerous. The
defaults lean cautious.

- **Ownership isolation.** Only tagged, proxmops-owned guests are ever modified
  or deleted.
- **Opt-in prune.** Deletion of resources dropped from the repo happens only when
  prune is enabled.
- **Detect-only default.** With auto-sync off, proxmops reports drift and plans
  without touching the cluster.
- **Encrypted secrets.** Cluster and Git tokens are encrypted at rest under a
  locally held key.

## Roadmap

Done:

- [x] Git source: clone/pull, token auth, configurable path
- [x] Manifest schema and loader (VM, container, ISO, network)
- [x] Reconcile loop, sync status, ArgoCD-style web UI over SSE
- [x] Auth, encrypted settings from the UI
- [x] ISO / template sync with checksum verification

Next:

- [ ] VM apply: create / update / delete (current focus)
- [ ] Container apply
- [ ] Spec-level drift detection for guests
- [ ] Network and storage reconciliation
- [ ] Explicit adoption of existing guests
- [ ] UI: dry-run preview, PR-impact preview, filters, drift detail
- [ ] Later: Git webhook triggers, health checks, multi-cluster

## Development

Requires Go and [Bun](https://bun.sh) (for the web UI).

```sh
# Backend
go test ./...
go build ./cmd/proxmops

# Web UI (from web/ui)
bun install
bun run build      # emits dist/, embedded by the Go build
bun run test       # vitest
```

Releases are cut with GoReleaser (`.goreleaser.yaml`): static linux/darwin
binaries with the UI embedded, plus the `checksums.txt` the installer verifies.

## Status

Early and incomplete. Interfaces and manifest schema will change. Not affiliated
with or endorsed by Proxmox Server Solutions GmbH.
