# Threat model

Architecture: **connected, self-hosted, bring-your-own-key**
([ADR-0007](decisions/0007-connected-cloud-architecture.md),
[ADR-0008](decisions/0008-self-hostable-backend-byok-llm.md)). The user runs their own
backend instance; their data syncs to *that* instance; the assistant talks to an LLM
provider the user configures with their own key. The project ships software and operates
no service. This model is not legal advice.

## Roles

- **Operator** — whoever runs the self-hosted backend (usually the user themselves). The
  operator is the **data controller and the primary defender**: they hold the storage,
  the encryption keys, the TLS material, and the BYOK provider credentials.
- **Project** — ships the client + server software. Runs no hosted service, holds no user
  data or keys, and receives **no telemetry**. Its security duty is sound defaults, honest
  representations, and a hardening runbook for operators.
- **LLM provider** — the user's chosen provider (OpenCode Zen / OpenRouter / OpenAI /
  Anthropic). Sees only the minimized, redacted context the user's instance sends under
  the user's key; its data handling is the **user's** relationship and responsibility.

## Assets

Private observations, corrections, estimates, schedules, and user-entered
health-related data; the self-hosted **server** and its storage; the **sync transport**
between devices and the server; **BYOK provider credentials**; **share permissions** and
**trusted-view output**. Confidentiality and permission correctness rank above
availability.

## Trust boundaries

1. Platform collectors → Go core.
2. Core → local storage (SQLite on each device).
3. Device ↔ **self-hosted sync server** (network; authenticated; TLS). *New.*
4. Server → **storage at rest** on the operator's instance. *New.*
5. Core/server → **BYOK LLM provider** — minimized, redacted context egresses under the
   user's key. *New.*
6. App ↔ **external agent** via MCP connector / skill — propose-only, allowlisted
   ([ADR-0006](decisions/0006-agent-accessible-interface.md)). *New.*
7. Wails service ↔ desktop frontend.
8. Android permission APIs ↔ Android repositories.
9. Private state → **trusted-view projection** → trusted recipient (default-deny).
10. External import (e.g. Google "My Activity" Takeout) → core. *New.*
11. Development tools / CI → synthetic fixtures only.

## Adversaries

A network attacker on the device↔server path; someone who obtains the server host or a
backup; a buggy or malicious **external agent** (MCP client); a malformed or hostile
**import file**; a supply-chain/CI compromise; a trusted-view recipient who retains a
projection. **Out of scope:** a fully compromised OS/device or server host, a malicious
operator against their own data, and nation-state adversaries.

## Assumptions

- The operating-system account, device, and server host are not fully compromised.
- The operator enables at-rest encryption and TLS and keeps the host patched (the
  runbook states this).
- Official toolchains and pinned dependencies are obtained over authenticated channels.
- Users understand the visible permission, provider, sharing, and revocation controls.

## Implementation status

The **local core** mitigations below are implemented (append-only model, projection
allowlist, typed refusal, deterministic fixtures, the agent/assistant propose-only
contract spec). The **Milestone 1 sync backend** is implemented as a self-hosted Go
daemon with TLS serving, per-device bearer tokens, token-hash storage, strict sync
validation, idempotent append-only records, and AES-256-GCM encrypted payloads at rest.
The **Milestone 2 BYOK provider layer and assistant backend** are implemented with
provider disclosure, strict JSON action validation, redacted context, pending proposals,
one-use approval tokens, and encrypted proposal/audit storage. The **Milestone 3
server-side read layer** is implemented with effective sleep-session replay from synced
observations/corrections, core-engine overview/rhythm/accuracy projections, typed
estimation refusals, and authenticated access. The **Milestone 4 local MCP connector** is
implemented as a stateless adapter over the backend API with read tools, propose-only
tools, call budgets, and no approval/apply tool. **Server-side erasure (ADR-0017)** is
implemented: an authenticated erase endpoint hard-deletes synced payloads, tombstones
block resurrection, and devices apply pulled tombstones as local hard-deletes. The
**cloud skill wrapper and live trusted-view transport do not exist yet**; their rows
remain design requirements for future workstreams, not current guarantees.

## Threats and mitigations

