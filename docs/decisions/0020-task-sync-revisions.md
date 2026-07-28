# ADR 0020: Task sync as immutable revision records

- Status: accepted
- Date: 2026-07-09
- Closes the sync deferral of [ADR-0018](0018-user-owned-tasks.md) using the
  log of [ADR-0009](0009-sync-backend-m1.md)/[ADR-0015](0015-desktop-backend-sync.md)
  and the erasure machinery of [ADR-0017](0017-server-erasure-tombstones.md).

## Context

Tasks are mutable planning items (ADR-0018: plain CRUD, no correction chain),
but the sync transport is an append-only log of immutable records whose
`Append` is idempotent **by record id** — a mutated payload under the same id
would be silently ignored. Sleep records never mutate, so this never mattered
before.

## Decision

1. **Each task edit syncs as a new immutable revision record.** Every local
   mutation bumps the task's `revision`; the current state travels as a
   `kind: "task"` record with id `"<task_id>_r<revision>"` carrying the full
   contract-shaped task (task-set `$defs/task` + required `revision`,
   `updated_at`). The server validates strictly (id convention enforced) and
   stores revisions like any other record; it never interprets them.
2. **Consumers apply last-writer-wins by revision.** On pull, an unknown task
   is created, a higher revision replaces local state, a stale revision is
   skipped and counted. Applied revisions are marked as already synced so
   pulled tasks are never pushed back (no echo loops).
3. **Deleting a task erases all its pushed revisions** through the ADR-0017
   erase endpoint: payload hard-deletes, tombstone minting, resurrection
   blocking — task titles are private user text, so task deletion is
   privacy-grade erasure, not a soft flag. The erasure outbox is shared with
   sleep records (opaque record ids; the backing table keeps its historical
   name). On pull, a tombstone for **any** revision of a task deletes the
   local task, idempotently, without re-enqueueing.
   Server erasure treats those records as one logical task: it expands any
   known revision to all retained revisions in the erase transaction and
   registers the task id so a later, previously unseen revision is ignored.
   This closes resurrection by a stale or concurrently offline device.

4. **Server-side estimation ignores task records** (the readmodel replays
   only sleep kinds), and assistant/agent redaction is unchanged: titles now
   reside encrypted at rest on the user's own instance, but still never enter
   LLM context or trusted views.

## Consequences

- Every enrolled device now shares one task list: add/edit/done/delete
  converge via revisions; the approval queue and scheduler operate on the
  same inventory everywhere.
- Concurrent edits on two offline devices can mint the **same revision
  number**; the log's per-id idempotence keeps one, and the next edit on the
  losing device supersedes it (revision monotonicity per device is
  best-effort, convergence is guaranteed, one edit may be silently
  superseded). Documented as acceptable for a single-user planner; a vector
  clock is not warranted.
- Residuals mirror ADR-0017: a device that never syncs again keeps its task
  copies until it pulls; a task re-created after deletion gets a fresh id and
  revision history. Explicit tombstone kinds make valid third-party task ids
  independent of the desktop's `task_` naming convention. Legacy id-only
  tombstones use local sync evidence and reject ambiguity.
- Pre-revision local rows are treated as revision 1 on first sync.
