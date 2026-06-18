# Roadmap

ZeitBoard grows from a local-first estimation scaffold into a daily-use planner
for free-running sleep-wake rhythms. Every phase keeps the same guarantees:
civil time stays primary, uncertainty stays visible, automation never applies
itself silently, and the app never gives medical advice or claims exact
circadian phase/DLMO.

Detailed, build-ready UI for the phase 2–4 features below lives in
[`ui-ux-feature-specs.md`](ui-ux-feature-specs.md); the master product design is
[`ui-ux-design.md`](ui-ux-design.md). The implementation/deferment map for
analysis and UI specs is [`specification-alignment.md`](specification-alignment.md).

---

## Phase 1 — Executable scaffold  ✅ (delivered)

- Strict v1 contracts and deterministic synthetic fixtures (Go `tools/` module).
- Append-only observations/corrections and effective reads.
- Robust drift estimation with typed refusal and uncertain forecasts.
- Deterministic schedule proposals without mutating fixed events.
- Desktop (Wails+React), trusted-web fixture prototype, Android fixture shell.
- Default-deny projection, privacy checks, green cross-platform CI.

Exit criteria: the acceptance checks in `implementation-plan.md`.

---

## Phase 2 — Local usability + the rhythm visualizer

Make the local single-user experience trustworthy and legible.

**Core usability**
- Harden import validation, conflict handling, retention, export, deletion.
- Represent source-specific missingness and forced-schedule/travel disruptions.
- User-visible provenance and correction history; correction diffs + undo.
- Refusal states, proposal review, and onboarding.
- Accessibility (WCAG 2.2 AA), localization readiness, time-zone test coverage.
- Light/dark parity and independent reduced-stimulation controls.
- Evaluate local DB encryption and OS credential storage.

**Desktop theme, reduced-stimulation, and accessibility consolidation**
- Add an Auto / Light / Dark appearance selector to desktop Settings, backed by a
  small theme module that applies `data-theme="light|dark"` to `<html>`,
  persists the choice, follows `prefers-color-scheme` in Auto, and applies before
  first paint.
- Add an independent reduced-stimulation toggle backed by `data-reduced="true"`;
  it must work in both light and dark themes, never force dark mode, persist
  locally, soften motion/saturation/contrast/density, and continue honoring
  `prefers-reduced-motion`.
- Extend the existing CSS custom property system rather than creating a parallel
  palette. Audit theme-relevant hardcoded colors in the desktop stylesheet so
  dark mode can override app chrome, panels, charts, status/confidence states,
  actogram/drift visuals, and trust-loop UI without invisible text or broken
  contrast.
- Close the desktop WCAG 2.2 AA gaps across Overview, Calendar, Tasks,
  Approvals, Rhythm, Medications, Sharing, Data Sources, and Settings: body text
  contrast, visible focus rings, 44px interactive targets, logical keyboard
  order, non-color-only status/confidence/conflict/origin cues, and polite live
  status announcements where state changes are surfaced.
- Validate with desktop tests for theme and reduced-stimulation persistence,
  full frontend quality checks, `scripts/dev.ps1 -Action check -Component
  desktop`, contrast spot-checks in both themes, and in-app browser smoke checks
  for Overview, Approvals, and Rhythm in light, dark, and reduced-stimulation
  combinations.
- Out of scope for this pass: Android theming, trusted-web prototype theming,
  backend/data changes, and new screens. Track those separately after the
  desktop consolidation lands.

**Sleep chart visualizer** (new — low risk; visualizes existing local data only)
- A dedicated **Rhythm** area (absorbs the current Timeline) with three views:
  **Actogram** (double-plotted sleep bands showing the free-running diagonal,
  Zeitlog-style), **Phase/Drift trend** (sleep-onset vs date with the Theil-Sen
  fit and ± band), and **Sources & corrections** (the existing provenance
  timeline).
- Forecast region rendered as widening uncertainty bands, never hard lines.
- Full keyboard + screen-reader table alternative for every chart.
- No new data sources, no network: this is a read-only projection of
  observations, corrections, and the current estimate. Spec:
  [`ui-ux-feature-specs.md` §3](ui-ux-feature-specs.md).
- **Automatic clinical charting + clinician export:** auto-generate the
  longitudinal clinical actogram (one row per calendar date, configurable day-start
  anchor, single- or double-plot) with intervention/medication/disruption
  annotations from logged data, and export a printable, redaction-controlled PDF
  for a sleep clinician — replacing hand-kept sleep logs. Recording only; never a
  light/melatonin recommendation. Leans on the import hardening above. Reference
  format = real clinical sleep logs + a SleepGraph-style actogram, kept out of the
  repo as private data. Spec: [`ui-ux-feature-specs.md` §3.6](ui-ux-feature-specs.md).

