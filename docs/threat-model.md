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
10. Owner-selected v1 sleep file → local importer; future external adapters
    (e.g. Google "My Activity" Takeout) → core.
11. Development tools / CI → synthetic fixtures only.
12. Local agent client (MCP) ↔ **desktop-local endpoint** on loopback: a
    same-machine boundary crossed with a bearer token, guarded by a loopback
    bind and per-request check, rejection of any request carrying an `Origin`
    header, and an owner-restricted descriptor (ADR-0028).
13. Link recipient ↔ **availability portal** on the self-hosted instance: the
    only boundary in the system crossed by someone the user has not enrolled.
    Its defining property is a *split store* — public handlers are constructed
    with a separate portal database and the portal package cannot import the
    private store, so the boundary is enforced by the dependency graph rather
    than by output filtering (ADR-0029). Off by default; not exposed.
14. Private read model → **allowlisted portal snapshot**: the one inbound path
    to the portal database, owned by `portalbridge`, narrowing an estimate to
    windows, a generation time, a horizon, and a status. Confidence labels are
    withheld because ADR-0022 measured them inverted.

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
**local v1 sleep import boundary (ADR-0022)** is implemented with an 8 MiB / 20,000-row
cap, strict JSON/CSV shape, per-row errors, source-id conflict detection, and atomic
all-or-nothing commit. **Local medication evidence and user-authored schedules
(ADR-0024/0025)** are implemented with private device-only definitions,
immutable events, correction chains, strict civil-time expansion, neutral
sleep-collision forecasts, opt-in claim-first desktop reminders, contract
export, and byte-checked hard erasure; no medication payload sync or agent
projection exists in M-A/M-B. **Local rhythm context markers (ADR-0026)** are
implemented as immutable manual/user-reported records with strict past-time
and civil-zone validation, display-only actogram joins, owner-initiated
contract export, typed byte-checked erasure, and no estimator, sync, sharing,
agent, or logging path. **The local clinician context report (ADR-0027)** is
implemented as a redaction-first Go projection over effective local records,
with explicit-record adherence, range-scoped drift, non-causal association,
typed export confirmation, and standalone script-free HTML; it adds no sync,
trusted-view, agent, or network surface. The
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
| Erased data lingering on the instance or other devices | A hard-deleted record survives elsewhere, defeating the erasure right | Authenticated `/v1/sync/erase` hard-deletes the synced payload and mints metadata-only tombstones (record id plus original kind when known, no health content) that every device applies in one atomic pull-page transaction; the tombstone registry makes re-pushing an erased id a silent no-op (no resurrection). Residual: a device that never syncs again keeps its copy until it does (ADR-0017) |
| Later task revision resurrects a deleted task | A stale or concurrently offline device publishes a revision whose record id did not exist at deletion time | Erasing any known task revision atomically erases every retained revision and records the logical task id; future revisions for that task are silent no-ops, while record-level tombstones still propagate deletion to clients (ADR-0017/ADR-0020) |
| Malicious or malformed local sleep import | Resource exhaustion; hidden row loss; misleading estimator input | 8 MiB and 20,000-row caps; strict v1 JSON/canonical CSV; only principal/nap `file_import` sleep rows; per-row errors; invalid/conflicting rows block the whole transaction; duplicate source IDs are explicitly reported; no payload logging |
| Malicious or oversized ICS/CalDAV source | Resource exhaustion, recurrence explosion, parser ambiguity, or network pivot | Input/response/component/occurrence/work caps; strict content-line parsing; bounded recurrence horizon; HTTPS except loopback; cross-origin redirects refused; sanitized endpoint persistence; atomic preview/commit |
| CalDAV credential disclosure | Password or secret-bearing URL survives locally or leaks in logs | Credentials are one-shot request fields and cleared by the UI; passwords are never persisted; userinfo/query secrets and non-loopback HTTP endpoints are rejected; calendar payloads and endpoints are excluded from logs |
| Future Takeout / My Activity import | Resource exhaustion; misleading inference | Not implemented; must add bounded parsing and mark inferred sleep low-confidence (`inferred`) before it may affect estimation |
| Source mutation | Audit history and estimator support become misleading | Append-only observations and corrections; effective read model; persistence tests |
| Time-zone confusion | Incorrect drift or schedule proposals | UTC instants plus IANA zones; half-open intervals; DST-focused tests |
| Context marker mistaken for diagnosis or cause | A self-report annotation appears to explain or medically classify a rhythm change | Only four neutral user-report kinds exist; markers are structurally excluded from estimation and scheduling; UI and export provenance identify self-report; copy says they do not establish cause, diagnose, or recommend treatment |
| Rhythm-marker text crosses a projection boundary | A private illness/travel note reaches a server, trusted recipient, agent, or log | Marker APIs exist only on the local desktop; the strict trusted-view schema rejects marker fields; no sync/MCP/assistant path exists; owner-initiated v1 export is the only egress |
| Rhythm-marker erase mistaken for suppression | Sensitive context remains after the owner expects deletion | Markers have no suppress or update operation; exact `DELETE` removes the row, checkpoints/truncates the WAL, vacuums SQLite, and is byte-tested with a unique private note |
| Medication correction mistaken for erasure | Sensitive raw evidence remains when the owner expected deletion | Ordinary edits and exclusion append immutable corrections and say that evidence remains; separate event/definition erasure requires exact `DELETE`, cascades corrections, checkpoints the WAL, vacuums SQLite, and is byte-tested |
| Duplicate or misleading medication reminder | Repeated prompts could be mistaken for a second dose instruction | Reminders require an explicit owner-authored clock schedule and separate opt-in; an immutable unique occurrence claim is committed before notification delivery, delivery failures are not retried, and copy says "Reminder you set" rather than directing a dose |
| Civil-time or forecast ambiguity | A DST transition, device-zone change, or forecast limit misstates schedule feasibility | Schedules own an explicit IANA zone; DST gaps are skipped and reported, repeated times use the first occurrence, cycles advance by civil date, and every occurrence outside the actual estimator horizon is labeled unavailable |
| Medication notification disclosure | A private label appears on a shared or observed desktop | Medication notifications are off by default; the schedule editor discloses that opt-in allows the local OS notification surface to display the label, and control characters are stripped before delivery |
| Medication text crosses a projection boundary | Names, clinician rules, or private notes reach a server, agent, trusted view, or log | M-A/M-B/M-C have no sync or agent projection; local DTOs are explicit, reminder claims contain no text, exports are owner-initiated, and privacy tests/architecture require opaque IDs before any later projection is enabled |
| Clinician report over-discloses private text | A saved report unexpectedly contains labels, notes, diagnosis, or location data | Redaction is applied in Go before preview/render; diagnosis and calendar/location are mandatory omissions; labels use aliases and both note classes are opt-in; the adapter reconciles the redaction list |
| Adherence or start comparison implies a missed dose or treatment effect | Sparse logging or simultaneous context is interpreted as medical causality | Only explicit scheduled records enter the denominator; as-needed is separate; absence is never missed; each association side requires five episodes, uses robust descriptive slopes, lists possible confounders, and states that alignment does not establish cause |
| Exported report loads active or remote content | Opening a local health artifact leaks data or executes injected markup | `html/template` auto-escapes private text; the standalone document has no script/external assets and a default-deny CSP; export is typed-confirmed and no network call is made |
| Estimator overclaim | Uncertain data appears authoritative | Typed refusal, ordinal confidence, widening windows, constrained product language; backtest harness measures calibration |
| Fixed-event mutation | User calendar intent is changed | Imported rows are database-guarded immutable; source removal is a separate confirmed erasure; approval writes only a separately owned ZeitBoard block; export filters to app-owned events |
| Stale local proposal approval | A task is placed against changed sleep or calendar evidence | Proposal IDs bind task revision, estimate, interval, sleep snapshot, and text-free event snapshot; the decision transaction recomputes task, sleep, and event state and fails closed before writing |
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