| Threat | Impact | Mitigation |
| --- | --- | --- |
| Over-broad platform permission | Unrelated private data becomes accessible | Request only required permissions; collection permission-gated and user-initiated; isolate adapters |
| Unauthenticated or MITM sync | Private model intercepted or altered in transit | TLS serving; per-device bearer credentials; token hashes stored server-side; repeated record IDs are idempotent no-ops and conflicting payloads are rejected |
| Server host compromise or stolen backup | Full private history disclosed | Operator-keyed AES-256-GCM payload encryption at rest; no project telemetry; self-hosting runbook covers TLS, key handling, and encrypted backups |
| BYOK credential leak | Provider key stolen or abused | Provider keys are loaded from env or secret files, never returned by status APIs, never logged, never placed in LLM context, and never projected |
| Over-broad context to the LLM provider | Health data over-exposed to a third party | The assistant builds role-scoped context field-by-field, omitting medication names, diagnosis, raw behavioral timestamps, full calendar text, tokens, and secrets; provider status discloses the active provider/model |
| Server read projection leak | Synced private records exposed through read APIs | Overview/rhythm/accuracy endpoints require device auth, replay only decrypted store records internally, return projection DTOs instead of sync envelopes, sanitize internal observation IDs, and test for forbidden fields |
| Assistant or agent mutation | Schedule changed without consent | Model emits only allowlisted actions the server resolves into proposals; approval queue; one-use signed token; no direct mutation path (tested) |
| Malicious external agent (MCP/skill) | Exfiltration or unauthorized change | Local MCP exposes allowlisted read projections only (never the raw model), propose-only tools, call budgets, and no approval/apply tool; cloud skills remain future and require a separate privacy review |
| Erased data lingering on the instance or other devices | A hard-deleted record survives elsewhere, defeating the erasure right | Authenticated `/v1/sync/erase` hard-deletes the synced payload and mints tombstones (record-id only, no health data) that every device applies on pull; the tombstone registry makes re-pushing an erased id a silent no-op (no resurrection). Residual: a device that never syncs again keeps its copy until it does (ADR-0017) |
| Malicious import (Takeout / My Activity) | Resource exhaustion; misleading inference | Size limits, strict schemas, bounded strings/arrays, transactional validation; inferred sleep marked low-confidence (`inferred`), never overclaimed |
| Source mutation | Audit history and estimator support become misleading | Append-only observations and corrections; effective read model; persistence tests |
| Time-zone confusion | Incorrect drift or schedule proposals | UTC instants plus IANA zones; half-open intervals; DST-focused tests |
| Estimator overclaim | Uncertain data appears authoritative | Typed refusal, ordinal confidence, widening windows, constrained product language; backtest harness measures calibration |
| Fixed-event mutation | User calendar intent is changed | Immutable input DTOs; proposals are separate outputs |
| Projection regression | Private fields enter a trusted view | Closed allowlisted DTO, explicit permissions, forbidden-key fixture checks, projection tests |
| Stale or revoked share | Recipient retains unintended access | Expiry and revocation checked before projection; no view on inactive profile |
| Sensitive logging | Payloads leak into support bundles or CI | Structured redaction; log categories/counts, never values, tokens, or exact timestamps |
| Dependency or CI compromise | Build or release artifacts are altered | Pinned tool/action versions, lockfiles, minimal workflow permissions, dependency review |
| Real data in fixtures | Private data enters source control | Deterministic synthetic generator, review policy, CI fixture regeneration check |

## Legal scope (US / North Carolina — not legal advice)

The project targets US law, specifically North Carolina; compliance outside the US/NC is
the operator's responsibility. The **operator** of an instance is the data controller and
bears any breach-notification duty under the **NC Identity Theft Protection Act**
(N.C.G.S. § 75-60 et seq.). The **project** must not misrepresent its privacy or security
practices (**NC UDAP**, § 75-1.1). HIPAA does not apply (no covered entity), and NC has no
comprehensive consumer-privacy statute as of this writing. Any data sent to an LLM
provider is governed by the agreement between the **user and that provider**.

## Residual risks

No protection against a fully compromised OS, device, or server host; the user's chosen
LLM provider's own handling of context it receives (outside project control — the user's
relationship); a trusted recipient copying a projection before revocation; secure-deletion
guarantees on all filesystems. The desktop webview, Android runtime, and the operator's
server host remain part of the trusted computing base.

## Security verification

- Sync: assert authentication is required, transport is TLS, and replayed messages are
  rejected.
- Agent/assistant: assert there is no path to mutate the schedule except by creating a
  pending proposal (no direct-mutation API); MCP exposes no approval/apply tool; read
  tools expose only allowlisted projection fields.
- Credentials: assert BYOK keys are never logged, projected, or placed in LLM context.
- LLM context: assert outbound context is minimized and redacted (no forbidden fields).
- Projection: test output against a forbidden-field list across all permission
  combinations; revoked and expired profiles return no view.
- Import: fuzz or property-test Takeout/import and correction-chain parsing.
- Logs: scan integration-test logs for representative sensitive values.
- Process: revisit this model when adding a data source, an LLM provider, a sync change,
  or any new external surface; review platform permissions and network calls each release.
