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

The 2026-07-27 architecture and performance hardening review is tracked in
[`architecture-performance-review-2026-07-27.md`](architecture-performance-review-2026-07-27.md).

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
  The desktop additionally serves its own loopback agent endpoint for
  backend-independent local control (ADR-0028); both share the medical-refusal
  policy in `core/agentpolicy`.

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
- The architecture/performance pass removed oversized-sync deadlock, mixed-log
  replay, quadratic backtest and scheduling paths, non-atomic pull pages,
  proposal N+1/unbounded reads, repeated snapshot/outbox scans, and session-only
  Android evidence. Remaining structural work is explicitly dispositioned in
  the review ledger rather than being presented as complete.

---

## Next slices (priority order)

The actionable near-term plan. Each slice is self-contained and lands with an
ADR when it changes architecture. The phase-level direction with a pasteable
`/goal` prompt per phase is [`phase-goals.md`](phase-goals.md); the portal
itself is designed in [`portal-design.md`](portal-design.md).

> **Priority correction, 2026-08-04.** An applicability/utility/automaticity
> review found the project had moved into platform maturity ahead of the loop
> it exists to close: fresh evidence still mostly arrives because the user
> typed it. Its verified findings and dispositions are in
> [`automaticity-review-2026-08-04.md`](automaticity-review-2026-08-04.md).
> **The next work is slice 9 (Android sync) and slice 11 (a real activity
> collector), not more portal or agent breadth.** Portal P5-c/P5-d and new
> assistant actions are paused — maintained and tested, not extended. Three
> claim-accuracy defects it surfaced are tracked as slice 12.
>
> *Progress, 2026-08-06.* Slice 9 is delivered (ADR-0032) and slice 11's
> collector exists (ADR-0031) but is not yet in a background service. Slice 13
> below adds the loop that makes any of it arrive without a screen open.

1. ~~**Close the control loop — approvals unification + sync robustness.**~~
   ✅ Delivered (ADR-0016): cross-device decisions via listed one-use tokens, a
   "Synced backend" approvals panel (absent when sync is off), and orphan
   synced corrections skipped instead of wedging the pull cursor. Remaining
   niceties (batch review, expiry surfacing) fold into later queue work.
2. ~~**Server-side erasure (tombstones).**~~ ✅ Delivered (ADR-0017): an
   authenticated erase endpoint hard-deletes synced payloads; tombstones
   carry the record id plus its non-sensitive original kind when known, so
   every device erases the correct record without naming conventions. No
   content is retained; the registry blocks resurrection by stale devices;
   the desktop enqueues erasures for pushed records at hard-delete time.
   Task erasure also records the logical task id, preventing a later unseen
   revision from resurrecting the deleted task.
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
   *Desktop-local agent endpoint delivered (ADR-0028):* the app serves its own
   loopback MCP (token + `Origin` rejection + an owner-only, inheritance-
   disabled descriptor ACL), exposing
   allowlisted read projections, `set_appearance` as the ADR-0021 direct
   display action, and propose-only mutations with **no approve/apply tool**,
   so a voice client works with the backend off. The medical refusal moved to
   shared `core/agentpolicy` and is byte-identical across chat, backend MCP,
   and the local endpoint.
