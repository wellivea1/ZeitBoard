# Threat model

Architecture: **connected, self-hosted, bring-your-own-key**
([ADR-0007](decisions/0007-connected-cloud-architecture.md),
[ADR-0008](decisions/0008-self-hostable-backend-byok-llm.md)). The user runs their own backend
instance; their data syncs to _that_ instance; the assistant talks to an LLM provider the user
configures with their own key. The project ships software and operates no service. This model is not
legal advice.

## Roles

- **Operator** — whoever runs the self-hosted backend (usually the user themselves). The
  operator is the **data controller and the primary defender**: they hold the storage,
  the encryption keys, the TLS material, and the BYOK provider credentials.
- **Project** — ships the client + server software. Runs no hosted service, holds no user
  data or keys, and receives **no telemetry**. Its security duty is sound defaults, honest
  representations, and a hardening runbook for operators.
- **LLM provider** — the user's chosen provider (OpenCode Zen / OpenRouter / OpenAI /
  Anthropic), or an OpenAI-compatible model the user runs. Sees only the minimized, redacted
  context the user's instance sends under the user's key, over HTTPS unless it runs on the
  same machine; its data handling is the **user's** relationship and responsibility.

## Assets

Private observations, corrections, estimates, schedules, and user-entered
health-related data; the self-hosted **server** and its storage; the **sync transport**
between devices and the server; **BYOK provider credentials**; **share permissions** and
**trusted-view output**. Confidentiality and permission correctness rank above
availability.

## Trust boundaries

1. Platform collectors → Go core.
2. Core → local storage (SQLite on each device).
3. Device ↔ **self-hosted sync server** (network; authenticated; TLS). _New._
4. Server → **storage at rest** on the operator's instance. _New._
5. Core/server → **BYOK LLM provider** — minimized, redacted context egresses under the user's key.
   _New._
6. App ↔ **external agent** via MCP connector / skill — propose-only, allowlisted
   ([ADR-0006](decisions/0006-agent-accessible-interface.md)). _New._
7. Wails service ↔ desktop frontend.
8. Android permission APIs ↔ Android repositories.
9. Private state → **trusted-view projection** → trusted recipient (default-deny).
10. Owner-selected v1 sleep file → local importer; owner-selected Google Takeout / My Activity
    export → times-only reader (ADR-0059) → local store; Windows System log → six power entries,
    with the owner's second choice (ADR-0060) → desktop activity records.
11. Development tools / CI → synthetic fixtures only.
12. Local agent client (MCP) ↔ **desktop-local endpoint** on loopback: a same-machine boundary
    crossed with a bearer token, guarded by a loopback bind and per-request check, rejection of any
    request carrying an `Origin` header, and an owner-restricted descriptor (ADR-0028).
13. Link recipient ↔ **availability portal** on the self-hosted instance: the only boundary in the
    system crossed by someone the user has not enrolled. Its defining property is a _split store_ —
    public handlers are constructed with a separate portal database and the portal package cannot
    import the private store, so the boundary is enforced by the dependency graph rather than by
    output filtering (ADR-0029). Off by default; not exposed.
14. Private read model → **allowlisted portal snapshot**: the one inbound path to the portal
    database, owned by `portalbridge`, narrowing an estimate to windows, a generation time, a
    horizon, and a status, and, only for a link with that grant, the times of doses logged as
    skipped in the last three days (ADR-0058). Confidence labels are withheld because ADR-0022 measured them inverted.
    Since ADR-0033 the stored windows are unfiltered and unclipped, with both rules applied at
    render, so the snapshot holds a few already-elapsed windows for the same profile that was
    already granted them. The generation time now means when the content last changed, so a
    visitor's staleness warning tracks the evidence rather than the last unrelated sync.
15. Visitor request → **owner proposal queue**, the first boundary in the system an outsider may
    write across (ADR-0030). It is crossed only by a transactional outbox with a stable idempotency
    key, never by a direct call: the portal stores a request durably and a bridge files exactly one
    pending proposal, which is decided with the same one-use token as every other proposal. Approval
    must name an exact block inside the requested window. `place_visitor_request` is absent from the
    agent action registry, so an agent cannot forge a proposal that appears to come from a real
    person.

## Adversaries

