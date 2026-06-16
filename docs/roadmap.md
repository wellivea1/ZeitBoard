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

Exit criteria: a fatigued user can read current state, recent drift, and the
next predicted windows in seconds; every chart has a non-visual equivalent;
usability research (synthetic/participant-controlled data) shows no one reads an
estimate as exact.

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

A local-first assistant to manage the schedule and answer questions about one's
own data — proposing, never applying, and never advising medically.

- **Manage the calendar by conversation.** The assistant turns requests
  ("find me 90 min for taxes before Friday, not right after I wake") into
  **proposals in the approval queue** — it has no authority to apply changes
  directly. Inspirations: an OpenCode-style transcript with inline tool/action
  cards; a board-style queue for the resulting pending changes.
- **Answer questions about local state** — current estimated phase, recent
  drift, next predicted windows, what a proposal means, why a task moved — always
  with uncertainty and civil time, and a hard refusal boundary on diagnosis,
  prescribing, dosing, and treatment timing.
- **Backend decision is a gated milestone.** Default is local/offline parsing
  with no health data leaving the device. Any cloud LLM is opt-in, off by
  default, scoped to the minimum non-identifying context, and requires its own
  privacy review and threat-model update (same bar as the relay). The UI always
  shows whether the assistant is running locally or via a connected service.

Spec: [`ui-ux-feature-specs.md` §4](ui-ux-feature-specs.md).

Exit criteria: the assistant cannot mutate the schedule except by creating
approval-queue proposals; it refuses medical questions with a consistent,
non-alarming script; with the default backend, no health payload leaves the
device; the active backend is always disclosed in the UI.

---

## Cross-cutting tracks (every phase)

- **Uncertainty system:** ranges not points; ordinal confidence; non-color-only
  encodings — applied to every new chart, proposal, and assistant answer.
- **Accessibility:** keyboard, screen-reader, reduced-stimulation, and 44px
  touch targets are acceptance criteria for each new surface, not a later pass.
- **Privacy & threat model:** each feature that adds a data source, a network
  call, or an external surface updates `privacy.md` and `threat-model.md` before
  it ships.
- **Contracts:** new surfaces extend the versioned schemas (and an ADR) rather
  than inventing UI-only data.

---

## Deferred / non-goals

No phase includes: exact circadian phase/DLMO claims; autonomous health
recommendations; the assistant or scheduler applying changes without approval;
hidden background collection; advertising; data brokerage; default cloud upload;
or a default cloud LLM. Any of these would require a new product scope, privacy
review, threat model, and user-consent design.
