# ADR 0015: Desktop backend sync

- Status: accepted
- Date: 2026-06-19
- Builds on [ADR-0009](0009-backend-sync-design.md),
  [ADR-0011](0011-server-side-estimation.md), and
  [ADR-0014](0014-local-sleep-export-erasure.md).

## Context

The self-hosted backend can enroll devices, store encrypted append-only sync
records, and compute authenticated overview/rhythm projections from synced sleep
data. The desktop app now owns real local sleep observations, append-only
corrections, export, and hard local erasure. The missing loop-closing slice is
desktop consent and sync wiring.

The desktop client must not make sync feel ambient or vendor-hosted. It connects
only to a user-controlled backend, must disclose when a server estimate is being
shown, and must preserve the local-only behavior when sync is off.

## Decision

Add explicit desktop backend sync controls and a thin HTTP client:

- Sync is opt-in and off by default. With no enrolled backend, desktop Overview,
  Rhythm, Proposals, and local storage behave as before and make zero network
  calls.
- Settings owns enrollment. The user enters an HTTPS backend URL, enrollment
  secret, device label, and an explicit development-only self-signed TLS toggle.
  `POST /v1/devices` returns a device ID and bearer token.
- The backend URL, device ID, TLS dev flag, last sync time, and last sanitized
  error are stored in local config. The bearer token is stored separately in an
  owner-restricted local token file as the desktop fallback credential store; it
  is not written to editable config, UI status, sync payloads, or logs. Moving to
  OS keychain storage remains a hardening item.
- HTTPS certificate verification is required by default. The insecure skip-verify
  knob is off by default, visible in Settings, and accepted only for localhost
  self-hosted development endpoints.
- Desktop pushes only v1 contract-shaped sleep observation and correction records
  from its local sleep tables to `POST /v1/sync/push`. Records are idempotent by
  contract ID and payload hash. A server `409` conflict is shown as sync status
  instead of crashing or rewriting local history.
- Desktop pulls `GET /v1/sync/pull?since=<cursor>`, stores the cursor, ignores
  records from its own enrolled device ID, inserts remote sleep observations and
  corrections by contract ID, and marks pulled records as already seen so they are
  not pushed back on the next cycle.
- When sync is enabled and reachable, desktop reads `GET /v1/overview` and
  `GET /v1/rhythm`. The UI labels these as `Synced - server estimate`. If the
  backend is unavailable or refuses, desktop falls back to the local estimator and
  labels it as local.

## Erasure Boundary

ADR-0014 local erasure remains local erasure. It removes local sleep observations,
correction history, and local sync tracking metadata for those records so private
record IDs do not survive the local delete. It does not send remote delete
tombstones and does not claim to erase data already synced to a backend. Remote
delete propagation needs a separate tombstone design.

## Scope

In scope for this slice:

- desktop enrollment/status controls;
- desktop HTTP client over `net/http`;
- local sync cursor and pushed/seen record tracking;
- push/pull of sleep observation and correction records only;
- server overview/rhythm read-through with local fallback;
- contract fixture coverage for `sleep-data-export`.

Out of scope:

- approval decision sync and backend decision UI unification;
- delete propagation, tombstones, and remote erasure workflows;
- import/calendar write-back;
- OS keychain credential migration;
- Android sync UI and cloud skill packaging.

## Consequences

The desktop app now closes the local-to-self-hosted loop without changing the
default privacy posture. A user can stay fully local, or can enroll a self-hosted
backend and see clearly when server-side estimates are in use. The same v1
observation/correction contracts drive local export, desktop sync, backend sync,
and server projections.

The remaining risk is operational clarity around remote data lifecycle. Until
tombstones and backend erasure are designed, the UI and docs must avoid implying
that local hard-delete removes already-synced server data.