The desktop-local agent endpoint adds two accepted residuals. It runs whenever the app
runs rather than behind an opt-in switch, so its exposure is bounded only by the loopback
bind, the bearer token, the `Origin` rejection, and the descriptor's owner-only ACL. And
any process already running as the user can read that descriptor and therefore drive the
endpoint with the user's own authority — it is a user-level boundary, not a sandbox. What
such a process could obtain is the allowlisted projection surface (including medication
timing facts and context markers) plus reversible appearance changes; it still cannot
approve or apply a schedule change, because no tool on any surface can.

The availability portal adds residuals of its own, all of them accepted rather than
solved. A recipient can screenshot or simply remember a window, which revocation cannot
reach; the create-link response states this. Sharing a live projection is inherently
observable — someone watching the link over time sees the user's rhythm shift, which is a
consequence of the feature and not a defect. The timing floor on link resolution bounds
enumeration signal without eliminating it. Source identifiers are pseudonymous, not
anonymous. The portal database is encrypted with a key derived one-way from the daemon
data key, so reading it never yields the private key, but the two cannot be rotated
independently. No independent review has run against this surface, so exposure remains
prohibited: `portal.enabled` is false by default and the section 12 exposure gate in
`portal-design.md` is not yet satisfied.

## Security verification

- Sync: assert authentication is required, transport is TLS, and replayed messages are
  rejected.
- Desktop-local endpoint: assert non-loopback callers, absent or invalid tokens, and any
  request carrying an `Origin` header are refused; assert the descriptor's DACL is
  owner-only and does not inherit; assert the medical refusal is byte-identical to the
  shared constant and that fact tools answer without a provider call.
- Agent/assistant: assert there is no path to mutate the schedule except by creating a
  pending proposal (no direct-mutation API); MCP exposes no approval/apply tool; read
  tools expose only allowlisted projection fields.
- Credentials: assert BYOK keys are never logged, projected, or placed in LLM context.
- LLM context: assert outbound context is minimized and redacted (no forbidden fields).
- Projection: test output against a forbidden-field list across all permission
  combinations; revoked and expired profiles return no view.
- Availability portal: plant canary values in private device labels, observation IDs,
  source record IDs, task titles, correction IDs, and the share label, then assert none
  appears in any public response body, any response header, or the portal database file
  bytes including its write-ahead log; assert the availability JSON carries exactly the
  five allowlisted keys; assert unknown, expired, and revoked links produce byte-identical
  responses; assert an already-authenticated session dies the moment its link is revoked;
  assert a session for one link grants nothing on another; assert mutations require
  same-origin attestation and that a request with neither `Sec-Fetch-Site` nor a matching
  `Origin` is refused; assert the read limit and the per profile-and-source passcode
  backoff persist; assert the portal package does not import the private store, read
  model, estimator, or domain packages; assert no `/p/` route and no owner sharing route
  exists when the portal is disabled.
- Import: unit-test size/shape limits, row accounting, duplicate/conflict
  handling, transaction atomicity, DST ambiguity, recurrence work caps,
  CalDAV redirect/credential boundaries, ownership-preserving calendar export,
  and erasure; fuzz/property tests remain required for future Takeout parsing.
- Logs: scan integration-test logs for representative sensitive values.
- Process: revisit this model when adding a data source, an LLM provider, a sync change,
  or any new external surface; review platform permissions and network calls each release.
