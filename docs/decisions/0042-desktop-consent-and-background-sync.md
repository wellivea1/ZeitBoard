# ADR 0042: Durable desktop consent and background sync

- Status: accepted
- Date: 2026-09-08
- Implements part of completion-plan C1; follows ADR-0039's pre-release direction.

## Problem

The desktop started its activity collector without saved consent and retained
observations only in `MemorySink`. Closing a window kept a process alive, but
that did not make collection durable or make synchronization automatic. These
were implementation gaps behind the existing collector and privacy statements.

## Decision

- Activity collection is default-off. Settings presents its scope, a collection
  time zone, saved consent, live running/stopped/error state and retained count.
  Consent lives in SQLite. A missing or unreadable grant never starts collection;
  no older settings backup can resurrect a revoked grant. Unsupported platforms
  cannot enable the collector.
- The existing source-observation table is the durable sink. A transition batch
  commits atomically. Per-run random identifiers plus sequence numbers prevent
  identity collisions across restarts and clock changes. Shutdown attempts one
  final bounded flush. A failed collector reports stopped and can restart.
- Steady unlocked use no longer emits unlocked/active on every Windows poll.
  Poll-gap suspend/resume records carry inferred provenance, algorithm version
  and unknown confidence. Clock adjustment or a paused process can also explain
  a gap; it is not an observed OS power event.
- Explicit activity export streams a consistent source-scoped snapshot to a
  staged, owner-only file before atomic replacement. The renderer receives only
  bounded metadata. Erasure disables collection, joins the collector, deletes
  its saved records and compacts SQLite; it does not erase sleep or other sources.
- Once enrolled, the same desktop sync pipeline runs at startup, on enrollment,
  and approximately one minute after each attempt while the app is running.
  It is independent of mounted views. The full exchange has a two-minute deadline
  and existing per-request/page bounds. Manual and background sync serialize;
  timer ticks do not pile up behind a foreground operation. Disabling cancels
  in-flight sync, serializes credential changes and prevents stale projection
  failures from restoring a previous connection. Quit joins work before closing
  clients or the database.
- Backend configuration and credential files now publish through the existing
  atomic private-file writer. HTTP redirects are refused, including enrollment
  redirects that could otherwise forward the enrollment secret.
- Desktop schema creation now installs the current tables directly. Historical
  migration bookkeeping, column backfills and the legacy duplicate-tolerating
  import trigger are removed. A partial unique index enforces current imported
  source identity. Current reopen/revision/deduplication/erasure tests remain.
  Unsupported development columns fail without rewriting their records.
- `ZEITBOARD_DATA_DIR` optionally selects an absolute desktop profile directory
  for isolated qualification. Omit it for the normal per-user profile. Tests and
  binding generation use disposable directories and synthetic records.

## Boundaries and remaining work

Activity records stay local. They are not sleep observations, do not change the
estimator or planning, and are not sent to the backend, a portal or an assistant.
The measured inference gate remains. Sync still covers the existing sleep/task
contracts; this ADR does not add medication or activity synchronization.

Automatic login/start-hidden controls, background estimate projection refresh,
desktop enrollment/restore reconciliation, and native hide/resume/quit recovery
qualification remain C1 work. The service tests and browser settings inspection
do not qualify real suspend, OEM behavior, supported installations, private pilot
results or packaged releases. The complete C1–C8 goal remains active.
