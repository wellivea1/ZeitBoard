# ADR 0014: Local sleep export and erasure controls

- Status: accepted
- Date: 2026-06-19
- Builds on [ADR-0013](0013-desktop-local-data.md) and the v1
  `observation-set`, `correction-set`, and `sleep-data-export` contracts.

## Context

ADR-0013 made the desktop app usable with local manual sleep observations and
append-only corrections, but Settings still exposed only placeholder copy for
export and deletion. Phase 2 needs a real local data-control slice before import
hardening or backend sync can be trusted.

The key product distinction is that "suppress from estimates" and "delete data"
are different operations:

- suppress appends a correction with `changes.excluded=true`; the raw observation
  and correction history remain available for audit/export;
- delete is a destructive privacy control that removes the local record and its
  correction history.

## Decision

Add a desktop-local sleep export and erasure surface:

- `ExportSleepData` returns a v1 `sleep-data-export` object containing nested
  `observation_set` and `correction_set` objects. The desktop UI downloads the
  JSON and also shows the generated payload for inspection.
- `DeleteSleepObservation` requires the exact confirmation token `DELETE`, then
  removes the target `local_sleep_observations` row and all
  `local_sleep_corrections` rows that target it.
- `DeleteAllSleepData` requires the same confirmation token, then removes every
  row from the local sleep observation and correction tables.
- Both erasure paths checkpoint/truncate the SQLite WAL and run `VACUUM`, matching
  the existing full-store delete cleanup. The local SQLite store also enables
  `PRAGMA secure_delete = ON`.

The Data Sources screen owns per-entry erasure. Settings owns export and
erase-all-local-sleep controls. Both screens explain that suppress is append-only
and erase is permanent.

## Scope Boundary

This decision covers only desktop-local sleep data stored in the local SQLite
tables introduced by ADR-0013. It does not define backend sync deletion,
cross-device tombstones, clinician PDF export, calendar deletion, or full-account
deletion. Those need separate sync/import decisions because remote erasure and
sync conflict semantics are materially different.

## Consequences

Users can back up or inspect their local sleep records without a network service,
and can permanently erase local sleep records without pretending that erasure is a
correction. Append-only correction history remains intact until the user invokes
an explicit erase control.

This is application-level erasure from ZeitBoard's active SQLite store and WAL.
It does not claim to remove copies in external backups, filesystem snapshots, or
storage-device wear leveling.

Backend sync of these contract-shaped records must not silently reinterpret local
hard delete as an append-only correction. A future sync slice must define explicit
delete/tombstone behavior before syncing erasure across devices.
