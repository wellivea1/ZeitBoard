# Medication tracking: comprehensive feature plan

> Implementation plan. M-A through M-C are delivered under ADR-0024 through
> ADR-0027; M-D..M-F remain gated.
> Extends `ui-ux-design.md` §9.6 and the
> roadmap's medication debt line into a full feature plan. Engineering notes
> only; not medical advice. The app records and displays — it never recommends
> a medication, dose, or timing.

## Delivery status (2026-07-22)

**M-A local logging, M-B schedules + feasibility, and M-C clinician context are
delivered.** The
desktop preserves the strict v1 logging contract and adds strict v2
schedule-capable medication, event, and export contracts;
revision-checked local definitions; immutable raw taken/skipped events with
correction chains; owner-entered `as_needed`, `fixed_clock`, and `cycling`
rules; neutral 14-day civil occurrence and sleep-collision forecasts; opt-in
at-most-once Windows desktop reminders; owner-initiated JSON export; and
distinct typed hard erasure for an event or a definition with byte-level
SQLite/WAL tests. The sample preview is removed.

Adding a medication still creates no schedule. A clock schedule requires the
owner to enter its IANA zone and civil times, and ZeitBoard never infers or
moves them. ADR-0026 now supplies local illness/travel/disruption/forced-
schedule confounder records and actogram annotations. ADR-0027 now delivers
explicit-record adherence summaries, medication dose/start markers, a
non-causal before/after view with named confounders, and the redaction-first
local clinician report as printable HTML.