9. ~~**Android companion sync.**~~ ✅ Delivered (ADR-0032): Health Connect
   sleep reaches the estimator without manual transcription. A revised source
   record supersedes through the append-only correction chain rather than
   arriving as a second episode; an episode whose offset contradicts the home
   zone is held and counted rather than labelled with a zone it disagrees with;
   the outbox is durable and idempotent at the source; and sync states stay
   distinguishable so an unreachable backend cannot hide behind a spinner.
   Verified end to end against a real daemon on an emulator. Travel records and
   pull remain follow-ups. *Original entry:* The Health Connect
   skeleton reads real sleep and stores it durably, but the companion has no
   sync client, so the device most likely to hold fresh wearable sleep cannot
   reach the estimator. Bring it onto the same enrollment + push/pull path
   (its ADR should reuse ADR-0015's model, ADR-0020's revision records, and
   ADR-0017's tombstones). This is the single highest-value change in the
   project: it connects the best existing passive source to the existing core.
9b. **Availability portal.** *P5-a delivered (ADR-0029):* a separate portal
    database that the public package cannot reach by import, an owner-side
    materializer narrowing the estimate to windows plus freshness, the public
    security middleware (CSP, per-source throttling, indistinguishable link
    failures, argon2id passcodes with per profile-and-source backoff,
    `Sec-Fetch-Site` mutation attestation), a no-JavaScript availability page
    that withholds a day-old estimate rather than showing it, and owner
    create/list/revoke/erase. Confidence labels are withheld because ADR-0022
    measured the buckets inverted. `portal.enabled` defaults false and no
    `/p/` route exists when it is off. *P5-b delivered (ADR-0030):* visitor
    time requests reach the owner's queue through a transactional outbox with
    a stable idempotency key, become proposals with origin `visitor` decided
    by the same one-use tokens, and approval must name an exact block inside
    the requested window; the decision returns to a visitor-visible status.
    A request stays honestly `queued` until the owner's queue confirms it, a
    decline carries no reason, and the requester secret travels only in a URL
    fragment. The owner decides in the desktop's Approvals screen on a
    surface separate from synced proposals, with a block picker bounded to
    the requested window and the approval disclosure above the buttons.
    **Next:** P5-c threads, P5-d the live layer and audit UI. Public exposure stays prohibited
    until the [`portal-design.md`](portal-design.md) §12 gate passes,
    including an independent review.
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
    *U-H delivered:* eight equal-weight destinations became five — `Home ·
    Plan · Rhythm · Log · Sharing` — with Data Sources and Settings in a
    separate utility group. Plan hosts Calendar, Tasks and Approvals as tabs
    and carries the pending count, so the queue is no longer a permanent
    destination that is empty most of the time; Log hosts sleep, medications
    and context markers. One shared tablist replaced what would have been
    three, every legacy hash still redirects to the right screen *and* tab,
    and the lint enforces the count so a ninth destination cannot arrive one
    merge at a time. **This closes `ui-refactor-plan.md`**: U-A through U-H
    are all delivered.

11. ~~**A real Windows activity collector.**~~ ✅ Delivered 2026-08-04 as P7-3.
    The old collector emitted one `"state": "startup"` observation and then
    blocked forever while being named `SafeCollector` in a package called
    `activity`. It now records bounded behavioural transitions with a time and
    how long the previous state lasted, from two narrow calls — time since last
    input, and whether the interactive desktop is locked — behind a platform
    interface so a Linux adapter stays possible. `privacy.md`'s commitment is
    unchanged and unrelaxed: the recorded shape has no field for an application,
    a window title, a document, or a keystroke, so widening it means changing
    the type and the commitment that constrains it. *This entry described the
    old placeholder for five days after it was replaced; corrected 2026-08-09.*

    Still open: the collector runs inside the desktop process, so it collects
    only while ZeitBoard is running (it survives closing the window, which hides
    to the tray, but not quitting). Desktop CPU, memory and startup baselines
    were not captured before it landed, so its cost is still described rather
    than measured.
12. ~~**Claim-accuracy defects.**~~ ✅ Delivered. *(b) and (c)* landed with the
    freshness policy and the honest-confidence pass: Home leads estimate quality
    with evidence age, the categorical badge sits behind a disclosure that says
    the buckets did not rank, and the shared `core/freshness` policy serves
    desktop, server, local agent and portal. *(a) delivered 2026-08-08:* the
    Sharing screen no longer claims link creation is unbuilt. It lists the
    owner's real links, creates them with a required passcode and expiry, and
    revokes or erases them, all against the owner's own instance. `off` (sync
    is not configured) and `unavailable` (the instance's portal is switched
    off) are now distinct answers rather than one shrug.

    Two defects surfaced while wiring it, both found by a live round trip and
    invisible to either side's unit tests. The desktop's shared HTTP client
    decodes with `DisallowUnknownFields`, and the response structs omitted
    `schema_version`, so **every** create and list failed as "invalid JSON".
    And the server's erase route revoked the link, cleared its audit and dropped
    its private label but left the profile row, so an "erased" link stayed in
    the owner's list, revoked and nameless — the handler's own comment claimed
    otherwise. `portal.Store.DeleteProfile` now removes it, and every dependent
    row cascades because the schema already declared it.

