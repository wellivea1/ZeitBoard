# Meta review and phase goals (2026-07)

> Direction review against the ultimate goal, and one `/goal` prompt per
> phase — each prompt is self-contained and pasteable into an agent session.
> Engineering notes only; not medical advice.

## Ultimate goal (owner-stated)

A **local assistant, calendar, and disease-management app** with an
**entirely self-hostable backend**, where the backend also serves a
**public-facing web portal**: people the user shares a link with can see or
ask **when the user will be awake** and **request a specific time**, with a
**shared messaging/notification system** linked to that portal.

## Meta review: where the app stands against that goal

**What is genuinely done and load-bearing.** The safety architecture is
complete and tested end-to-end: propose-only mutation everywhere (UI, chat,
MCP), one-use decision tokens, redaction boundaries proven by tests (titles
and health data never reach providers), erasure-grade deletion with
tombstones, revision sync for mutable entities, BYOK multi-provider LLM, and
a validated-on-synthetic estimator with honest refusals. The UI now has an
identity (rhythm-first, semantic color, five presets incl. Amber glasses
mode, rhythm-linked night switching). This foundation is exactly what the
portal needs, because **a visitor's time request is just one more proposal
origin** — the queue, tokens, and audit already exist.

**Closed gate 1 — real-life validation** (roadmap slice 3). ADR-0022 delivers
strict local import, explicit source-reviewed coverage of the handwritten 2021
charts, measured chart-boundary uncertainty, and a combined 2021–2023
walk-forward backtest. It rejects blanket window tightening by measured delta
and exposes a confidence-window calibration/misfit follow-up. Phase 1's stated
acceptance criteria are met; broader pilot validation remains separate work.

**Closed gate 2 - real local calendar.** ADR-0023 delivers bounded read-only
ICS and CalDAV snapshots, real fixed events in planning, a local civil-time
board, persistent approved ZeitBoard placements, undo, source erasure, and an
app-owned ICS export. Imported event text remains local. Remote write-back to
an external calendar is still separately gated; the delivered path never
mutates an imported event.

**Closed gate 3 - local disease-management substance.** Medication slices M-A
through M-C are delivered under ADR-0024/0025/0027: local definitions,
append-only taken/skipped evidence, corrections, explicit user-authored
schedules, neutral collision
forecasts, opt-in claim-first desktop reminders, contract export, real erasure,
and the real Medications workspace. ADR-0026 also delivers local
disruption/travel/illness/forced-schedule context markers. ADR-0027 completes
explicit-record adherence, dose/start markers, descriptive association with
named possible confounders, and the redaction-first local clinician report.
M-D sync, M-E agent projection, and M-F sharing remain separate future gates.

**Closed gate 4 - the local assistant.** ADR-0028 delivers the desktop-local
agent endpoint ADR-0021 deferred: a loopback MCP the desktop app serves
itself, so a voice client can read state and switch appearance with the
backend off. Medication timing facts and context markers are answerable
through `ask_zeitboard_facts`, and the medical refusal now lives in shared
`core/agentpolicy` so chat, backend MCP, and the local endpoint cannot drift
apart. Cloud skill packaging remains separately gated.

**Gap 5 — the portal does not exist, and it is the largest threat-model
change in the project's history.** Today's trusted-web prototype is static
synthetic HTML; `future-relay-design.md` was written for exactly this
successor. New requirements beyond the old design: *interactive* ask
("when will they be awake?"), *time requests* that land in the approval
queue, and visitor↔user messaging with notifications. Public-facing means
anonymous-adjacent traffic against the same server that stores health data
— the design must keep the portal surface projection-only (allowlisted
availability fields, never health records), expiring, revocable,
rate-limited, and enumeration-proof.

**Gap 6 — notifications have no transport.** Portal requests and Medfriend-
style alerts need a push path that works self-hosted (web push with
operator VAPID keys; the Android companion as a second receiver). The
companion still has no sync client (slice 9).

**Sequencing logic.** Trust before reach: P1's estimator gate, P2's local
calendar gate, and P3's local disease-management substance through M-C are
closed. P4's local assistant gate is
closed as well. The next primary dependency is opening the portal (P5), then
fanning out notifications (P6); P5 can now build on P2's approved local
placement path and on P4's reviewed projection surface.

---

## Phase goals (`/goal` prompts)

### `/goal phase-1-ground-truth`

Status: achieved on 2026-07-19 via ADR-0022 and `verification.md`.

