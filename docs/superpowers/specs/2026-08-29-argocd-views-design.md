# Overview: two ArgoCD-style views

**Date:** 2026-08-29
**Status:** approved design, pre-implementation

## Problem

The overview (`web/ui/src/pages/DashboardView.vue`) currently offers three view
modes — `flow` (source block fanning out to flat cards over an SVG overlay),
`cards` (one dense grid), and `modules` (one section per kind). We want only two,
both modelled on ArgoCD:

1. A **card view** — resource tiles.
2. A **schema view** — an interactive topology graph.

## Decisions

- **Drop the flat `cards` grid.** Keep `flow` (schema) and `modules` (grouped
  cards). The `modules` view becomes the card view; the flat grid was redundant
  with it.
- **Schema view = full interactive graph** (option C), not a static SVG.

## Scope

### 1. Schema view — interactive graph (Vue Flow)

Replace the current bespoke SVG fan-out with a real node graph.

- **Library:** `@vue-flow/core` for the canvas, `@vue-flow/controls`,
  `@vue-flow/background`, `@vue-flow/minimap`, and `@dagrejs/dagre` for layout.
- **Layout:** dagre computes a left→right hierarchical tree. Positions are
  recomputed whenever the resource set changes; between recomputes the user may
  drag nodes freely (Vue Flow default).
- **Hierarchy — 3 levels:**
  - Level 1: a single **source** node (the git repo, from saved settings).
  - Level 2: one **kind** node per kind present (VirtualMachine, Container, Iso),
    labelled with `n/m synced`.
  - Level 3: one **resource** node per resource, child of its kind node.
- **Custom nodes:** Vue components rendered by Vue Flow. Resource nodes reuse the
  tile styling (kind badge, name, status LED, amber border on drift). Source and
  kind nodes are distinct compact nodes.
- **Edges:** bezier, tinted amber on branches that lead to a drifted resource.
- **Controls:** zoom +/− and fit-to-view (`@vue-flow/controls`).
- **Minimap:** navigation minimap in a corner (`@vue-flow/minimap`).
- **Filters:** toggle to hide `Synced` resources (show drift only). When a kind
  has no visible resources under the active filter, its kind node is hidden too.
- **Background:** dotted/grid background.

### 2. Card view — ArgoCD tiles

Keep the grouped-by-kind sections. Restyle `ResourceCard.vue` into an ArgoCD-like
tile:

- Left status stripe (colour = sync state).
- Kind badge, resource name, status LED.
- Metadata row: `#vmid · node` (when present).
- Sync age line.
- Action/reason line when drifted.

The level-3 resource node in the graph is a thin Vue Flow node wrapper that
renders `ResourceCard` inside, so the tile styling lives in one place and Vue
Flow's node props (handles, selection) don't leak into the card.

### 3. Toolbar & state cleanup

- Toolbar drops from three buttons to two: **schema** (`flow`) and **cards**.
- `ViewMode` becomes `'flow' | 'cards'`. The `modules` template block is renamed
  to `cards`; the old flat `cards` block is deleted.
- `localStorage` key `proxmops-view`: only `'flow'` and `'cards'` are valid; any
  other stored value (including the legacy `'modules'`) falls back to a valid
  default (`'flow'`).

## Non-goals

- No server/API changes — everything is driven by the existing
  `snapshot.resources` and settings.
- No graph persistence (dragged positions are not saved across reloads).
- No health model beyond the existing `Synced` / `OutOfSync` states.

## Files touched

- `web/ui/package.json` — add Vue Flow packages + dagre.
- `web/ui/src/pages/DashboardView.vue` — replace flow SVG with Vue Flow graph,
  drop flat cards block, rename modules→cards, trim toolbar & view state.
- `web/ui/src/components/ResourceCard.vue` — ArgoCD tile restyle.
- New graph node components under `web/ui/src/components/graph/` (source node,
  kind node, resource node) and a dagre layout helper.
- Possibly `web/ui/src/style.css` — import Vue Flow CSS if not inlined per
  component.

## Risks / unknowns

- Vue Flow + Vue 3.5 + Vite 8 + Tailwind 4 compatibility — verify the CSS import
  and that the canvas sizes correctly inside the existing layout.
- Dagre layout tuning (node spacing, rank direction) will need a couple of
  iterations against the live test cluster data.