A network attacker on the device↔server path; someone who obtains the server host or a
backup; a buggy or malicious **external agent** (MCP client); a malformed or hostile
**import file**; a supply-chain/CI compromise; a trusted-view recipient who retains a
projection. **Out of scope:** a fully compromised OS/device or server host, a malicious
operator against their own data, and nation-state adversaries.

## Assumptions

- The operating-system account, device, and server host are not fully compromised.
- The operator enables at-rest encryption and TLS and keeps the host patched (the
  runbook states this). This assumption covers the **instance**. It has never
  covered the user's own computer, where the database is unencrypted; ADR-0035
  corrected the documentation that implied otherwise.
- Official toolchains and pinned dependencies are obtained over authenticated channels.
- Users understand the visible permission, provider, sharing, and revocation controls.

## Implementation status

The **local core** mitigations below are implemented (append-only model, projection allowlist, typed
refusal, deterministic fixtures, the agent/assistant propose-only contract spec). The **Milestone 1
sync backend** is implemented as a self-hosted Go daemon with TLS serving, per-device bearer tokens,
token-hash storage, strict sync validation, idempotent append-only records, and AES-256-GCM
encrypted payloads at rest. The **Milestone 2 BYOK provider layer and assistant backend** are
implemented with provider disclosure, strict JSON action validation, redacted context, pending
proposals, one-use approval tokens, and encrypted proposal/audit storage. The **Milestone 3
server-side read layer** is implemented with effective sleep-session replay from synced
observations/corrections, core-engine overview/rhythm/accuracy projections, typed estimation
refusals, and authenticated access. The **Milestone 4 local MCP connector** is implemented as a
stateless adapter over the backend API with read tools, propose-only tools, call budgets, and no
approval/apply tool. **Server-side erasure (ADR-0017)** is implemented: an authenticated erase
endpoint hard-deletes synced payloads, tombstones block resurrection, and desktop clients apply
pulled tombstones as local hard-deletes. Android applies pulled records and erasures atomically with
its cursor, invalidates affected cached forecasts, and persists Health Connect re-import suppression
across re-enrollment. Android has separately consented background sleep sync, durable bounded retry
and enrollment generations; HTTP redirects cannot forward credentials, and uploads cannot promote
provider revisions to human confirmation. The server rejects new corrections targeting erased
observations and removes retained dependent revisions during erasure (ADR-0038). Clients trust the owner's server through
the system's roots plus at most one certificate the owner chose (ADR-0055), with verification on.
Skipping verification is refused everywhere except localhost. The daemon's kept localhost key is
readable by the owner only. Source reports
(ADR-0054) carry only check outcomes, check instants and a held count. The server validates them
with the same `core/sources` code the desktop uses and reads none of them. A forged report could
only misstate why records stopped. It cannot add, move or hide a sleep record. The **local v1
sleep import boundary (ADR-0022)** is implemented with an 8 MiB / 20,000-row cap, strict JSON/CSV
shape, per-row errors, source-id conflict detection, and atomic all-or-nothing commit. **Local
medication evidence and user-authored schedules (ADR-0024/0025)** are implemented with private
device-only definitions, immutable events, correction chains, strict civil-time expansion, neutral
sleep-collision forecasts, opt-in claim-first desktop reminders, contract export, and byte-checked
hard erasure; no medication payload sync or agent projection exists in M-A/M-B. **Local rhythm
context markers (ADR-0026)** are implemented as immutable manual/user-reported records with strict
past-time and civil-zone validation, display-only actogram joins, owner-initiated contract export,
typed byte-checked erasure, and no estimator, sync, sharing, agent, or logging path. **The local
clinician context report (ADR-0027)** is implemented as a redaction-first Go projection over
effective local records, with explicit-record adherence, range-scoped drift, non-causal association,
typed export confirmation, and standalone script-free HTML; it adds no sync, trusted-view, agent, or
network surface. **Packaging for Claude (ADR-0061)** adds files, not a network surface: a Claude
Desktop extension around the existing local connector, and a skill. The **live trusted-view
transport does not exist yet**; its rows remain design requirements for future workstreams, not
current guarantees.

## Threats and mitigations