> **Goal: validate the estimator on the owner's real 2021–2023 sleep
> history and gate estimator changes on measured deltas.**
> Context: `docs/roadmap.md` slice 3, ADR-0019 (synthetic suite),
> `core/estimation` (Backtest), `contracts/v1` observation format,
> ADR-0014 export as the shape reference. Build the missing **local import
> path**: a v1 observation-set JSON/CSV import binding + Data Sources UI
> (append-only, provenance `imported`, dedupe by source record id, dry-run
> preview with per-row validation errors, never silently skipping). Support
> an owner-assisted transcription workflow (CSV template → converter →
> import). Run `Backtest` on the imported history; record point error,
> hit-rate, and calibration in `docs/verification.md`; then decide
> window-width tightening / misfit-signal work strictly by backtest delta.
> Invariants: no handwriting-OCR promises; refusals stay honest; imported
> data is erasable like everything else (ADR-0017 flows). Acceptance: the
> owner's real history imports cleanly, the backtest table is committed,
> and at least one estimator decision cites a measured delta.

### `/goal phase-2-real-calendar`

Status: achieved on 2026-07-22 via ADR-0023 and the calendar verification
record in `verification.md`. External-provider write-back remains out of scope.

> **Goal: make ZeitBoard a real calendar: imported events, real fixed
> events in planning, and write-back of approved placements.**
> Context: roadmap slice 6 + Phase 3c, `docs/data-model.md` fixed events,
> scheduling engine inputs, ADR-0018/0020 task model. First write the
> placement ADR: adapters live device-side first (ICS file + CalDAV
> read-only; no Google OAuth yet), imported event **text stays out of
> projections** (times/ids only reach the scheduler; titles render only
> locally like task titles). Then: contract for event sets, local storage,
> Calendar screen showing real events against predicted sleep/wake bands
> (retiring the five-day fixture), scheduler consuming real fixed events.
> Finally Phase 3c: applying an *approved* placement writes a ZeitBoard-
> owned calendar block (local store; exportable ICS), never edits imported
> events. Invariants: import is read-only; every write is an approved
> proposal; sync via revision records if events sync at all. Acceptance:
> real .ics imports; proposals avoid real events; approving a placement
> materializes a block visible on Calendar and in export.

### `/goal phase-3-disease-management`

Status: delivered as of 2026-07-22. M-A local logging, M-B schedules +
feasibility, the local rhythm-marker prerequisite, and M-C clinician context
are delivered via ADR-0024/0025/0026/0027. M-D..M-F remain separately gated.

> **Goal: complete medication tracking M-C using the delivered M-A/M-B and
> rhythm-context foundations so the app manages the disease, not just the
> schedule.**
> Context: `docs/medication-feature-plan.md` (authoritative; Medisafe-
> benchmarked, M-A local logging → M-B schedules/collision forecasts → M-C
> adherence + clinician export), `core/medication` timing, validation-plan
> scenario 19, ADR-0013 append-only pattern, and ADR-0026's delivered
> **context markers**: travel / illness / disruption / forced-schedule records
> (immutable, erasable) rendered on the actogram and listed as
> confounders in the association view. Then the §3.6 clinician report:
> actogram + drift + dose markers + adherence-vs-rhythm table as printable
> PDF/HTML export — redaction-checked, no diagnosis text. Invariants: no
> recommendations, no interaction checking (disclaimed in UI), labels never
> leave the local trust zone, timing facts in neutral ink. Acceptance: the
> Medications sample preview is retired; a fixed-clock regimen shows its
> collision forecast; the clinician export renders from real data.

### `/goal phase-4-local-assistant`

Status: delivered 2026-07-26 via ADR-0028. P4-a ships the desktop-local
loopback MCP endpoint (allowlisted read projections, `set_appearance` as the
ADR-0021 direct display action, propose-only mutations, no approve/apply
tool), P4-b ships `ask_zeitboard_facts` over medication timing facts and
context markers with the medical refusal moved to shared `core/agentpolicy`
so it is byte-identical on every surface, and P4-c ships the runbook voice
walkthrough plus `scripts/smoke-local-mcp.ps1`. Residual recorded in the ADR:
the endpoint runs whenever the app runs and is not behind an opt-in toggle.

Slice plan (2026-07-22): **P4-a** desktop-local agent endpoint — a
loopback-only MCP served by the desktop app itself (not the backend),
exposing allowlisted read projections (overview, rhythm summary, tasks,
medication timing facts, markers, appearance state) and two action
classes: propose-only mutations (reusing the existing action registry)
and the ADR-0021 direct display action (set preset / night rule). Every
projection passes the same redaction review as the M-E gate in
`medication-feature-plan.md`; labels stay local because the endpoint IS
local, but the tool results must still exclude raw records. **P4-b**
assistant answer scope over medication/marker facts (server-side, using
the same redacted context discipline; medical refusal byte-identical).
**P4-c** voice-path polish: runbook walkthrough of Claude Desktop voice →
local endpoint, call budgets, and a scripted smoke test. Acceptance
carries the prompt below; P4-a is the prerequisite for "local assistant"
as stated.

