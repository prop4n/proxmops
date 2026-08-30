# Observability: commit, per-resource history, daemon logs

**Date:** 2026-08-30
**Status:** approved, implementing increment 1

Three related increments, delivered in order (each its own PR):

1. **Git commit** surfaced in status ("reconciled on abc123").
2. **Per-resource history** persisted in SQLite (events on change + applied action).
3. **Daemon logs** in the UI via an in-memory ring buffer streamed over SSE.

## Increment 1: Git commit

The Git source resolves a commit hash every pass; expose it so the UI can show
which commit the cluster is reconciled against.

- `source.Git` stores the resolved short+full hash in `sync()`; a `Commit()
  string` method returns it. `source.Dir` (local path) has no commit and is not
  a committer.
- `reconcile.Engine`, after computing the plan, reads the commit via an optional
  `interface{ Commit() string }` on the source and puts it on the status
  snapshot (`Snapshot.Commit`).
- `status.Snapshot` gains `Commit string`; the API already serialises the
  snapshot. The UI source node shows the short commit when present.

Scope: display only. No persistence yet.

## Increment 2: per-resource history

A persistent event log per resource.

- **Store:** SQLite table `resource_events(id, kind, name, type, reason, commit,
  at)`, in `internal/store`. `AppendEvent` and `EventsFor(kind, name)`.
- **Event types:** `created`, `deleted`, `drifted`, `synced`, `applied`,
  `failed`.
- **Recording:**
  - The engine records transition events (created/drifted/synced/deleted) by
    comparing the previous snapshot state with the new one — it already computes
    transitions in `recordStatus`, and it has the pass commit.
  - The dispatcher records `applied`/`failed` when an action finishes. The
    `Action` carries the pass commit (set at plan time) so the event is attributed
    to the right commit. A new `EventSink` is passed to the dispatcher.
- **API:** `GET /api/v1/resources/{kind}/{name}/events` returns the events,
  newest first.
- **UI:** clicking a resource opens a history timeline (event, reason, commit,
  relative time).
- **Retention:** keep everything for now (documented debt; a later increment adds
  purge/cap).

## Increment 3: daemon logs

- A `slog.Handler` tees records into an in-memory ring buffer (~1000 lines),
  wrapping the existing handler.
- **API:** `GET /api/v1/logs` returns the buffer; SSE streams new lines.
- **UI:** a Logs page showing the live tail.
- Lost on restart by design; the persistent history covers durable audit.

## Non-goals

- Event retention/purge (increment 2 keeps all).
- Log persistence (ring buffer only).
- Diffing manifest content between commits.

## Risks

- Source interface change kept minimal via an optional `Commit()` assertion, so
  `Dir` and tests are unaffected.
- Threading the commit onto async actions: the `Action` carries it, set when the
  plan is built, so applied/failed events attribute correctly even across passes.
- Unbounded event growth with keep-all — acceptable short term, flagged for a
  follow-up cap.