13. ~~**The recompute orchestrator.**~~ ✅ Delivered (ADR-0033): analysis is
    scheduled rather than performed inside whichever HTTP request happened to
    arrive. A burst of pushes coalesces into one run; a result reports when its
    own answer expires and the loop wakes for that instant; durability is by
    reconciliation against an input fingerprint rather than by a queue that can
    drop work; and an unchanged projection keeps the time it last changed, so a
    page's staleness warning finally measures the evidence rather than the
    housekeeping. This is what makes ADR-0031's freshness policy take effect: it
    decided withholding at materialization, and nothing was causing a
    materialization when the user recorded nothing — which is exactly when the
    rule applies. Verified live: twelve pushes → one run, an unrelated task push
    leaving the stamp unmoved, and erasure withdrawing the projection with
    nobody refreshing anything. The desktop is unchanged and still recomputes on
    screen load.

14. ~~**The 48-72 hour operational view.**~~ ✅ Delivered (ADR-0034): the
    question the product exists for, answered on the screen a person opens.
    A three-state timeline — awake, asleep, and *uncertain*, because the
    estimator's envelopes overlap and a crisp boundary would be hours wrong
    several times a month; office-hours overlap reported as reachable time and
    merely possible time, never added together; fixed events flagged when they
    fall inside predicted sleep; task placements confined to confidently awake
    stretches; and the whole view withheld rather than anchored to nothing when
    the evidence is stale. It replaces Overview's single "useful task window"
    tile rather than adding a ninth destination. *Office hours became configurable
    in slice 16.*

15. ~~**One-tap sleep logging.**~~ ✅ Delivered 2026-08-09. Recording a night
    cost a four-field form filled in by someone who had just woken up, which is
    the automaticity review's highest-value usability item and the dependency
    that held UI guideline finding 7. Home now carries `I am going to sleep` and
    `I woke up` directly under the current state.

    The rule that shapes it: **a tap is not an observation.** The first tap
    parks an onset in `local_sleep_pending` as a recorded intent, because
    appending at that moment would put a row in the append-only log whose end
    had not happened, and correcting it afterwards would leave a permanent
    record of a boundary nobody saw. The night is appended when the pair falls
    between 3 and 14 hours — `core/estimation`'s floor and `core/inference`'s
    ceiling, not new numbers — and the marked onset is under 20 hours old.

    Everything else returns a typed question and records nothing until it is
    answered: a nap or a mistap, a missed wake tap, an onset so old that "now"
    says nothing about when the person woke, or no marked onset at all. Where
    a prefill comes from the estimator it is labelled a prediction on the field
    itself. Decision logic lives in `core/quicklog` so a companion surface
    answers identically, and `DeleteAllSleepData` takes the unfinished sleep
    with it — otherwise erasure leaves an onset that writes a fresh row on the
    next tap.

    One defect found in the preview rather than by reasoning: the confirm form's
    inputs were uncontrolled, so when one question replaced another React reused
    the element and kept the previous question's time while the app's own
    suggestion said otherwise. The fields are controlled now, and a regression
    test pins it.

