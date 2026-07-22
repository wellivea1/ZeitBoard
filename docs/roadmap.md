# Roadmap

ZeitBoard is a planner for free-running sleep-wake rhythms: a **connected,
entirely self-hostable, bring-your-own-key** system (ADR-0007/0008) with a
visual-first desktop app, a self-hosted Go backend, and an agent-accessible
capability layer. Every phase keeps the same guarantees: civil time stays
primary, uncertainty stays visible, automation never applies itself silently,
and the app never gives medical advice or claims exact circadian phase/DLMO.

Build-ready UI for the feature phases lives in
[`ui-ux-feature-specs.md`](ui-ux-feature-specs.md); the master product design is
[`ui-ux-design.md`](ui-ux-design.md); the spec/implementation map is
[`specification-alignment.md`](specification-alignment.md); enforced desktop UI
boundaries are in [`frontend-architecture.md`](frontend-architecture.md).
Architecture decisions are ADRs under [`decisions/`](decisions/); per-milestone
detail lives there rather than being repeated here.

---

## Where things stand (delivered)

**Desktop (data + estimate)**
- Manual sleep entry with immutable observations, append-only corrections
  (edit/suppress), provenance history, and honest empty/refusal states
  (ADR-0013). Contract-shaped local records (sync-ready by construction).
- Overview, Rhythm (double-plot actogram + Theil-Sen drift + widening
  forecast), and the proposals queue are all computed by the core engine from
  the user's real local data — no synthetic data outside labeled sample mode.
- Export (v1 contract-shaped JSON) and hard erasure (single entry or all),
  distinct from append-only suppress (ADR-0014).
- Opt-in, off-by-default backend sync: enrollment, push/pull with dedupe,
  TLS-verified, token stored outside config; Overview/Rhythm can render the
  server's estimate, labeled distinctly, with local fallback (ADR-0015).
- Unified approvals: the desktop lists the backend's assistant/agent proposals
  and decides them via the one-use token (any enrolled device may decide;
  audited); orphan synced corrections are skipped so one bad record can never
  wedge the pull cursor (ADR-0016).
- End-to-end erasure: local hard-deletes propagate to the self-hosted instance
  (payloads hard-deleted server-side) and to every device via id-only
  tombstones in the pull stream; erased records can never be re-pushed
  (ADR-0017).
- User-owned tasks: contract-shaped task CRUD, a real Tasks screen, the
  scheduler planning only stored open tasks (ADR-0018), and cross-device task
  sync as immutable revision records with erasure-grade deletion (ADR-0020).
- Medication M-A/M-B: private local definitions, append-only taken/skipped
  events and correction chains, explicit user-authored schedules, neutral
  observed/predicted collision context, opt-in claim-first desktop reminders,
  versioned local export, typed hard erasure, and a dense real-data Medications
  workspace (ADR-0024/0025). No schedule, reminder time, interaction check, or
  treatment recommendation is inferred.
- The Rhythm Sources tab and Data Sources run on real local state (refusals,
  correction history, per-source composition, sync status); synthetic previews
  are confined to the labeled browser fixture mode.
- Appearance manager with Auto plus Paper, Dark, Pitch black, Amber, and High
  contrast presets; reduced stimulation composes with each preset. Contrast,
  simulated dark-amber contrast, and blue-channel token assertions are
  automated (ADR-0005, UI slices U-A..U-C).

**Backend (`apps/server`, self-hosted)**
- M1 sync: TLS, per-device bearer tokens (hashes stored), strict v1
  validation, idempotent append-only log, AES-256-GCM at rest (ADR-0009).
- M2 assistant: BYOK multi-provider LLM (OpenCode Zen / OpenRouter / OpenAI /
  Anthropic), redacted field-by-field context, strict-JSON allowlisted actions
  resolved by the server into *pending* proposals, one-use signed approval
  tokens, encrypted proposal/audit storage, device revocation (ADR-0010).
- M3 server-side estimation: effective sessions replayed from synced records,
  core-engine overview/rhythm/accuracy projections, typed refusals (ADR-0011).
