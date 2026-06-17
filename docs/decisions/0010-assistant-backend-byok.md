# ADR 0010: Assistant backend with BYOK providers

- Status: accepted
- Date: 2026-06-17
- Builds on [ADR-0006](0006-agent-accessible-interface.md),
  [ADR-0008](0008-self-hostable-backend-byok-llm.md), and
  [ADR-0009](0009-backend-sync-design.md).

## Context

Milestone 1 added the self-hosted sync daemon. Milestone 2 adds the server-side
assistant backend: a user message can produce either a local answer or a pending
schedule proposal. The LLM is a new risk surface. A provider credential proves only
that the instance can call a model; it does not authorize schedule changes.

## Decision

Add an assistant backend to `non24.app/server` with these packages:

- `internal/provider`: a small `LLM` interface and plain-HTTP provider implementations
  for OpenAI, Anthropic, OpenRouter, and OpenCode Zen. No vendor SDKs are used.
- `internal/assistant`: redacted context construction, medical refusal, strict JSON
  action validation, model retry/compaction behavior, and action resolution.
- `internal/store`: pending proposals, append-only audit entries, and one-use approval
  nonces, using the same AES-GCM payload encryption pattern as sync records.
- `internal/api`: authenticated assistant/proposal/provider endpoints.

## Provider Model

The operator configures provider name, model, API key or key file, and optional endpoint
through config/env. The project ships no keys. Settings and status APIs disclose only
provider name, model, and whether a usable credential is configured. Raw keys are never
returned, logged, stored in fixtures, or placed in model context.

Providers are credential transport only. They do not receive tools, file access, web
access, raw private models, device tokens, or the authority to mutate state.

## Redaction And Context

The assistant context is built field-by-field from an allowlisted request-scoped planning
snapshot: task IDs, durations, bounded civil-time windows, confidence, and coarse
schedule constraints. It does not serialize the private domain model. It omits medication
names, diagnosis, full calendar text, exact raw behavioral timestamps, tokens, and
secrets. If the context is too large, the server compacts lower-priority detail and
retries once with a smaller request.

## Strict JSON Action Contract

The model must return `contracts/v1/assistant-action.schema.json`:

- `recommended_action` is one of `propose_move_task`, `propose_place_task`,
  `propose_reminder_shift`, or `answer_only`.
- `target` contains only the structured fields needed to resolve the action.
- `answer` is optional plain assistant text and must obey product-language rules.

Unknown, invalid, unresolved, or over-budget output is normalized to an answer-only
unknown result and creates zero proposals.

## Action Resolution

The server resolves allowlisted actions through `core/scheduling.Scheduler`. The model
never chooses a final schedule mutation. It can only identify a task/action target. The
server validates the target against the request-scoped planning snapshot, runs the
scheduler over validated availability windows and immutable fixed events, and stores the
result as a pending proposal. Existing schedule-proposal explanation codes are reused.

There is no API path that applies a schedule change directly from model output.

## Approval Tokens And Audit

Every pending proposal receives a short-lived HMAC-signed one-use token containing the
proposal ID, action ID, device actor, resolved-target hash, replay nonce, and expiry. A
decision endpoint accepts approve/reject only with a valid token. Expired tokens, replayed
nonces, mismatched proposal IDs, and already-decided proposals are rejected.

Proposal lifecycle events are appended to an audit table. Sensitive proposal and audit
payloads are AES-GCM encrypted at rest.

## Scope

Milestone 2 includes provider configuration, provider disclosure, assistant message
handling, redaction, strict action validation, proposal creation, one-use approval
tokens, proposal listing, proposal decisions, device list/revoke, contracts, tests, and
docs.

Out of scope for Milestone 2: MCP connector, Claude/ChatGPT skill packaging, live voice,
desktop/Android UI wiring, reviewer-model auto-apply, calendar write-back, and arbitrary
web research.

## Next Steps

Milestone 3 is the server-side estimation/read-projection layer described in
[ADR-0011](0011-server-side-estimation.md): the backend computes estimates from synced
sleep data and exposes overview, rhythm, and accuracy projections without raw records.
Milestone 4 should then expose those read projections plus the M2 propose-only action
registry to the agent-accessible interface from ADR-0006: a local MCP connector first,
optional Claude/ChatGPT skill wrappers later, live voice supplied by the client, and the
same redaction, propose-only, approval-token, and audit rules.
