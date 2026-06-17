# ADR 0011: Server-side estimation and read projections

- Status: accepted
- Date: 2026-06-17
- Builds on [ADR-0006](0006-agent-accessible-interface.md),
  [ADR-0009](0009-backend-sync-design.md), and
  [ADR-0010](0010-assistant-backend-byok.md).

## Context

The self-hosted server now receives encrypted, authenticated sync records and can create
pending assistant proposals. The next backend capability is read-side: compute the user's
current estimate from the sleep observations already synced to the server, then expose
civil-time, uncertainty-visible projections that future clients and the M4 MCP connector
can read without needing raw private records.

## Decision

Add a server read model that replays decrypted sync records from the append-only log,
maps v1 sleep observations and corrections into core domain types, applies the existing
core correction and ingest logic, and runs the existing estimator:

- `internal/readmodel` decrypts sync records through the store API, maps
  `observation` payloads into `domain.SleepSession`, maps `correction` payloads into
  `domain.ManualCorrection`, marks superseded corrections inactive, applies
  `domain.ApplySleepCorrections`, and then calls
  `ingest.ResolveOverlappingSleepReports`.
- `internal/projection` runs `estimation.RobustEstimator{}.Estimate`,
  `.Project`, and optionally `.Backtest`; it does not implement a second estimator.
- `internal/api` exposes authenticated read endpoints:
  - `GET /v1/overview`
  - `GET /v1/rhythm`
  - `GET /v1/accuracy`

The overview DTO mirrors the desktop `OverviewDTO` field names so desktop-over-sync can
consume the same concepts. Rhythm wraps the existing `estimation.RhythmProjection`
shape under a status/refusal envelope, with internal row IDs rewritten to presentation
IDs before serialization. Both overview and rhythm return HTTP 200 for typed estimator
refusals instead of treating insufficient data as a server error.

## Refusal Behavior

Estimator refusals pass through as `{status:"refused", refusal:{code,message}}` with
plain copy. The server never fabricates phase, drift, or forecast values when the synced
history is insufficient, ambiguous, or outside the estimator's validation range.

## Projection Boundary

Read endpoints return projection DTOs only. They do not return raw sync envelopes,
observation payloads, notes, medication names, calendar text, device tokens, or provider
keys. They also avoid exposing synced observation IDs or source record IDs. Times are
civil-time-primary strings plus confidence/uncertainty labels where the projection shape
supports them. Raw timestamps remain inside the server and core engine.

## M4 Hook

These read endpoints are the read side of the M4 agent-accessible interface. MCP tools
should expose these same projections, then pair them with the M2 propose-only action
registry. Agents can read and propose; approval remains a human action through the
existing one-use token path.

## Out Of Scope

M3 does not add the MCP connector, Claude/ChatGPT skill, live voice, reviewer-model
auto-apply, desktop/Android UI wiring, task sync, calendar event sync, or schedule
write-back. Task and fixed-event data remain client-supplied to the assistant until a
future data-model extension syncs them.