- M4 local MCP connector: stateless stdio adapter, allowlisted read +
  propose-only tools, call budgets, **no approve/apply tool** (ADR-0012).

**Measurement**
- `estimation.Backtest` (walk-forward validation) scores point error,
  forecast-window hit-rate, and per-confidence calibration. Synthetic findings:
  clean linear rhythms recover at ~0h error; a 3h phase jump degrades point
  error to ~3h while confidence correctly drops; hit-rate stays 1.0 because the
  windows are honestly wide — so **point error is the sharper quality signal**
  and window width was a tightening candidate.
- Combined real-history validation is recorded in ADR-0022 and
  `verification.md`: 952 principal episodes produce 809 evaluations and 136
  typed refusals, with baseline median error 1.71h, P90 error 5.41h, 0.78
  window hit-rate, and 0.856 evaluable coverage. Blanket tightening was
  rejected: a 25% uncertainty reduction saved 1.40h of mean window width but
  lost 6 percentage points of hit-rate, with no point-error benefit.

---

## Next slices (priority order)

The actionable near-term plan. Each slice is self-contained and lands with an
ADR when it changes architecture. The phase-level direction (through the
public availability portal) with a pasteable `/goal` prompt per phase is
[`phase-goals.md`](phase-goals.md); slices below map onto its phases 1-3.

1. ~~**Close the control loop — approvals unification + sync robustness.**~~
   ✅ Delivered (ADR-0016): cross-device decisions via listed one-use tokens, a
   "Synced backend" approvals panel (absent when sync is off), and orphan
   synced corrections skipped instead of wedging the pull cursor. Remaining
   niceties (batch review, expiry surfacing) fold into later queue work.
2. ~~**Server-side erasure (tombstones).**~~ ✅ Delivered (ADR-0017): an
   authenticated erase endpoint hard-deletes synced payloads; tombstones
   (record-id only) flow through the pull stream so every device erases its
   copy; a tombstone registry makes re-pushing an erased id a silent no-op;
   the desktop enqueues erasures for pushed records at hard-delete time.
   Threat model + privacy updated. Residual: never-syncing devices retain
   their copy until they pull.
3. ~~**Validate the estimator on real history.**~~ ✅ Delivered. The owner's
   2021–2023 sleep history is represented in the v1 import format and measured
   with the walk-forward backtest. The result remains the standing gate:
   window-width, misfit-signal, and phase-dependent-duration changes require a
   positive measured delta before shipping.
   *Interim delivered (ADR-0019):* `core/simulate` — the validation plan's
   seeded synthetic generator (latent truth retained) plus a 12-scenario
   suite running the real estimator: drift recovered within ~1 min/cycle on
   every linearly-describable pattern (incl. naps, fragmentation,
   deprivation, forced wake, travel, DST); the change-point scenario (S6)
   degrades honestly with self-reported low confidence, now locked by tests.
   *Real-history path delivered (ADR-0022):* strict v1 JSON/CSV import,
   Data Sources preview/commit UI, owner-assisted Fitbit/transcription tools,
   a source-checked and date-complete chart ledger, overlap-calibrated chart
   uncertainty, an ignored validation database proving 953 benchmark
   observations import cleanly with chart/Fitbit overlaps held out, and a
   committed baseline/candidate backtest.
   Blanket window tightening is rejected by measured delta. The confidence
   result justifies a separately measured confidence-window calibration or
   misfit candidate, not a speculative model change.
4. ~~**Make tasks real.**~~ ✅ Delivered (ADR-0018): user-owned tasks with a v1
   contract (`task-set`), local CRUD persistence, a real Tasks screen (add /
   done / delete, honest empty and read-only states), and the scheduler now
   plans only stored open tasks — no fabricated proposals, honest
   `estimate_unavailable` for real tasks without an estimate. *Task sync
   delivered* (ADR-0020): edits travel as immutable revision records over the
   append-only log with last-writer-wins application, and task deletion
   erases all pushed revisions via ADR-0017 tombstones. Approved local
   placement materialization is now delivered by ADR-0023; external-provider
   write-back remains separately gated.