16. ~~**Configurable reaching hours.**~~ ✅ Delivered 2026-08-09, closing the
    residual named in slice 14. The 48-72 hour view intersected predicted waking
    time with Monday to Friday, 09:00 to 17:00, in the user's own zone, with no
    way to say otherwise — and every "reachable for three hours" figure drawn
    from it inherited the error. A great many people with a drifting rhythm need
    a pharmacy open until eight, a clinic that runs Tuesday and Thursday, or
    family six time zones away.

    Settings now carries the schedule: whose hours they are, when they open and
    close, which days, and **their** zone rather than the user's. It can be
    switched off entirely, which reports nothing rather than a working week
    nobody chose. The outlook prints a sentence generated from the setting and
    worded as something the person recorded, not a fact about the world.

    Three things had to change in `core/outlook` to make the setting expressible.
    A close at or before the open now runs to the following civil day, so an
    overnight desk is one stretch instead of a silently dropped window, and equal
    times mean a service that never shuts. The day walk starts a civil day early,
    so a window that opened last night and is still open is reported — the most
    useful thing this view can say. And an `OfficeHours` with no open days now
    means *closed*: it used to fall back to Monday-to-Friday, which turned
    "switched off" back into the assumption being removed. That last one was
    found by a test, not by reading the code.

    The desktop's durable settings-file machinery — staged write, backup,
    validate-or-discard — was extracted from the appearance file rather than
    copied, and appearance moved onto it unchanged.

    Not done: one schedule at a time, and the same hours on every selected day.
    Several named contacts, and per-day hours, are a larger change to what
    `core/outlook` returns and to what Home draws.

17. ~~**Local file protection, and an honest at-rest claim.**~~ ✅ Delivered
    2026-08-10 (ADR-0035). `privacy.md` said in bold that at-rest encryption was
    required *locally and on the instance*. The instance half was true and
    tested. The local half was never true: the SQLite database holding every
    recorded sleep is not encrypted. Worse, the weaker fallback the document
    leaned on — file permissions restricted to the owner — was not true either,
    because every private file was created with an `0o600` mode argument, and on
    Windows that sets the read-only attribute and leaves the inherited DACL
    alone. Measured before the fix, the real data directory granted access to
    five trustees including `SYSTEM` and `BUILTIN\Administrators`.

    The project had already learned this once: ADR-0028 says the descriptor's
    "restrictive-permissions claim has to be enforced with a real DACL or not
    claimed at all". That fix reached one file and nothing else — not the bearer
    token for the user's own server, not the settings, not the exports, not the
    database.

    `core/platform/privatefile` now applies a protected single-grant DACL (the
    file mode elsewhere) to the database and its write-ahead log and
    shared-memory companions, the data directory, the token, the sync
    configuration, the settings files and every staged export. It also exposes
    `Describe`, which reads the permission back, so the tests assert what the
    operating system reports rather than that a call returned nil — and one test
    asserts `Describe` can report an unprotected file as unprotected, so the
    rest cannot pass vacuously.

    Settings → Local data now reports which files were checked and states that
    they are restricted **and not encrypted**, in those words. `privacy.md` and
    `threat-model.md` say the same, and name the residual as unmitigated rather
    than covered.

    Whole-database encryption is not shipped and ADR-0035 records why:
    `modernc.org/sqlite` is CGo-free by choice and exposes no VFS or serialize
    hook to put a cipher behind, SQLCipher requires CGO, decrypt-to-a-working-
    file leaves plaintext on disk for as long as a tray app runs, and column
    encryption that leaves `start_at` queryable protects nothing for a product
    whose health data *is* the timing.

**Small debts (fold into adjacent slices):** finish ordered local/server migrations; ~~
→ domain decoder (now duplicated across desktop storage, server readmodel, and
the sync validator);~~ establish one versioned assistant action registry; extract
context-aware desktop feature services at behavior boundaries; add
repository-backed long-history pages; prefer a CA-cert path over localhost
skip-verify for the
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
- Local DB encryption at rest. *Partly addressed 2026-08-10 (slice 17,
  ADR-0035):* every private local file — the database, its write-ahead log, the
  backend token, the settings files and exports — now carries a real owner-only
  DACL that is read back and asserted, and privacy.md no longer claims the local
  store is encrypted, because it is not. Whole-database encryption remains open
  and is blocked on the CGo-free driver offering no VFS or serialize hook;
  SQLCipher would require CGO.
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