| Threat                                                                  | Impact                                                                                                     | Mitigation                                                                                                                                                                                                                                                                                                                                                                                            |
| ----------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Over-broad platform permission                                          | Unrelated private data becomes accessible                                                                  | Request only required permissions; collection permission-gated and user-initiated; isolate adapters                                                                                                                                                                                                                                                                                                   |
| Unauthenticated or MITM sync                                            | Private model intercepted or altered in transit                                                            | TLS serving; per-device bearer credentials; token hashes stored server-side; repeated record IDs are idempotent no-ops and conflicting payloads are rejected                                                                                                                                                                                                                                          |
| Server host compromise or stolen backup                                 | Full private history disclosed                                                                             | Operator-keyed AES-256-GCM payload encryption at rest; no project telemetry; self-hosting runbook covers TLS, key handling, and encrypted backups                                                                                                                                                                                                                                                     |
| Another account on the user's own computer                              | Full local history, and the device's bearer token, read from disk                                          | The database, its write-ahead log, the backend token, the settings files and every export carry a protected DACL granting only the account that created them (a real ACL, not a mode argument Windows ignores); the permission is read back and asserted, and Settings reports what was checked (ADR-0035)                                                                                            |
| Stolen or imaged disk, or a program running as the user                 | Full local history disclosed                                                                               | **Not mitigated.** The local database is not encrypted. `modernc.org/sqlite` is CGo-free by choice and offers no VFS or serialize hook to put a cipher behind; SQLCipher needs CGO. ADR-0035 records the analysis. Users are told this in Settings and in privacy.md so they can decide about full-disk encryption themselves                                                                         |
| BYOK credential leak                                                    | Provider key stolen or abused                                                                              | Provider keys are loaded from env or secret files, never returned by status APIs, never logged, never placed in LLM context, and never projected                                                                                                                                                                                                                                                      |
| Over-broad context to the LLM provider                                  | Health data over-exposed to a third party                                                                  | The assistant builds role-scoped context field-by-field, omitting medication names, diagnosis, raw behavioral timestamps, full calendar text, tokens, and secrets; provider status discloses the active provider/model                                                                                                                                                                                |
| Server read projection leak                                             | Synced private records exposed through read APIs                                                           | Overview/rhythm/accuracy endpoints require device auth, replay only decrypted store records internally, return projection DTOs instead of sync envelopes, sanitize internal observation IDs, and test for forbidden fields                                                                                                                                                                            |
| Assistant or agent mutation                                             | Schedule changed without consent                                                                           | Model emits only allowlisted actions the server resolves into proposals; approval queue; one-use signed token; no direct mutation path (tested)                                                                                                                                                                                                                                                       |
| Malicious external agent (MCP/skill)                                    | Exfiltration or unauthorized change                                                                        | Local MCP exposes allowlisted read projections only (never the raw model), propose-only tools, call budgets, and no approval/apply tool. The Claude packages (ADR-0061) add no tool and no authority: the extension runs the installed connector, and the skill is guidance a model may ignore without gaining anything. Returned data excludes free text; the skill says to treat results as data, and any change is still a proposal. Packages are written only where the owner chooses, and the bundle is unsigned, so documentation says to make it from ZeitBoard |
| Local files read from another account, by an administrator, or from another operating system | Sleep history and the backend credential exposed off this account                                       | Owner-only DACLs on every private file (ADR-0035). By choice, Windows file encryption of the data folder and each file in it, applied at start before the store opens and read back per file; only a saved choice is acted on; the readout claims encryption only once Windows reports every file encrypted (ADR-0063). Residual: programs running as the owner, exports saved elsewhere, and a lost key if the owner skipped Windows' backup |
| Erased data lingering on the instance or other devices                  | A hard-deleted record survives elsewhere, defeating the erasure right                                      | Authenticated `/v1/sync/erase` hard-deletes synced payloads and dependent corrections and mints metadata-only tombstones. Desktop and Android clients apply them atomically; Android suppresses erased Health Connect re-imports across re-enrollment. The server rejects erased IDs and later corrections targeting them. Residual: a client without a completed pull keeps its copy (ADR-0017/0038) |
| Later task revision resurrects a deleted task                           | A stale or concurrently offline device publishes a revision whose record id did not exist at deletion time | Erasing any known task revision atomically erases every retained revision and records the logical task id; future revisions for that task are silent no-ops, while record-level tombstones still propagate deletion to clients (ADR-0017/ADR-0020)                                                                                                                                                    |
| Malicious or malformed local sleep import                               | Resource exhaustion; hidden row loss; misleading estimator input                                           | 8 MiB and 20,000-row caps; strict v1 JSON/canonical CSV; only principal/nap `file_import` sleep rows; per-row errors; invalid/conflicting rows block the whole transaction; duplicate source IDs are explicitly reported; no payload logging                                                                                                                                                          |
| Malicious or oversized ICS/CalDAV source                                | Resource exhaustion, recurrence explosion, parser ambiguity, or network pivot                              | Input/response/component/occurrence/work caps; strict content-line parsing; bounded recurrence horizon; HTTPS except loopback; cross-origin redirects refused for CalDAV, and for subscribed addresses (ADR-0062) redirects allowed only while they stay on HTTPS; sanitized endpoint persistence; atomic preview/commit; a failed hourly refresh keeps the last copy |
| Calendar address or CalDAV credential disclosure                        | Password or secret-bearing URL survives locally or leaks in logs                                           | Import credentials are one-shot request fields and cleared by the UI; userinfo/query secrets and non-loopback HTTP endpoints are rejected; calendar payloads and endpoints are excluded from logs. The write-back sign-in (ADR-0053) is kept only while the owner has writing on, in the owner-protected database like the backend token, is sent only to its calendar's origin (cross-origin redirects are refused), never appears in a status, log or export, and is erased on stopping or removing the calendar. A subscribed calendar's private address (ADR-0062) is kept in its own table in the same database, cascades with the calendar, is shown by host only, and appears in no message, status, log, sync or export: fetch errors name the host alone |
| Calendar account sign-in intercepted or its token leaked (ADR-0064)    | A code or refresh token lets someone else read the owner's calendars                                      | The owner's own app registration, so no third party holds a client or sits between the owner and the provider; sign-in in the system browser with PKCE (S256) and a random state compared in constant time; the answer is taken on a loopback port opened for it alone, closed after one answer, a cancel or five minutes, so a stolen code is useless without the verifier; read-only scopes; the refresh token and client secret are kept in the owner-protected database (and, by choice, Windows' encryption, ADR-0063), sent only to the provider's fixed endpoints, never shown, synced, exported or logged, and no error carries a token or response body; access tokens stay in memory; paging may not leave the provider's host; removing the account erases the token with its calendars and compacts the database, and Google is asked to revoke the grant. Residual: a program running as the owner could read the token, and Microsoft grants are withdrawn in the account's own settings |
| Calendar write-back damages the owner's calendar                        | An owner's own event is overwritten or deleted, or an event the owner changed is removed                   | Off by default, one calendar at a time, after a PROPFIND shows the sign-in may add events; ZeitBoard writes only resources named by its own UIDs, directly in the collection; creates use If-None-Match and adopt only an object carrying the same UID; removals use If-Match on the ETag it was given, so a changed event waits for the owner; weak ETags are dropped; a redirect that turns a write into a read counts as a failure; imports recognise ZeitBoard's own events instead of re-importing them as busy time |
| Reading Windows' System log reads more than power state                 | Over-collection (other events, account identifiers, the computer's name); misleading inference            | Off unless desktop activity and a separate choice are both on; the query names six provider and event numbers; an entry is decoded for its provider, number, time and record number only, and its data, user and computer are never decoded or kept; a rendered entry is capped at 64 KiB and a read at 20,000 entries; records stay local, kept once each, and are erased with activity records; suggestions shadow-only behind ADR-0031's gate |
| Malicious or oversized Takeout / My Activity export                     | Resource exhaustion; content retained; misleading inference                                                | Owner-chosen file only; zip entries and JSON read as streams, never held whole; one entry may use at most 1 MiB and an export at most 10,000,000 entries; the decoder keeps only a header's presence and the time, so titles, links, products and places never leave it; times before 2000 or more than a day after reading are counted unusable; HTML exports refused; minutes kept locally, never synced or projected; suggestions shadow-only behind ADR-0031's gate; a fuzz test runs over the reader |
| Source mutation                                                         | Audit history and estimator support become misleading                                                      | Append-only observations and corrections; effective read model; persistence tests                                                                                                                                                                                                                                                                                                                     |
| Time-zone confusion                                                     | Incorrect drift or schedule proposals                                                                      | UTC instants plus IANA zones; half-open intervals; DST-focused tests                                                                                                                                                                                                                                                                                                                                  |
| Context marker mistaken for diagnosis or cause                          | A self-report annotation appears to explain or medically classify a rhythm change                          | Only four neutral user-report kinds exist; markers are structurally excluded from estimation and scheduling; UI and export provenance identify self-report; copy says they do not establish cause, diagnose, or recommend treatment                                                                                                                                                                   |
| Rhythm-marker text crosses a projection boundary                        | A private illness/travel note reaches a server, trusted recipient, agent, or log                           | Marker APIs exist only on the local desktop; the strict trusted-view schema rejects marker fields; no sync/MCP/assistant path exists; owner-initiated v1 export is the only egress                                                                                                                                                                                                                    |
| Rhythm-marker erase mistaken for suppression                            | Sensitive context remains after the owner expects deletion                                                 | Markers have no suppress or update operation; exact `DELETE` removes the row, checkpoints/truncates the WAL, vacuums SQLite, and is byte-tested with a unique private note                                                                                                                                                                                                                            |
| Medication correction mistaken for erasure                              | Sensitive raw evidence remains when the owner expected deletion                                            | Ordinary edits and exclusion append immutable corrections and say that evidence remains; separate event/definition erasure requires exact `DELETE`, cascades corrections, checkpoints the WAL, vacuums SQLite, and is byte-tested                                                                                                                                                                     |
| Duplicate or misleading medication reminder                             | Repeated prompts could be mistaken for a second dose instruction                                           | Reminders require an explicit owner-authored clock schedule and separate opt-in; an immutable unique occurrence claim is committed before notification delivery, delivery failures are not retried, and copy says "Reminder you set" rather than directing a dose                                                                                                                                     |
| Civil-time or forecast ambiguity                                        | A DST transition, device-zone change, or forecast limit misstates schedule feasibility                     | Schedules own an explicit IANA zone; DST gaps are skipped and reported, repeated times use the first occurrence, cycles advance by civil date, and every occurrence outside the actual estimator horizon is labeled unavailable                                                                                                                                                                       |
| Medication notification disclosure                                      | A private label appears on a shared or observed desktop                                                    | Medication notifications are off by default; the schedule editor discloses that opt-in allows the local OS notification surface to display the label, and control characters are stripped before delivery                                                                                                                                                                                             |
| Medication text crosses a projection boundary                           | Names, clinician rules, or private notes reach a server, agent, trusted view, or log                       | M-A/M-B/M-C have no sync or agent projection; local DTOs are explicit, reminder claims contain no text, exports are owner-initiated, and privacy tests/architecture require opaque IDs before any later projection is enabled                                                                                                                                                                         |
| Clinician report over-discloses private text                            | A saved report unexpectedly contains labels, notes, diagnosis, or location data                            | Redaction is applied in Go before preview/render; diagnosis and calendar/location are mandatory omissions; labels use aliases and both note classes are opt-in; the adapter reconciles the redaction list                                                                                                                                                                                             |
| Adherence or start comparison implies a missed dose or treatment effect | Sparse logging or simultaneous context is interpreted as medical causality                                 | Only explicit scheduled records enter the denominator; as-needed is separate; absence is never missed; each association side requires five episodes, uses robust descriptive slopes, lists possible confounders, and states that alignment does not establish cause                                                                                                                                   |
| Exported report loads active or remote content                          | Opening a local health artifact leaks data or executes injected markup                                     | `html/template` auto-escapes private text; the standalone document has no script/external assets and a default-deny CSP; export is typed-confirmed and no network call is made                                                                                                                                                                                                                        |
| Estimator overclaim                                                     | Uncertain data appears authoritative                                                                       | Typed refusal, ordinal confidence, widening windows, constrained product language; backtest harness measures calibration                                                                                                                                                                                                                                                                              |
| Fixed-event mutation                                                    | User calendar intent is changed                                                                            | Imported rows are database-guarded immutable; source removal is a separate confirmed erasure; approval writes only a separately owned ZeitBoard block; export filters to app-owned events                                                                                                                                                                                                             |
| Stale local proposal approval                                           | A task is placed against changed sleep or calendar evidence                                                | Proposal IDs bind task revision, estimate, interval, sleep snapshot, and text-free event snapshot; the decision transaction recomputes task, sleep, and event state and fails closed before writing                                                                                                                                                                                                   |
| Projection regression                                                   | Private fields enter a trusted view                                                                        | Closed allowlisted DTO, explicit permissions, forbidden-key fixture checks, projection tests                                                                                                                                                                                                                                                                                                          |
| Stale or revoked share                                                  | Recipient retains unintended access                                                                        | Expiry and revocation checked before projection; no view on inactive profile                                                                                                                                                                                                                                                                                                                          |
| Sensitive logging                                                       | Payloads leak into support bundles or CI                                                                   | Structured redaction; log categories/counts, never values, tokens, or exact timestamps                                                                                                                                                                                                                                                                                                                |
| Dependency or CI compromise                                             | Build or release artifacts are altered                                                                     | Pinned tool/action versions, lockfiles, minimal workflow permissions, dependency review                                                                                                                                                                                                                                                                                                               |
| Real data in fixtures                                                   | Private data enters source control                                                                         | Deterministic synthetic generator, review policy, CI fixture regeneration check                                                                                                                                                                                                                                                                                                                       |

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
consequence of the feature and not a defect. A link with the skipped-doses grant tells its
holder that the owner takes a medication on a schedule, and when doses were logged as skipped;
that is the grant's purpose, it is off unless chosen, and the screen says so before it can be
chosen. The timing floor on link resolution bounds
enumeration signal without eliminating it. Source identifiers are pseudonymous, not
anonymous. The portal database is encrypted with a key derived one-way from the daemon
data key, so reading it never yields the private key, but the two cannot be rotated
independently. No independent review has run against this surface, so exposure remains
prohibited: `portal.enabled` is false by default and the section 12 exposure gate in
`portal-design.md` is not yet satisfied.

