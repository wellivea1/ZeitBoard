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

**Gap 1 — the estimate is unproven on real life** (roadmap slice 3). Every
promise the portal makes ("likely awake ~15:00–23:00") rests on estimator
accuracy on *real* logging behavior. Synthetic validation (ADR-0019) is
strong but cannot prove this. Real-history import + backtest is the
epistemic gate for everything public-facing and stays **Phase 1**.

**Gap 2 — "calendar app" is still a fixture.** The Calendar screen is a
synthetic five-day board; fixed events exist only as scheduler test inputs.
Real ICS/CalDAV import (read-only first), real fixed events in planning,
and write-back of approved placements (Phase 3c) are the distance between
"planner with proposals" and "calendar app."

**Gap 3 — "disease management" is planned, not built.** The
Medisafe-benchmarked medication plan (M-A..M-F) is unimplemented; the
Medications screen is the last full sample-preview surface. Disruption /
travel / illness / forced-schedule markers and the §3.6 clinician report
are deferred. These three together are what "disease management" means
here — rhythm + meds + markers + a report a clinician will actually read.

**Gap 4 — the assistant is desktop-chat only.** Voice rides the MCP client
(documented), but agent access to local device state (appearance, quick
logging) waits on the desktop-local agent endpoint deferred in ADR-0021.
"Local assistant" as stated needs that endpoint plus answer scope over
meds/markers once they exist.

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

**Sequencing logic.** Trust before reach: prove the estimate (P1), make the
calendar real so requests have something to land in (P2), build the health
substance (P3), mature the assistant over it (P4), then open the portal
(P5) and fan out notifications (P6). P3 and P4 can interleave; P5 hard-
depends on P1 (honest availability) and P2 (requests need real placement).

---

## Phase goals (`/goal` prompts)

### `/goal phase-1-ground-truth`

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

> **Goal: implement medication tracking M-A..M-C and rhythm markers so the
> app manages the disease, not just the schedule.**
> Context: `docs/medication-feature-plan.md` (authoritative; Medisafe-
> benchmarked, M-A local logging → M-B schedules/collision forecasts → M-C
> adherence + clinician export), `core/medication` timing, validation-plan
> scenario 19, ADR-0013 append-only pattern. Also add the deferred
> **markers**: travel / illness / disruption / forced-schedule records
> (append-only, erasable) rendered on the actogram and listed as
> confounders in the association view. Then the §3.6 clinician report:
> actogram + drift + dose markers + adherence-vs-rhythm table as printable
> PDF/HTML export — redaction-checked, no diagnosis text. Invariants: no
> recommendations, no interaction checking (disclaimed in UI), labels never
> leave the local trust zone, timing facts in neutral ink. Acceptance: the
> Medications sample preview is retired; a fixed-clock regimen shows its
> collision forecast; the clinician export renders from real data.

### `/goal phase-4-local-assistant`

> **Goal: mature the assistant into the app's local agent: a desktop-local
> agent endpoint, richer answer scope, and voice-path polish.**
> Context: ADR-0006/0010/0012/0021, slice-8 rail, `zeitboard-mcp`.
> Implement the **desktop-local agent endpoint** deferred in ADR-0021: a
> local-only surface (Wails-bound or loopback MCP) exposing allowlisted
> reads (rhythm, tasks, meds/markers once built, appearance state) and the
> direct display action (set appearance preset/night rule) plus propose-
> only mutations — so a voice client on the same machine can drive the app
> without the backend. Extend assistant answer scope to medication timing
> facts and markers (facts only; the medical refusal stays byte-identical).
> Keep redaction: local agent sees projections, not raw records; cloud
> paths unchanged. Acceptance: with the backend off, a local MCP client
> can read status and switch appearance; medical prompts still refuse;
> redaction tests extended to the new surface.

### `/goal phase-5-availability-portal`

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
