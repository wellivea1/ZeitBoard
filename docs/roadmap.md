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
[`specification-alignment.md`](specification-alignment.md). Architecture
decisions are ADRs under [`decisions/`](decisions/); per-milestone detail lives
there rather than being repeated here.

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
- Theming (Auto/Light/Dark), reduced stimulation, WCAG 2.2 AA contrast pass
  with a regression test (ADR-0005).

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
  and window width is a tightening candidate. Not yet run on real history.

---

## Next slices (priority order)

The actionable near-term plan. Each slice is self-contained and lands with an
ADR when it changes architecture.

1. **Close the control loop — approvals unification + sync robustness.**
   The desktop's in-session approval queue and the backend's persisted
   proposals are still two disconnected worlds. When sync is on, the desktop
   lists backend proposals and approves/rejects them via the one-use-token
   decision endpoint (audited); the in-session queue remains the offline path.
   Include the known pull-robustness fix: one unresolvable synced correction
   (missing target) must be skipped/quarantined, not allowed to wedge the pull
   cursor forever.
2. **Server-side erasure (tombstones).** Local hard-delete does not yet
   propagate; the sync log is append-only by design. Add an authenticated
   erasure endpoint + tombstone semantics so ADR-0014's erasure right extends
   to the synced instance, with a threat-model update in the same change.
3. **Validate the estimator on real history.** Transcribe the owner's real
   2021–2023 sleep charts into the v1 import format (manual/assisted — no
   handwriting-OCR promises), import locally, and run the backtest. The
   results gate estimator work: window-width tightening, an explicit
   linear-misfit signal, and phase-dependent sleep duration are pursued only
   as the numbers justify, each justified by a backtest delta.
4. **Make tasks real.** Flexible tasks are currently hardcoded planner
   fixtures (desktop `taskRows`, `localPlannerTasks`) — yet the approval queue,
   calendar phase, and assistant all assume user-owned tasks exist to move.
   Task CRUD + local persistence + sync (contract + ADR), replacing the
   hardcoded task list. This is a prerequisite for Phase 3/4 delivering real
   value, and it lets assistant/agent proposals target real tasks.
5. **Replace the remaining fixture UI.** The Rhythm "Sources" tab still renders
   synthetic conflict/correction-preview/refusal fixtures next to real data;
   Data Sources shows synthetic source states. Drive them from real local
   state (correction history exists; source status exists for sync) or clearly
   label and demote what has no real backing yet.
6. **Calendar import (Phase 3a) — after an explicit placement ADR.** The
   original scoping predates the backend; decide *where adapters live*
   (device-side like Health Connect vs server-side on the instance), which
   protocols first (ICS/CalDAV before Google OAuth verification burdens), and
   how imported-event text is kept out of projections. Then implement
   read-only import per the re-scoped ADR.
7. **Takeout / "My Activity" import + activity→sleep inference.** File import
   (no Google API exists for My Activity) feeding the deferred probabilistic
   inference; emits low-confidence `inferred` episodes only, validated by the
   backtest before it can influence the estimate.
8. **Assistant desktop UI (Phase 4 front end).** The backend and safety
   architecture are delivered (M2/M4); build the §4 chat surface over the same
   propose-only endpoints, with provider disclosure. Voice remains the agent
   client's job (ADR-0006); document the Claude-Desktop-over-MCP voice path in
   the runbook. Cloud skill packaging and any reviewer-gated auto-apply stay
   future and separately gated.
9. **Android companion sync.** The Health Connect skeleton exists but the
   companion has no sync client; bring it onto the same enrollment + push/pull
   path (its ADR should reuse ADR-0015's model).

**Small debts (fold into adjacent slices):** consolidate the correction-record
→ domain decoder (now duplicated across desktop storage, server readmodel, and
the sync validator); prefer a CA-cert path over localhost skip-verify for the
MCP client; medication logging should support fixed-clock-time regimens (how
tasimelteon/melatonin are actually prescribed) alongside wake-relative display;
periodic real screen-reader/TalkBack walkthroughs.

---

## Phase 2 — Local usability + the rhythm visualizer

Largely delivered (see "Where things stand"). Remaining in scope:

- Import hardening for external files (size/shape limits exist on sync; the
  local import path arrives with slices 3 and 7).
- Source-specific missingness and forced-schedule/travel disruption markers.
- Correction diff/undo backed by real history end-to-end (UI affordances
  exist; the Sources tab still shows fixture previews — slice 5).
- Onboarding beyond the empty state; localization readiness.
- Local DB encryption at rest + OS credential storage for the desktop store
  (the token file is 0600; the SQLite store itself is not yet encrypted —
  privacy.md requires it).
- **Automatic clinical charting + clinician export** (§3.6): the longitudinal
  clinical actogram with annotations and a printable, redaction-controlled
  PDF — replacing hand-kept sleep logs. Recording only; never a treatment
  recommendation. Depends on slices 3 (real history) and import hardening.

Exit criteria: a fatigued user can read current state, recent drift, and the
next predicted windows in seconds; the app is keyboard-operable and meets WCAG
2.2 AA with chart text equivalents wherever they don't compromise the visuals;
usability research (synthetic/participant-controlled data) shows no one reads
an estimate as exact.

---

## Phase 3 — Interoperability: calendars, tasks, and the approval gate

**Delivered:** the approval-gate *mechanics* — the desktop engine-backed
proposal queue with explanation codes and honest unplaced reasons; the
backend's persisted pending proposals, one-use signed approval tokens, and
audit trail. **Not yet delivered:** unification (slice 1), real tasks
(slice 4), calendar import (slice 6), and write-back.

**3a. Read-only calendar import** — re-scope via ADR first (adapter placement,
protocol order, OAuth verification realities); imported events are immutable
inputs whose text never reaches trusted views or LLM context.

**3b. Approval queue completion** — one queue: desktop approves backend
proposals (assistant/agent/scheduler origins) through the decision endpoint;
batch review, expiry of stale proposals, decision history surfaced.

**3c. Two-way calendar write-back — last, and separately gated.** Approved
changes to app-owned flexible items may be written back via a least-privilege
write scope. Gated behind the unified queue, explicit per-calendar write
consent, and a dedicated security review + threat-model update before
enablement. (The former "relay" review reference is superseded; the relay
design survives only as input to trusted-view sharing.)

Exit criteria: no code path applies a calendar change without a recorded
approval; import is revocable and least-privilege; write-back is off by
default and passes its security review before enablement.

---

## Phase 4 — Conversational assistant

**Delivered (backend):** the BYOK multi-provider assistant service — redacted
context, strict-JSON allowlisted actions, server-resolved *pending* proposals,
medical refusal, provider disclosure (ADR-0010) — and the agent-accessible MCP
interface with propose-only tools and no approve/apply path (ADR-0006/0012).

**Remaining (product):**
- The desktop chat surface (§4 spec): transcript with inline action cards,
  provider status dot, refusal states, "omit when unusable".
- Wiring assistant proposals into the unified approval queue (slice 1) and
  real tasks (slice 4) so conversation manages actual schedule items.
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