5. ~~**Replace the remaining fixture UI.**~~ ✅ Delivered: in local/synced mode
   the Rhythm "Sources" tab now shows the estimator's real refusal, the real
   correction history (append-only inspector; the dead fixture undo button is
   gone), and the real per-source composition with corrected/suppressed
   counts; Data Sources gained a real server-sync source row. The synthetic
   conflict/correction/refusal previews survive only in the browser-preview
   fixture mode, which is labeled "Sample data". A real *conflict* list still
   needs an engine-surfaced overlap DTO — deferred until multiple sources
   exist (slice 7), rather than faking overlap logic in the chart layer.
6. ~~**Calendar import and local placement materialization.**~~ Delivered
   (ADR-0023): device-side bounded ICS and read-only CalDAV snapshots, a strict
   event-set contract, recurrence expansion with DST handling, atomic local
   persistence and revocable source erasure, real fixed events in the
   scheduler, and a dense Calendar board that keeps private text local.
   Approval records bind transactionally to the exact task revision,
   sleep-data snapshot, and text-free busy-event snapshot. Approval creates
   only an app-owned local block; rejection writes no event; undo removes only
   that block; app-owned placements export as RFC 5545 ICS. OAuth providers
   and remote write-back remain future, separately permissioned work.

**Disease-management track.** ~~M-A local medication logging~~, ~~M-B
user-authored schedules + feasibility~~, ~~local rhythm context markers~~, and
~~M-C adherence + clinician context export~~ are delivered
(ADR-0024/0025/0026/0027),
including byte-verified erasure distinct from append-only exclusion, explicit
civil/DST semantics, neutral sleep-collision forecasts, and opt-in at-most-once
desktop reminders. Context markers for illness, travel, disruption, and forced
schedules are immutable, erasable, local-only chart annotations and explicit
possible confounders beside M-C's descriptive association. M-C adds a 6 PM/noon
clinical-day chart, selected-range drift, dose/start markers, explicit-record
adherence, and confirmed redaction-first printable HTML. **Next:** M-D sync and
M-E's separately reviewed local agent projection.

7. **Takeout / "My Activity" import + activity→sleep inference.** File import
   (no Google API exists for My Activity) feeding the deferred probabilistic
   inference; emits low-confidence `inferred` episodes only, validated by the
   backtest before it can influence the estimate.
8. ~~**Assistant desktop UI (Phase 4 front end).**~~ ✅ Delivered: the §4
   assistant rail (status-strip toggle, Chat/Queue tabs, action cards wired to
   the same one-use-token queue decisions, Enter/Shift+Enter composer, typing
   indicator, disclaimer) over the M2 propose-only endpoint. The desktop sends
   a redacted context — task ids and bounds, never titles; titles are
   re-attached locally for display — and tests assert the redaction, the
   medical-refusal passthrough, and that the flow touches no endpoint that
   could apply a change. Provider disclosure comes from the backend
   (`Connected: <provider>` / `Offline`). The Claude-Desktop-over-MCP voice
   path is documented in the self-hosting runbook. Cloud skill packaging and
   any reviewer-gated auto-apply stay future and separately gated.
