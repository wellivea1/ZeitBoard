# ADR 0007: Connected cloud architecture (convenience-first)

- Status: accepted
- Date: 2026-06-17
- Supersedes the local-first / offline-by-default posture assumed by the initial
  scaffold (commit `c8fba7f`) and the sync-related assumptions in
  [`future-relay-design.md`](../future-relay-design.md) and
  [`threat-model.md`](../threat-model.md).

## Context

The initial scaffold and UI/UX brief assumed a local-first, offline-by-default posture:
the private domain model never leaves the device; the only egress was a minimal
encrypted trusted-view projection and an opt-in, off-by-default cloud LLM. That
assumption shipped in the first import commit and was not an explicit owner
requirement.

In practice ZeitBoard is a connected client-server product: the companion app syncs with
a server/desktop, and an LLM connection is part of the intended assistant workflow. The
owner chose a convenience-first posture: cloud-style sync and LLM connectivity with
conventional app privacy.

## Decision

ZeitBoard is a connected application; connectivity is required for normal operation.

- The user's data, including the private domain model (observations, corrections,
  estimates, schedule), syncs across their own devices over an authenticated, encrypted
  channel. The previous "never serialize the private model off-device" rule is replaced
  by "sync the private model to the user's own backend instance."
- A connected LLM is a standard assistant/agent backend. The active backend is disclosed
  in the UI.
- Conventional app privacy applies: authentication, encryption in transit and at rest,
  no advertising, no data brokerage, and user-facing export and deletion.

## Consequences

- **New first-class component: a sync backend/server.** The desktop app is a Wails
  client; the backend is a separate Go module. It is a prerequisite for companion,
  server, and desktop sync.
- **Sharing to other people stays default-deny and projection-only** (ADR-0003 is
  unchanged): syncing to the user's own backend is not the same as exposing data to third
  parties. The trusted-view allowlist still governs what other humans can see.
- **`future-relay-design.md` is superseded for sync** because it was explicitly not a
  sync engine. A real backend replaces it for device sync. The relay may still serve the
  trusted-view sharing path.
- **`threat-model.md` is updated around the self-hosted-instance boundary.** Assets and
  surfaces include device auth, server-side storage of identifiable health data,
  transport, and future LLM-provider access.
- **Health/circadian data is sensitive.** Doing connected sync correctly still requires
  explicit consent where applicable, user export and deletion, careful LLM provider
  choices, and Apple App Store / Google Play health-data policy compliance where those
  distribution channels are used.
- Fixtures and tests stay synthetic.

## Open questions resolved by ADR-0008

- **Backend and identity:** entirely self-hostable Go server; the operator is the data
  controller.
- **LLM provider and terms:** bring-your-own-key, multi-provider, modeled on OpenCode;
  the project ships no keys.
- **Target markets:** US / North Carolina; compliance elsewhere is the user's.

ADR-0008 refines the convenience-first / managed-cloud framing above toward a
user-sovereign model: connected, but self-hosted and self-keyed.

## Status / next steps

Recorded as accepted (owner decision). ADR-0008 defines the self-hosted/BYOK posture.
ADR-0009 records the Milestone 1 sync backend design, and `apps/server` implements that
authenticated encrypted sync path. ADR-0010 records the Milestone 2 assistant backend
design, and `apps/server` implements BYOK provider transport plus propose-only assistant
actions. The MCP connector/skill layer remains future work.
