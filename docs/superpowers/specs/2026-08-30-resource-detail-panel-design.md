# Resource detail panel (right drawer)

**Date:** 2026-08-30
**Status:** approved, implementing

Replace the per-card action buttons with a right-hand drawer opened by clicking a
resource (card or graph node). The drawer shows desired vs observed config,
history, and actions.

## Backend

New endpoint `GET /api/v1/resources/{kind}/{name}` returning a detail:

```json
{
  "status":      { ...ResourceStatus... },
  "desiredYAML": "apiVersion: ...\n",
  "observed":    { "present": true, "cores": 2, "memoryMB": 2048, "cpu": "x86-64-v2",
                   "running": true, "ip": "ip=dhcp", "nameserver": "1.1.1.1" }
}
```

- Implemented by the app via a `ResourceDetailer` interface (like `ResourceDeleter`):
  build the effective config, load desired state, find the resource by kind+name.
  - `desiredYAML`: `yaml.Marshal` of the matched manifest resource (uniform across
    kinds, no per-kind mapping).
  - `observed`: best-effort from the cluster.
    - VirtualMachine / Template: match by vmid in `ListGuests`; report
      present/cores/memoryMB/cpu/running/ip/nameserver (fields already gathered).
    - Iso: presence via `ListISOs`.
  - Not found in desired → 404.
- History reuses the existing `GET /resources/{kind}/{name}/events`.

## Frontend

- `ResourceDetail.vue`: a fixed right drawer (overlay on small screens, side panel
  on wide) with sections:
  - Header: icon, name, kind, status LED, close button.
  - Actions: Delete (two-step confirm), enabled for Iso and VirtualMachine.
  - Desired: the YAML in a scrollable mono block.
  - Observed: key/value list; "not present on the cluster" when absent.
  - History: the events timeline (moved out of the card).
- Selection is held in a tiny shared store (`stores/selection.ts`): `select(kind,
  name)` / `clear()`. The drawer reads it and fetches detail + events on open.
- `ResourceCard` becomes a button: clicking it selects the resource. The trash and
  history buttons are removed from the card (they live in the drawer now). The
  card keeps the drift stripe/LED and a subtle selected ring.
- Graph resource nodes select on click too (the card inside already handles it;
  ensure the click isn't swallowed by pan/drag — keep `nodrag`).

## Non-goals

- No live-streaming observed config; it is fetched once when the drawer opens and
  on manual refresh.
- Daemon-log lines in the drawer (that is the separate logs increment).
- Editing config from the panel.

## Risks

- Marshaling manifests to YAML: use the same yaml library as the loader so field
  names match the manifest schema.
- Observed fetch hits the cluster per drawer-open; acceptable (one call), and it
  degrades to "unavailable" on error rather than blocking the panel.