9. **Android companion sync.** The Health Connect skeleton exists but the
   companion has no sync client; bring it onto the same enrollment + push/pull
   path (its ADR should reuse ADR-0015's model).
10. **UI refactor + theme manager** (tracked in
    [`ui-refactor-plan.md`](ui-refactor-plan.md)). *U-A..U-C delivered:*
    the structural follow-up replaced Overview's metric-card grid with one
    status surface, a source-matched cycle strip, compact fact/confidence rows,
    conditional attention, and the trust boundary; Rhythm gives the actogram
    full-width visual priority. Sage is limited to action/awake semantics,
    sleep visuals use the asleep blue family, and the appearance manager ships
    Paper, Dark, Pitch black, **Amber glasses mode**, and High contrast plus
    Auto. Contrast regression covers every preset, including Amber's
    through-lens (≥7:1) and no-blue-channel assertions. Route modules, shared
    visuals, data adapters, and UI lint boundaries are documented and enforced.
    *U-D delivered (ADR-0021):* opt-in rhythm-linked night mode — a chosen
    night preset engages N hours before the *predicted* sleep onset and
    releases at predicted wake, honest civil-time fallback when the
    estimator refuses; display actions are direct local actions, never
    queue-gated. Agent-driven switching deferred until a desktop-local
    agent endpoint exists (recorded in the ADR). *U-E delivered*
    (ui-refactor-plan §7): a hover time probe on every chronological
    surface — hairline + tabular-numeral chip showing the exact civil time
    under the cursor (double-plot day resolution, `predicted` qualifiers,
    structured row dates in the local DTO rather than parsed labels). The
    server/MCP projection remains a separate default-deny allowlist and does
    not expose raw local zone identifiers. *U-F delivered:* Data Sources now
    leads with a dense provenance ledger, Sharing uses an honest default-deny
    capability/template ledger rather than fake active profile cards, and the
    Android companion replaced its generic rounded section `Panel` with ruled
    sections while retaining minimum touch targets. Desktop compact navigation
    no longer exposes its internal horizontal scrollbar, and Android's 680 dp
    large-width content cap is now actually enforced. *U-G delivered:* Tasks
    and Approvals now share one ruled proposal-queue language, expose confidence
    as text plus pattern, keep no-safe-window context in the same workflow, and
    use compact mobile actions; the phone appearance picker is a two-column
    selector and icon-only navigation retains explicit accessible names.

**Small debts (fold into adjacent slices):** consolidate the correction-record
→ domain decoder (now duplicated across desktop storage, server readmodel, and
the sync validator); prefer a CA-cert path over localhost skip-verify for the
MCP client; medication tracking M-A..M-C is delivered and M-D..M-F remain specified
([`medication-feature-plan.md`](medication-feature-plan.md), slices M-A..M-F:
benchmarked against Medisafe; fixed-clock regimens + wake-relative display,
reminder collision forecasts, adherence-in-rhythm-context, association-not-
causality markers, revision-synced definitions + append-only dose events,
redacted agent surface; interaction checking explicitly rejected);
periodic real screen-reader/TalkBack walkthroughs.

---

## Phase 2 — Local usability + the rhythm visualizer

Largely delivered (see "Where things stand"). Remaining in scope:

- Import adapters beyond sleep observation-set JSON/CSV (the local sleep path
  is hardened by ADR-0022; Takeout/activity import remains slice 7).
- Source-specific missingness remains open. Manual forced-schedule, travel,
  illness, and disruption markers are delivered in ADR-0026.
- Correction *undo* as a one-click affordance (the Sources tab now shows the
  real correction history and diff; reversal today means appending another
  correction in Data Sources).
- Onboarding beyond the empty state; localization readiness.
- Local DB encryption at rest + OS credential storage for the desktop store
  (the token file is 0600; the SQLite store itself is not yet encrypted —
  privacy.md requires it).
- Clinical-chart refinements beyond delivered M-C: reserved 48-hour clinical
  orientation, direct PNG/PDF generation, and a blank clinical sleep-log
  template. The shipped 24-hour report already prints to PDF from standalone
  local HTML; refinements must preserve the same redaction and no-advice gate.

Exit criteria: a fatigued user can read current state, recent drift, and the
next predicted windows in seconds; the app is keyboard-operable and meets WCAG
2.2 AA with chart text equivalents wherever they don't compromise the visuals;
usability research (synthetic/participant-controlled data) shows no one reads
an estimate as exact.

---

## Phase 3 — Interoperability: calendars, tasks, and the approval gate

**Delivered:** the desktop engine-backed proposal queue with explanation codes
and honest unplaced reasons; backend-persisted assistant/agent proposals,
one-use signed approval tokens, cross-device decisions, audit trail, real local
tasks, immutable task revision sync with erasure-grade deletion, read-only
calendar import, local approved-placement materialization, persistent local
decision history/undo, and app-owned ICS export.
**Not yet delivered:** external-provider calendar write-back, batch review,
unified local/backend presentation, and richer proposal expiry handling.

**3a. Read-only calendar import - delivered locally (ADR-0023).** ICS and
CalDAV adapters run device-side; imported events are immutable inputs whose
text never reaches trusted views, sync envelopes, or LLM context.

**3b. Approval queue completion** — the desktop already approves backend
assistant/agent proposals through the decision endpoint. Local scheduler
decisions now persist with visible history and per-item undo. Remaining work is
one combined presentation across local and backend origins, batch review, and
surfaced expiry.

**3c. External calendar write-back - last, and separately gated.** ADR-0023
delivers the ownership-safe local precursor: approval materializes a ZeitBoard
block and an owner can export those blocks as ICS. Writing into an external
provider remains off. It requires least-privilege scope, explicit per-calendar
consent, conflict semantics, and a dedicated security review before enablement.

Exit criteria: no code path applies a calendar change without a recorded
approval; import is revocable and least-privilege; write-back is off by
default and passes its security review before enablement.

---

## Phase 4 — Conversational assistant

**Delivered (backend):** the BYOK multi-provider assistant service — redacted
context, strict-JSON allowlisted actions, server-resolved *pending* proposals,
medical refusal, provider disclosure (ADR-0010) — and the agent-accessible MCP
interface with propose-only tools and no approve/apply path (ADR-0006/0012).

**Delivered (desktop):** the assistant rail, provider/offline disclosure,
transcript, refusal handling, redacted task context, and inline action cards
wired to the same one-use-token backend decisions.

**Remaining (product):**
- Broader usability validation of the assistant and queue under fatigue;
  batch review and surfaced proposal history remain Phase 3 queue work.
- Voice: ZeitBoard ships no speech stack; the supported path is an MCP-capable
  client with voice (documented in the self-hosting runbook).
- Cloud skill packaging (Claude/ChatGPT) — additive, opt-in, needs its own
  privacy/threat review. Reviewer-gated auto-apply remains future and is never
  the default gate.

Exit criteria: the assistant cannot mutate the schedule except by creating
approval-queue proposals; it refuses medical questions with a consistent,
non-alarming script; context sent to the user's provider is minimized and
redacted; the active provider is always disclosed; every feature is operable
non-visually by an agent (read state + propose actions) through the
allowlisted, redacted capability layer.

---

## Cross-cutting tracks (every phase)

- **Uncertainty system:** ranges not points; ordinal confidence;
  non-color-only encodings — applied to every new chart, proposal, and
  assistant answer. The backtest harness is the standing accuracy gate: model
  changes are justified by measured deltas, and new inference sources (device
  activity, Takeout) must pass it before influencing the estimate.
- **Accessibility — visual-first, accessible where reasonable:** never
  sacrifice the visuals; add accessible names, non-color-only cues, keyboard
  operation, WCAG 2.2 AA, reduced stimulation, 44px targets, and chart text
  equivalents wherever they don't compromise aesthetics or functionality. See
  `accessibility.md`.
- **Agent-accessible interface:** every feature exposes a non-visual,
  agent-operable surface — structured readable state + allowlisted
  *propose-only* actions through the approval gate. The primary non-visual
  path is an agent + live voice (ADR-0006), not a transcription of the charts.
  A standing design constraint on every new feature, not a later pass.
- **Privacy & threat model:** connected, self-hosted, BYOK (ADR-0007/0008):
  the user's data syncs to *their own* instance; the project runs no service,
  ships no keys, and collects no telemetry; legal scope US/NC. Implementation
  status per surface is tracked in `threat-model.md`; every new data source,
  endpoint, or external surface updates `privacy.md`/`threat-model.md` in the
  same change.
- **Contracts:** new surfaces extend the versioned schemas with deterministic
  fixtures (and an ADR) rather than inventing UI-only data — every schema in
  `contracts/v1` must have a registered, validated fixture.

---

## Non-goals

No phase includes: exact circadian phase/DLMO claims; autonomous health
recommendations; the assistant, scheduler, or any agent applying changes
without a recorded human approval; hidden background collection; advertising;
data brokerage; or a project-operated hosted service holding user data. Any of
these would require a new product scope and user-consent design.