Visitor requests add their own accepted residuals. A visitor may send unwanted text
within the caps; the owner sees it, which is the point, and per-link disabling and
revocation are the remedies. Approval discloses the chosen time to whoever holds the
requester cookie or the one-time code, including anyone the visitor passed it to.
Request limits are per portal session and per profile, so a visitor with several
sessions is bounded by the per-profile pending cap rather than the per-session daily
one. A visitor proposal is filed against an enrolled device because proposals are keyed
to a creating device and a visitor has none; with no enrolled device the request stays
visibly queued.

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
  source record IDs, task titles, correction IDs, the share label, a medication's name and ID,
  and a dose's note and ID, then assert none
  appears in any public response body, any response header, or the portal database file
  bytes including its write-ahead log; assert the availability JSON carries exactly the
  five allowlisted keys, and a sixth, `skippedDoses`, only on a link with that grant; assert unknown, expired, and revoked links produce byte-identical
  responses; assert an already-authenticated session dies the moment its link is revoked;
  assert a session for one link grants nothing on another; assert mutations require
  same-origin attestation and that a request with neither `Sec-Fetch-Site` nor a matching
  `Origin` is refused; assert the read limit and the per profile-and-source passcode
  backoff persist; assert the portal package does not import the private store, read
  model, estimator, or domain packages; assert no `/p/` route and no owner sharing route
  exists when the portal is disabled; assert a visitor request round-trips to a decision
  and back to a visitor-visible status, that a failing bridge leaves it queued rather
  than claiming delivery, that repeated pumps create exactly one proposal, that approval
  outside the requested window or of the wrong duration is refused, that the generic
  decision route refuses visitor proposals, that a decision token is one-use, that a
  visitor holding the link but not the requester secret cannot read another's request,
  and that a decline discloses no reason.
- Import: unit-test size/shape limits, row accounting, duplicate/conflict
  handling, transaction atomicity, DST ambiguity, recurrence work caps,
  CalDAV redirect/credential boundaries, ownership-preserving calendar export,
  and erasure; a fuzz test runs over the Takeout reader (`FuzzReadActivity`).
- Calendar write-back: against an in-memory CalDAV server, assert that a create never
  overwrites, a lost answer is adopted only for the same UID, a changed event is never
  removed without the owner, a redirected write is not taken as done, a failed write
  retries on its schedule, and stopping erases the sign-in and leaves the calendar alone.
- Logs: scan integration-test logs for representative sensitive values.
- Process: revisit this model when adding a data source, an LLM provider, a sync change,
  or any new external surface; review platform permissions and network calls each release.
