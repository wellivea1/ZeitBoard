# ADR 0017: Server-side erasure with tombstones

- Status: accepted
- Date: 2026-07-02
- Builds on [ADR-0009](0009-backend-sync-design.md) (append-only sync log),
  [ADR-0014](0014-local-sleep-export-erasure.md) (local hard erasure), and
  [ADR-0015](0015-desktop-backend-sync.md) / [ADR-0016](0016-approvals-unification.md)
  (desktop sync + pull-orphan handling).

## Context

ADR-0014 gave the user real local erasure, but the backend sync log is
append-only by design: a hard-deleted record's encrypted copy survived on the
self-hosted instance, and other devices that had pulled it kept theirs. The
erasure right must extend to the whole synced system without breaking the
append-only cursor model that sync correctness depends on.

## Decision

**Tombstones.** Erasing a synced record does three things on the server, in one
transaction (`POST /v1/sync/erase`, authenticated, any enrolled device):

1. **Hard-deletes** the record's row from `sync_records` — the encrypted
   payload is really gone (tombstone envelopes themselves are never deleted, so
   repeat erasure cannot remove the erase signal).
2. **Registers** the record id in `sync_tombstones`. `Append` checks this
   registry first: a stale device re-pushing an erased id is a **silent
   no-op** — an erased record can never be resurrected, and the stale device
   is not wedged by an error.
3. **Mints a tombstone envelope** in the pull stream (same seq/cursor space,
   `kind: "tombstone"`), whose payload carries **only the record id** — no
   health data. Every device that pulls it hard-deletes its local copy.

**Routing-metadata amendment (2026-07-27).** V1 record identifiers overlap
syntactically, so id-only tombstones cannot reliably distinguish a task
revision from a sleep record. New tombstones therefore also carry the original
`observation`, `correction`, or `task` kind when the erased row existed. This is
non-sensitive routing metadata, not record content. Legacy id-only tombstones
use local sync evidence; ambiguous legacy ids abort the atomic page transaction.

**Logical-task erasure amendment (2026-07-27).** Immutable task revisions are
transport records for one logical task, not independent user data. Erasing any
known task revision therefore hard-deletes and tombstones every retained
revision of that task in the same transaction. The server also registers the
logical task id: all later revisions are silent no-ops even when their record
ids did not exist when deletion was requested. Sleep observations and
corrections remain record-scoped.


**Desktop outbox.** Local hard-deletes (ADR-0014) enqueue the deleted record
ids in `local_sleep_erasures` — but **only ids that were actually pushed**;
never-synced records never left the device and need no tombstone. `SyncNow`
pushes records, then erasures, then pulls. Applying a *pulled* tombstone
hard-deletes locally without re-enqueueing (no propagation loops), and is
idempotent.

## Consequences

- The user's erasure right now covers the full system: local store, the
  self-hosted instance, and every other enrolled device (on its next sync).
- The append-only model survives: erasure appends a tombstone to the stream;
  the cursor semantics and idempotent push are unchanged; the server-side read
  model ignores tombstone envelopes (the erased record's data row is gone).
- Tombstone payloads are metadata-only (record id plus an optional three-value
  original kind). An observer learns that *something* was erased and its broad
  storage class, never what it said. Mixed pull pages and their cursors commit
  atomically, followed by one secure compaction for all page tombstones.
- **Residual:** a device that never syncs again keeps its local copy — erasure
  reaches devices only when they pull. The tombstone registry grows
  monotonically (ids only; negligible size). Both recorded in the threat model.
  The logical-task tombstone registry is also monotonic and stores only task
  ids.
- Server-side proposals/audit entries are out of scope here: they contain no
  raw sleep records (redacted planning windows only) and follow their own
  retention (ADR-0010).