Current implementation status: the first desktop trust-loop slice is in place,
and the first desktop-local sleep-data slice has landed with manual sleep entry,
immutable local observations, append-only correction history, and real effective
reads for Overview, Rhythm, and Proposals. The desktop trust-loop UI still includes
Approvals, Rhythm tabs, a double-plotted actogram with
widening forecast bands, a sleep-onset Drift chart, source conflict/missingness
review, correction diff and undo affordance, refusal copy, and an Overview
review strip. The Rhythm visualizer is now **engine-backed**: the actogram bands,
the Theil-Sen drift fit and its robust uncertainty band, the widening forecast,
the slope/confidence labels, and the drift chart's y-range are all derived by the
Go estimation engine (`estimation.Project`, exposed over the `GetRhythm` Wails
binding) over the same sessions the Overview uses, so the two screens never
disagree; the screen falls back to the shared fixture only when the Wails service
is unavailable in a browser preview and shows whether it is running on the local
estimate, local data, or sample data.
The desktop service no longer substitutes synthetic sleep sessions for a local
store; manual desktop observations now drive the core estimator. The **Approvals
trust gate is now engine-backed** too: the
`GetProposals` Wails binding runs the real `scheduling.Scheduler` over the current
estimate's predicted waking windows plus the current functional window, so every
queued proposal is one the engine actually produced, with
contract-aligned `explanation_codes` for the constraints it enforced and an
honest `unplaced` list (reason codes) when no safe
window exists. The scheduler emits only guarantees it applied and never claims an
`uncertainty_buffer_applied` it does not yet perform. Desktop theming
(Auto/Light/Dark) and an independent reduced-stimulation toggle are implemented
with before-first-paint application, `localStorage` persistence, extended CSS
custom properties, and tests; the WCAG 2.2 AA contrast/target-size consolidation
pass has landed with a regression test. An **estimation accuracy harness**
(`estimation.Backtest`) now answers "is the estimate any good?" by walk-forward
validation — refit on each prefix, predict the next sleep onset, and score point
error, forecast-window hit-rate, and per-confidence calibration. Early findings on
synthetic data: a clean linear rhythm is recovered with ~0h error and a 1.0
hit-rate; a 3h mid-series phase jump (relative coordination) drives median point
error to ~3h while the engine correctly *drops* its confidence — and the
forecast-window hit-rate stays 1.0, confirming the predicted windows are honestly
wide (so point error, not hit-rate, is the sharper quality signal, and window width
is a candidate for future tightening). Approval decisions are still in-session (no
write-back); persisting decisions, import hardening, export/deletion UI, backend
sync of the same contract-shaped local records, running the harness on
participant-controlled real history, and full onboarding remain phase-two work.

Exit criteria: a fatigued user can read current state, recent drift, and the
next predicted windows in seconds; the app is keyboard-operable and meets WCAG 2.2
AA, with screen-reader text equivalents for charts wherever they don't compromise
the visuals; usability research (synthetic/participant-controlled data) shows no
one reads an estimate as exact.

---

## Phase 3 — Interoperability: auto-syncing calendar + approval queue

Bring external calendars in, and let the planner *propose* changes — but gate
every outward or schedule-altering action behind explicit human approval.

**3a. Read-only calendar import**
- Least-privilege, read-only adapters (e.g. ICS/CalDAV/Google read scope) behind
  an availability + permission boundary, mirroring the Health Connect pattern.
- Imported events are immutable inputs (like fixed events); their text stays
  local and is never projected to trusted views.
- Per-source sync status, last-success time, and conflict/duplicate handling.

**3b. Approval queue (the safe-automation gate) — prerequisite for any change**
- A first-class **Approvals** surface. Nothing the system or assistant decides is
  applied automatically; every move/placement/reminder shift becomes a *pending
  proposal* the user approves, edits, or rejects, with plain-language reasons
  (explanation codes), civil + rhythm context, confidence, and one-tap undo.
- Fixed/imported events are never moved; only flexible tasks and app-created
  reminders are ever proposed for movement.
- Batch review; expiry of stale proposals; full audit trail.

**3c. Two-way sync (write-back) — only through the queue**
- Approved changes to *app-owned* flexible items may be written back to a
  connected calendar via a least-privilege write scope.
- Gated behind: the approval queue, the relay/security review already required
  for any non-local transport, and explicit per-calendar write consent.
- Remote revocation semantics and metadata-minimizing operations.

Spec: [`ui-ux-feature-specs.md` §2](ui-ux-feature-specs.md).

Exit criteria: no code path applies a calendar change without a recorded
approval; read-only import works with revocable, least-privilege scopes;
write-back is off by default and passes a security review before enablement.

---

## Phase 4 — Conversational assistant (chatbox)

An assistant to manage the schedule and answer questions about one's own data —
proposing, never applying, and never advising medically. Per
[ADR-0008](decisions/0008-self-hostable-backend-byok-llm.md) the assistant uses a
**bring-your-own-key, multi-provider LLM** (the user supplies their own key; a
local/offline mode may remain as a fallback).

- **Manage the calendar by conversation.** The assistant turns requests
  ("find me 90 min for taxes before Friday, not right after I wake") into
  **proposals in the approval queue** — it has no authority to apply changes
  directly. Inspirations: an OpenCode-style transcript with inline tool/action
  cards; a board-style queue for the resulting pending changes.
