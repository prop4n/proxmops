# UI-driven resource deletion (ArgoCD-style)

**Date:** 2026-08-30
**Status:** approved, implementing (increment 1: ISO)

## Goal

A manual "Delete" action on a resource in the web UI that removes it from the
Proxmox cluster immediately — like ArgoCD's delete. Increment 1 covers ISO
images; VM/Container get a disabled button until guest delete lands.

## Decisions

- **Imperative, not declarative.** A UI button deletes now; it does not touch the
  repo.
- **One-shot (comes back).** If the resource is still desired in the repo and
  auto-sync is on, the next reconcile pass re-creates/re-downloads it. Good for
  forcing a re-download. No exclusion state is stored.
- **Explicit per-resource = safe.** The user targets a named managed resource, so
  ISO deletion needs no ownership tag (which ISOs lack).
- **ISO first.** VM/Container deletion needs `DeleteGuest` (not implemented); the
  button is disabled for them.

## Components

1. **`manifest.Iso.Filename()`** — derives the storage filename from `Spec.Source`
   (URL basename). Replaces the private `isoFilename` in the ISO reconciler so the
   deleter and reconciler agree.
2. **`proxmox.IsoStore.DeleteISO(ctx, node, storage, filename)`** — PVE impl
   deletes the volume via `DELETE /nodes/{node}/storage/{storage}/content/{volid}`.
3. **`app.DeleteResource(ctx, kind, name)`** — builds the effective config, loads
   desired state, finds the resource by kind+name, and for `Iso` calls
   `DeleteISO`. Returns `server.ErrResourceNotFound` / `server.ErrDeleteUnsupported`.
4. **Server** — `DELETE /api/v1/resources/{kind}/{name}` (authenticated). Calls a
   `ResourceDeleter` interface (implemented by app) injected via Options. Maps
   errors to 404 / 400 / 500, success to 204.
5. **UI** — `api.deleteResource(kind, name)`; a trash action on `ResourceCard`
   with a two-step inline confirm (no dialog dependency), disabled for
   non-Iso kinds. Status updates arrive over the existing SSE stream.

## Non-goals

- No guest (VM/Container) deletion yet.
- No immediate forced status refresh; the next reconcile pass reflects the change
  over SSE.
- No repo mutation.

## Safety

Authenticated endpoint, explicit per-resource action, inline confirmation.
