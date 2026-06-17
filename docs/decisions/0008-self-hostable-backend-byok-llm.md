# ADR 0008: Self-hostable backend and bring-your-own multi-provider LLM

- Status: accepted
- Date: 2026-06-17
- Refines [ADR-0007](0007-connected-cloud-architecture.md) by resolving its open
  questions (backend/identity, LLM provider/terms, target markets).

> Engineering and compliance notes only — not legal advice.

## Context

ADR-0007 made ZeitBoard a connected cloud app and left three open questions. The owner
resolved them, and the answers pull the posture back toward **user sovereignty**:
connected, but neither vendor-hosted nor vendor-keyed.

## Decision

1. **Self-hostable backend (entirely).** The server that syncs the user's data across
   devices (companion ↔ server ↔ desktop) is **entirely self-hostable**. The project
   ships the server software (Go); the user — or whoever they designate — runs it. There
   is **no mandatory hosted service operated by the project**, and the project receives
   no telemetry. The operator of an instance is the data controller; the project is a
   software supplier.
2. **Bring-your-own, multi-provider LLM (BYOK).** The assistant/agent LLM is
   **self-provided** through a provider abstraction modeled on **OpenCode** (see the
   OpenCode repo for the reference architecture). Integrated provider support:
   **OpenCode Zen, OpenRouter, OpenAI, and Anthropic**, with the **OpenCode Go**
   implementation as the reference for the backend's provider layer. The project ships
   **no API keys**; the user supplies their own. LLM context flows only to the provider
   the user selects, under that provider's terms — surfaced in-app, but the user's
   relationship and responsibility.
3. **Legal scope: US / North Carolina.** The project targets US law, specifically North
   Carolina. Compliance with laws outside the US/NC is the responsibility of the
   user/operator.

## Consequences

- **User-sovereign privacy, not "managed cloud."** Self-hosted backend + BYOK LLM means
  the user controls hosting and the model relationship; the project never holds health
  data or keys. This is materially more private than a vendor-run cloud and shrinks the
  project's own obligations. It also supersedes the "convenience-first / managed cloud"
  reading of ADR-0007: connected, yes; vendor-hosted, no.
- **The project's compliance surface is mainly honest representation** (NC UDAP,
  N.C.G.S. § 75-1.1 — don't misrepresent privacy/security) plus **documenting
  breach-notification considerations** (NC Identity Theft Protection Act, § 75-60 et
  seq.) for whoever operates an instance. HIPAA does not apply (no covered entity), and
  NC has no comprehensive consumer-privacy statute as of this writing. Data-protection
  obligations of any chosen LLM provider are between the **user** and that provider.
- **Backend is Go and OpenCode-referenced**, and can build on the NoobBoard Go server
  patterns the assistant spec already cites. A **self-hosting runbook** (config, TLS,
  storage, backup, key handling) is a required deliverable since the operator runs it.
- **Transport is authenticated + TLS; data is encrypted at rest** on the self-hosted
  instance, with the operator holding the keys.
- **Provider layer requirements:** store BYOK credentials in a secret store / OS
  keychain, never log them, disclose the active provider in the UI, send only
  minimized/redacted context, and degrade gracefully when no provider is configured
  (local-only Q&A where feasible).

## Status / next steps

`privacy.md` is reframed to the self-host / BYOK / US-NC model; `threat-model.md` still
needs a full rewrite around the **self-hosted-instance** trust boundary (operator as
defender; new assets = the server, the sync transport, and BYOK credentials). The
**self-hostable Go backend + BYOK provider layer** is the gating workstream before
companion↔server sync.