- **Answer questions about local state** — current estimated phase, recent
  drift, next predicted windows, what a proposal means, why a task moved — always
  with uncertainty and civil time, and a hard refusal boundary on diagnosis,
  prescribing, dosing, and treatment timing.
- **Bring-your-own-key, multi-provider LLM (ADR-0008).** The user supplies their own
  key; integrated providers are OpenCode Zen, OpenRouter, OpenAI, and Anthropic (modeled
  on OpenCode, OpenCode Go reference). The project ships no keys; context sent to the
  user's chosen provider is minimized and redacted, and the active provider is always
  disclosed. The model still only *proposes* (approval gate) and never advises medically;
  the provider's data terms are the user's relationship.
- **The assistant's action registry doubles as an agent-accessible interface.** The
  same allowlisted, *propose-only*, redacted capability layer the in-app assistant
  uses is exposed to an external agent so the whole app can be driven non-visually by
  conversation + live voice — the intended primary path for blind users (see the
  Agent-accessible interface cross-cutting track and ADR-0006). Delivered as a **local
  MCP connector** (leading option; works with a local agent so health data can stay on
  device) and/or a **Claude/ChatGPT skill** (cloud, opt-in, gated). The agent only
  proposes; the approval queue and medical refusal are unchanged; ZeitBoard ships no
  speech stack (voice is the client's).

Spec: [`ui-ux-feature-specs.md` §4](ui-ux-feature-specs.md);
agent interface = [ADR-0006](decisions/0006-agent-accessible-interface.md).

Exit criteria: the assistant cannot mutate the schedule except by creating
approval-queue proposals; it refuses medical questions with a consistent,
non-alarming script; context sent to the cloud backend is minimized and redacted
(and the provider does not train on or retain it); the active backend is always
disclosed in the UI; **every feature is operable non-visually by an agent (read
state + propose actions) through the allowlisted, redacted capability layer.**

---

## Cross-cutting tracks (every phase)

- **Uncertainty system:** ranges not points; ordinal confidence; non-color-only
  encodings — applied to every new chart, proposal, and assistant answer.
- **Accessibility — visual-first, accessible where reasonable:** the product is
  visual-first for its primary audience (sighted Non-24) and never sacrifices visual
  feedback, but every element that can reasonably be made accessible should be:
  accessible names, non-color-only cues, keyboard operation, WCAG 2.2 AA,
  reduced-stimulation, 44px targets, and chart text equivalents where they don't
  compromise aesthetics or functionality. See `accessibility.md`.
- **Agent-accessible interface:** every feature exposes a non-visual, agent-operable
  surface — structured readable state + allowlisted *propose-only* actions through the
  approval gate. The intended primary path for blind users is an **agent + live voice**
  (a local MCP connector or a Claude/ChatGPT skill), not a transcription of the charts;
  cloud agents are opt-in and gated like any connected backend. This is a standing
  design constraint, not a later pass. See ADR-0006 and Phase 4.
- **Privacy & threat model:** the product is **connected, self-hosted, and BYOK**
  ([ADR-0007](decisions/0007-connected-cloud-architecture.md) +
  [ADR-0008](decisions/0008-self-hostable-backend-byok-llm.md)) — an **entirely
  self-hostable** Go backend syncs the user's data to *their own* instance (TLS,
  encrypted at rest), and the assistant LLM is **bring-your-own-key, multi-provider**
  (no shipped keys). The project runs no service and collects no telemetry; the operator
  is the data controller. Legal scope **US / North Carolina**. Backend Milestone 1 has
  landed: `apps/server` provides authenticated device enrollment, TLS sync endpoints,
  strict v1 sync validation, idempotent append-only push/pull, and AES-256-GCM encrypted
  payload storage. Backend Milestone 2 has landed too: BYOK providers, redacted assistant
  context, strict JSON action validation, pending proposals, one-use approval tokens,
  provider disclosure, device revocation, and encrypted proposal/audit storage. Backend
  Milestone 3 has landed: the server derives effective sleep sessions from synced
  observations/corrections, computes estimates with the core engine, and exposes
  authenticated overview, rhythm, and accuracy projections with typed refusals. Backend
  Milestone 4 has landed: a local stdio MCP adapter exposes those read projections and
  propose-only scheduling tools over the backend API with call budgets, while approval
  remains the human one-use-token path. Cloud skill packaging and reviewer-gated
  auto-apply remain future work.
- **Contracts:** new surfaces extend the versioned schemas (and an ADR) rather
  than inventing UI-only data.

---

## Deferred / non-goals

No phase includes: exact circadian phase/DLMO claims; autonomous health
recommendations; the assistant or scheduler applying changes without approval;
hidden background collection; advertising; or data brokerage. Each would require a
new product scope and user-consent design. (Cloud sync of the user's data to their
account and a cloud LLM backend are now **in scope** per
[ADR-0007](decisions/0007-connected-cloud-architecture.md) — gated by explicit
consent, encryption, and export/deletion — and are no longer non-goals.)