Medical boundary evidence is intentionally conservative. The
[AASM intrinsic circadian-rhythm guideline](https://pmc.ncbi.nlm.nih.gov/articles/PMC4582061/)
uses disorder-, population-, and treatment-specific recommendations and rates
strategically timed melatonin for blind adults with N24SWD as weak evidence;
it does not justify app-generated individual timing advice. A
[MedISAFE-BP randomized trial](https://pmc.ncbi.nlm.nih.gov/articles/PMC6145760/)
found a small self-reported adherence improvement without a blood-pressure
difference, which does not establish dosing guidance or clinical benefit for
this product. The large
[REMIND randomized trial](https://pmc.ncbi.nlm.nih.gov/articles/PMC5470369/)
found no adherence improvement from its low-cost reminder devices. ZeitBoard
therefore records facts and qualifies derived context; it never recommends a
medication, dose, or time or claims that reminders improve outcomes.

## 1. Reference review: Medisafe

Medisafe is the highest-profile, clinically studied medication tracker
(RCT-evaluated, HIPAA-audited, ISO 27001). Its feature inventory is the
benchmark for "comprehensive":

| # | Medisafe capability | Notes |
|---|---|---|
| 1 | Medication list: name, form, strength, appearance | pill visuals aid recognition |
| 2 | Complex schedules: X/day, every-N-hours, weekdays, cycles (e.g. 21/7), PRN "as needed" | |
| 3 | Reminders: persistent, snooze, timezone-aware while traveling | |
| 4 | Adherence log: taken / skipped / snoozed per dose, streaks, weekly % | |
| 5 | Refill tracking: pill count-down, restock threshold alert | |
| 6 | Medfriend: a chosen person is notified after a missed dose | |
| 7 | Drug–drug interaction warnings (licensed database) | |
| 8 | 90+ health measurements (BP, glucose, weight…), Apple Health | |
| 9 | Doctor-appointment calendar | |
| 10 | Shareable adherence reports for clinicians | |
| 11 | Condition resource centers, med info leaflets | |
| 12 | Multi-profile (dependents, pets) | |

Sources: [Medisafe features](https://medisafeapp.com/features/),
[App Store listing](https://apps.apple.com/us/app/medisafe-medication-management/id573916946),
[Google Play listing](https://play.google.com/store/apps/details?id=com.medisafe.android.client),
[GoodRx reminder-app roundup](https://www.goodrx.com/healthcare-access/digital-health/medication-reminder-apps),
[RCT via PMC](https://pmc.ncbi.nlm.nih.gov/articles/PMC11026187/).

## 2. Disposition: what ZeitBoard adopts, adapts, rejects

ZeitBoard is not a general adherence app: it is a **single-user, self-hosted,
no-medical-advice planner for a free-running rhythm**. Each Medisafe
capability is adopted, *adapted to circadian reality*, or rejected with a
reason. The two right-hand columns are the ones this plan exists for.

| Medisafe capability | Disposition | Needs circadian tracking? | Needs agentic surface? |
|---|---|---|---|
| 1 Medication list | **Adopt** (label is private user text; form/strength optional labels) | – | read via opaque ids only |
| 2 Complex schedules | **Adapt**: `fixed_clock` for a user-entered clinician rule, `as_needed`, `cycling`; plus display-only wake-relative timing. No drug-specific schedule is built in. | **Yes** — the schedule kinds are only meaningful against the rhythm | schedule described in speakable form |
| 3 Reminders | **Adapt**: reminders fire at civil times, but ZeitBoard adds **feasibility awareness** — a fixed-clock reminder that will land inside predicted sleep is *flagged*, never moved | **Yes** — core differentiator (§3) | reminder-shift **proposals** only (existing `propose_reminder_shift` action) |
| 4 Adherence log | **Adopt**: taken/skipped raw evidence; any "late" classification is derived later from a user-entered schedule, never stored as a raw status | **Yes** for the rhythm-context column (§3) | speakable summary |
| 5 Refill tracking | **Adopt** (simple count + threshold; no pharmacy integration) | – | read + "propose refill task" via existing task proposals |
| 6 Medfriend | **Defer** to the sharing-transport slice: a missed-dose signal is a *sharing profile permission*, off by default, expiring, revocable | – | – |
| 7 Interaction warnings | **Reject.** A licensed interaction database is medical-advice territory and a liability the project's US/NC posture (ADR-0008) cannot carry. Honest UI copy: "This app does not check interactions — ask your pharmacist." | – | refusal script covers it |
| 8 Measurements | **Defer.** Sleep *is* the vital sign here; general measurement tracking dilutes scope. Revisit only with a concrete rhythm-relevant use (e.g. temperature). | – | – |
| 9 Appointment calendar | **Covered elsewhere**: appointments are fixed events (calendar import, roadmap slice 6) | – | – |
| 10 Clinician reports | **Adopt** as part of the §3.6 clinician export: doses on the actogram + adherence-vs-rhythm table | **Yes** — the report's value *is* the rhythm context | – |
| 11 Resource centers / leaflets | **Reject** (content curation ≈ implied advice; out of scope) | – | – |
| 12 Multi-profile | **Reject** (single-user system by design) | – | – |

## 3. Where circadian tracking changes the design

These are the aspects a normal tracker cannot do and ZeitBoard must — each
consumes the estimator and inherits its honesty rules (typed refusals,
confidence, no fabrication):

1. **Dual-clock regimens.** When the user enters a clinician-defined *fixed
   civil time*, the schedule anchor is `fixed_clock`; ZeitBoard does not infer
   a rule from a medication name or generalize how a drug is prescribed. The
   *display* is dual: "22:00 —
   currently 3 h before your predicted sleep onset, drifting +40 min/day."
   Wake-relative (`after wake`) display uses the existing
   `medication.ResolveRelativeTiming`; `ConfidenceUnknown` renders as "couldn't
   tie this dose to a wake anchor," never a guess.
2. **Reminder feasibility, not reminder rescheduling.** For a Non-24 user a
   fixed-clock dose periodically collides with predicted sleep. ZeitBoard
   computes the collision forecast ("this reminder falls inside your predicted
   sleep window for ~9 of the next 14 days") and *shows* it. It never moves a
   dose. If the user asks the assistant to shift a reminder, that is a
   `propose_reminder_shift` **proposal** through the approval queue —
   and even then it shifts the *reminder*, with the clinician-rule tag shown
   if one exists.
3. **Adherence in rhythm context.** A missed 22:00 dose while the estimator
   predicted sleep at 21:30 is a different fact than a missed dose mid-wake.
   The adherence view adds a neutral rhythm-context column ("predicted asleep
   at dose time") — observation, not excuse or judgment, in normal ink (the
   §9.6 no-red rule).
4. **Association without causality** (validation plan scenario 19). Starting
   a medication becomes an actogram/drift **marker**; the drift chart can show
   before/after slope segments *labeled as association*, with simultaneous
   confounders listed (schedule changes, travel, light exposure if logged).
   The copy never says "the medication changed your drift." M-C consumes the
   delivered local rhythm-context records from ADR-0026 and renders dose/start
   markers separately from those possible confounders.
5. **Timezone-aware reminders** (Medisafe #3) use an explicit schedule-owned
   IANA zone. Civil occurrences resolve to UTC instants with tested DST-gap and
   repeated-time behavior; the app does not silently follow device-zone travel
   or reinterpret the owner's rule.

## 4. Where agentic functionality is required (ADR-0006)

Every capability must be operable by an agent non-visually, propose-only,
through the same redaction gate proven for tasks (slice 8):

- **Redaction invariant: medication labels never leave the device-local /
  instance-local trust zone.** LLM context and MCP projections carry opaque
  `med_*` ids, schedule kinds, and timing only; the rail and local UI
  re-attach the private label for display (the exact pattern tested in
  `TestAssistantMessageSendsRedactedContextAndMapsProposals`). Trusted views
  continue to carry no medication data at all (ui-ux-design §9.7/9.8).
- **Read tools (MCP + assistant context):** speakable adherence summary
  ("two scheduled doses today; the evening one falls 1 h before predicted
  sleep"), collision forecast, next-dose query. All confidence-visible.
- **Propose tools:** `propose_reminder_shift` (exists since M2); add
  `propose_log_dose` (agent-assisted logging still lands as a pending
  confirmation — logging is a health record, so the human confirms it) and
  refill → ordinary task proposal ("place 'pick up refill' before Friday").
- **Refusals hold everywhere:** "when should I take X?" gets the existing
  medical refusal script verbatim, in chat, MCP, and any future skill. The
  assistant may state *facts* ("you logged it 3 h after wake the last five
  times") but never timing advice. Test the boundary explicitly.

## 5. Data model and sync sketch

Follows the established split: **definitions are mutable intentions, events
are append-only evidence.**

- `medication` (mutable, revision-synced like tasks, ADR-0020):
  `med_id`, `label` (private), optional `form`/`strength_label`,
  `schedule` (`kind: fixed_clock|as_needed|cycling`, civil times, cycle
  days-on/off, explicit IANA zone, reminder opt-in), optional
  `clinician_rule` (verbatim user-entered text, always attributed to the
  clinician in UI), `active` flag, and optional `started_at` plus explicit zone
  (drives the delivered descriptive association marker). Refill state remains
  future work.
- `medication_event` (append-only, correction-chained like sleep records,
  ADR-0013): `event_id`, `medication_id`, `dose_at` (ZonedInstant), `status`
  (`taken|skipped`), `scheduled` flag, optional private note; wake-relative
  fields are *computed*, never stored (they change when the estimate does).
- Reminder claims (local delivery state): opaque occurrence digest,
  medication ID, scheduled UTC instant, and claimed UTC instant. The unique,
  immutable claim is inserted before notification delivery and is excluded
  from export and sync.
- Contracts: M-A's strict v1 medication/event/export schemas remain unchanged.
  M-B adds schedule-capable v2 versions of `medication-set.schema.json`,
  `medication-event-set.schema.json`, and the local export envelope. M-C adds
  the strict v1 `clinical-chart-request` projection contract. M-D
  must add explicit sync-batch kinds and remote erasure tombstones before any
  medication payload leaves the device.
- Erasure/export: M-A delivers `ExportSleepData`-style contract export and the
  separate DELETE-typed local erasure flow (ADR-0014/0024).

## 6. Slice plan (each lands with tests; ADR when architecture changes)

| Slice | Scope | Depends on |
|---|---|---|
| **M-A Local logging — delivered (ADR-0024)** | contract + storage + real Medications screen (quick log / backdate / skipped / notes), wake-relative + before-sleep display via `core/medication`, honest no-estimate states; sample preview retired | nothing (core exists) |
| **M-B Schedules + feasibility — delivered (ADR-0025)** | `fixed_clock`/`as_needed`/`cycling` schedules, explicit-zone civil expansion, opt-in at-most-once desktop reminders, neutral collision forecast surface, clinician-rule attribution | M-A |
| **M-C Adherence + clinician export - delivered (ADR-0027)** | explicit-record adherence-vs-rhythm table, dose/start markers, robust observational association with the ADR-0026 confounder list, redacted preview, and standalone printable HTML/PDF path | M-A; marker records (delivered) |
| **M-D Sync** | definitions as revision records, events as append-only records, tombstone deletion | M-A (mirrors ADR-0020) |
| **M-E Agent surface** | redacted assistant/MCP context + `propose_log_dose`; refusal-boundary tests | M-A, slice-8 rail |
| **M-F Missed-dose sharing** | Medfriend-equivalent as an off-by-default sharing permission | sharing transport (deferred) |

Ordering rationale: M-A retired the last full "Sample preview" screen; M-B
completed the explicit schedule/collision part of §9.6; M-C adds the local
clinician-facing context. M-D/M-E remain separately gated because they create
new sync and agent projection boundaries.

## 7. Standing safety acceptance (every slice)

1. No recommendation path exists: no UI or agent output ever suggests a
   medication, dose, or time. Timing facts are neutral ink, never red.
2. Wherever a prompt surface exists, the medical refusal script is
   byte-identical across chat and MCP, and a medical prompt creates no proposal
   (already tested; extend to medication-specific prompts in M-E). The M-A/M-B
   record and schedule UI has no prompt or recommendation action.
3. Interaction checking is explicitly disclaimed in the UI, not silently
   absent.
4. Medication labels/notes: never in LLM context, MCP output, trusted views,
   or logs; asserted by redaction tests like the slice-8 title test.
5. Clinician rules are displayed verbatim and attributed; the app never
   paraphrases them into guidance.