> **Goal: mature the assistant into the app's local agent: a desktop-local
> agent endpoint, richer answer scope, and voice-path polish.**
> Context: ADR-0006/0010/0012/0021, slice-8 rail, `zeitboard-mcp`.
> Implement the **desktop-local agent endpoint** deferred in ADR-0021: a
> local-only surface (Wails-bound or loopback MCP) exposing allowlisted
> reads (rhythm, tasks, medication/marker projections once explicitly designed,
> appearance state) and the
> direct display action (set appearance preset/night rule) plus propose-
> only mutations — so a voice client on the same machine can drive the app
> without the backend. Extend assistant answer scope to medication timing
> facts and markers (facts only; the medical refusal stays byte-identical).
> Keep redaction: local agent sees projections, not raw records; cloud
> paths unchanged. Acceptance: with the backend off, a local MCP client
> can read status and switch appearance; medical prompts still refuse;
> redaction tests extended to the new surface.

### `/goal phase-5-availability-portal`

Implementation-ready design: [`portal-design.md`](portal-design.md)
(projection firewall, hashed link tokens with uniform 410s, origin-
`visitor` proposals, requester secrets, messaging caps, threat-model v2
delta, slices P5-a..P5-d with a default-off exposure gate, and the
honesty budget derived from the measured 1.71 h median / 5.41 h P90
backtest — windows with a live now-state, no public confidence labels
until calibration is fixed). The four §9 owner decisions were resolved
2026-07-22: live dashboard yes (SSE + polling + meta-refresh, freshness
always visible), passcodes required on every link, request horizon
uncapped with a beyond-horizon warning, and calendar-inline
approve/decline that also materializes the ADR-0023 block.

> **Goal: the public-facing portal: share a link that shows when the user
> is likely awake, lets visitors ask, and lets them request a time — every
> request landing in the approval queue.**
> Context: `docs/future-relay-design.md` (input), `apps/trusted-web-
> prototype` (to be replaced), sharing spec §9.7/9.8, ADR-0016 queue,
> threat model. Server-side on the self-hosted instance: per-profile
> expiring, revocable, passcode-optional links serving a **projection-only
> availability page** (waking windows + confidence bands; civil-time
> primary; zero health records, zero names of medications/tasks ever).
> Visitor actions: (a) "ask" — canned availability answers computed from
> the projection, no LLM on the public path; (b) "request a time" —
> structured slot request (window + short message, length-capped,
> sanitized) that becomes a **proposal with origin `visitor`** in the
> existing queue, decided with one-use tokens like every proposal;
> (c) messaging — a thread per request, visible in-app; visitor sees
> status changes (pending/approved/declined) without seeing the calendar.
> Hard requirements: rate limits + proof-of-work or captcha-free
> throttling, link-token entropy + no enumeration, per-profile audit log,
> kill switch (revoke = 410), threat-model v2 + privacy.md update BEFORE
> exposure, CSP with zero third-party origins. Acceptance: a visitor with
> the link sees only allowlisted fields in every response body (asserted
> by test), a request round-trips to an in-app decision and a visitor-
> visible status, and revocation is immediate.

### `/goal phase-6-companion-and-notifications`

> **Goal: notification fan-out and the Android companion: portal requests
> and decisions reach the user wherever they are.**
> Context: roadmap slice 9, ADR-0015 sync model, phase-5 portal events.
> Implement self-hosted **web push** (operator VAPID keys, no vendor
> service) for: new visitor request, request decision recorded elsewhere,
> optional Medfriend-style missed-signal (off by default, its own sharing
> permission). Bring the Android companion onto enrollment + push/pull
> (reusing ADR-0015/0020 record kinds) with the same honest states, and
> register it as a push receiver. Notification content is projection-safe
> (no health data in notification text — "New time request from your
> link" only). Acceptance: with the desktop closed, a portal request
> raises a push on an enrolled device; notification bodies contain no
> restricted fields (tested); the companion syncs sleep + tasks
> round-trip.

---

Cross-phase invariants (every prompt inherits these): entirely
self-hostable, BYOK, no telemetry, no shipped keys; propose-only mutation
with human approval; civil time primary, uncertainty visible, typed
refusals; no medical advice/diagnosis/dosing/timing recommendations; US/NC
legal scope; visual-first with accessibility where it costs nothing;
private text (titles, labels, notes) never leaves the local/instance trust
zone; every new surface ships with its redaction test.
