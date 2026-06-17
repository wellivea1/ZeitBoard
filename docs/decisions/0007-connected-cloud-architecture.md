# ADR 0007: Connected cloud architecture (convenience-first)

- Status: accepted
- Date: 2026-06-17
- Supersedes the local-first / offline-by-default posture assumed by the initial
  scaffold (commit `c8fba7f`) and the sync-related assumptions in
  [`future-relay-design.md`](../future-relay-design.md) and
  [`threat-model.md`](../threat-model.md).

## Context

The initial scaffold + UI/UX brief assumed a **local-first, offline-by-default**
posture: the private domain model never leaves the device; the only egress was a
minimal encrypted trusted-view projection (the relay) and an opt-in, off-by-default
cloud LLM. That assumption shipped in the first import commit and was not an explicit
owner requirement.

In practice ZeitBoard is a connected client–server product: the companion app syncs
with a server/desktop and an LLM connection is part of normal operation. The owner
chose a **convenience-first** posture: standard cloud sync + cloud LLM with
conventional app privacy.

## Decision

ZeitBoard is a **connected cloud application**; connectivity is required for normal
operation.

- The user's data — including the private domain model (observations, corrections,
  estimates, schedule) — **syncs to their account in the cloud** and across their
  devices (companion ↔ server ↔ desktop). The previous "never serialize the private
  model off-device" rule is replaced by "sync the private model to the user's own
  account over an authenticated, encrypted channel."
- A **cloud LLM is a standard assistant/agent backend** (no longer off-by-default).
  The active backend is still disclosed in the UI.
- Conventional app privacy applies: authentication/accounts, encryption in transit and
  at rest, no advertising, no data brokerage, and user-facing export + deletion.

## Consequences

- **New first-class component: a sync backend/server.** None exists today (the desktop
  app is a Wails client; there is no account system, auth, or sync service). This is now
  the critical missing piece and a prerequisite for companion↔desktop sync — a new
  roadmap workstream.
- **Sharing to *other people* stays default-deny and projection-only** (ADR-0003 is
  unchanged): syncing to the user's *own* account is not the same as exposing data to
  third parties. The trusted-view allowlist still governs what other humans can see.
- **`future-relay-design.md` is superseded for sync** (it was explicitly *not* a sync
  engine); a real backend replaces it. The relay may still serve the trusted-view
  sharing path.
- **`threat-model.md` needs a rewrite.** The "no health payload leaves the device"
  invariant is gone. New assets/surfaces: accounts and auth, server-side storage of
  identifiable health data, cloud-provider access, transport, and the LLM provider's
  data handling.
- **Health/circadian data is special-category.** Doing convenience-first *correctly*
  still requires (non-optional where these markets are targeted): explicit consent for
  collecting and processing health data (e.g. GDPR special category), user export +
  deletion rights, an **LLM provider tier that does not train on or retain the data**
  (a DPA / zero-retention business tier), and Apple App Store / Google Play health-data
  policy compliance. These are engineering and compliance facts, not optional extras.
- Fixtures and tests stay synthetic (good practice), but the "synthetic-only because
  nothing leaves the device" rationale no longer applies.

## Open questions — resolved by [ADR-0008](0008-self-hostable-backend-byok-llm.md)

- **Backend & identity:** **entirely self-hostable** Go server (no mandatory
  project-run service); the operator is the data controller.
- **LLM provider & terms:** **bring-your-own-key, multi-provider** (OpenCode Zen,
  OpenRouter, OpenAI, Anthropic), modeled on OpenCode; the project ships no keys.
- **Target markets:** **US / North Carolina**; compliance elsewhere is the user's.

This refines the "convenience-first / managed cloud" framing above toward a
**user-sovereign** model: connected, but self-hosted and self-keyed.

## Status / next steps

Recorded as accepted (owner decision). `AGENTS.md` privacy rules are reconciled to this
posture now; `privacy.md`, `threat-model.md`, and `future-relay-design.md` carry an
"under revision per ADR-0007" banner and are rewritten once the backend/identity model
and LLM provider are chosen. The **sync backend** is the gating new workstream and a
prerequisite for the companion↔server sync this assumes.
