# ZeitBoard — UI/UX & Product Design Specification

**Status:** Design handoff, phase one → phase two.
**Audience:** Codex (implementation), core/contract authors, future usability research.
**Scope:** Windows desktop (Wails + React), Android companion (Kotlin/Compose), trusted-person web view (static, mobile-first).
**Not in scope:** Production code, visual mockup PNGs, brand wordmark.
**Companion:** build-ready, placement-first specs for the calendar sync + approval queue,
sleep-chart visualizer, and conversational assistant live in
[`ui-ux-feature-specs.md`](ui-ux-feature-specs.md); phasing is in [`roadmap.md`](roadmap.md).

This document is grounded in the existing repository contracts (`contracts/v1/*`), the
`OverviewDTO` already exposed by `apps/desktop/app.go`, and the product-language rules in
`AGENTS.md`. Where this document and a contract disagree, the contract wins and this
document should be amended.

> **Language rule enforced throughout.** This product estimates an *observed sleep-wake
> rhythm*. It says **estimated sleep-wake phase**, **likely awake / asleep**, **predicted
> sleep window**, **predicted wake window**, **time since wake**, **current rhythm state**,
> and **forecast confidence**. It never claims exact biological time, circadian phase, or
> DLMO from device activity, and it never diagnoses, prescribes, or recommends treatment,
> light, melatonin, meals, or exercise.

---

## Table of contents

1. [Executive summary](#1-executive-summary)
2. [Product principles](#2-product-principles)
3. [Personas and jobs-to-be-done](#3-personas-and-jobs-to-be-done)
4. [Information architecture](#4-information-architecture)
5. [Desktop navigation](#5-desktop-navigation)
6. [Android navigation](#6-android-navigation)
7. [Trusted-website structure](#7-trusted-website-structure)
8. [End-to-end user flows](#8-end-to-end-user-flows)
9. [Detailed screen specifications](#9-detailed-screen-specifications)
10. [Low-fidelity wireframes](#10-low-fidelity-wireframes)
11. [Responsive behavior](#11-responsive-behavior)
12. [Component inventory](#12-component-inventory)
13. [Component states and variants](#13-component-states-and-variants)
14. [Design tokens](#14-design-tokens)
15. [Typography and icons](#15-typography-and-icons)
16. [Light and dark themes](#16-light-and-dark-themes)
17. [Uncertainty visualization system](#17-uncertainty-visualization-system)
18. [Accessibility specification](#18-accessibility-specification)
19. [Privacy and sharing UX](#19-privacy-and-sharing-ux)
20. [Error, offline, and missing-data states](#20-error-offline-and-missing-data-states)
21. [Microcopy examples](#21-microcopy-examples)
22. [Notification strategy](#22-notification-strategy)
23. [Usability-testing plan](#23-usability-testing-plan)
24. [MVP vs later prioritization](#24-mvp-vs-later-prioritization)
25. [Design risks and bad ideas to avoid](#25-design-risks-and-bad-ideas-to-avoid)
26. [Codex implementation handoff](#26-codex-implementation-handoff)

---

## 1. Executive summary

ZeitBoard helps a person whose sleep-wake timing drifts a little later (or earlier)
every day answer three plain questions, on demand and at a glance:

- **Am I likely awake, asleep, or is it uncertain right now?**
- **When is my next good window to do a thing — and when is sleep likely coming?**
- **What do my fixed obligations cost me, given where my rhythm actually is?**

It does this from data the user already controls (Health Connect, a wearable, phone
activity, Windows active/idle, manual corrections, medication logs, calendar). It produces
an **estimate with explicit uncertainty**, not a verdict. It never moves a fixed
appointment by itself, never schedules into a forecast it doesn't trust, and never frames a
late wake or a missed window as a failure.

The design's hardest constraint is not visual; it is **trust under fatigue**. The primary
user is frequently reading this interface while cognitively impaired by circadian
misalignment. So the bar is: *the single most important fact must be legible, correct, and
calm in under three seconds, with the machinery of how we got there available but never in
the way.*

### What this document recommends changing about the brief

The brief is strong and safety-conscious. Three things in it are over-scoped or quietly
risky, and this document pushes back rather than affirming:

1. **Eleven "product states" must not be eleven things the user sees.** Surfacing all of
   them is cockpit design and will read as instability to a tired user. We collapse them
   into **one primary status (Awake / Asleep / Uncertain)** plus a small set of **secondary
   descriptors** and a **provenance tag** (inferred / confirmed / clinician-set). See §9.2
   and §13.

2. **The trusted-person website is the single largest privacy hazard in the product**,
   precisely because it is the only network-exposed surface and a Non-24 availability
   pattern over several days *is* health-revealing. A leaked link should expose as little as
   possible and degrade safely. We treat "several days of availability" as an opt-in,
   passcode-gated, horizon-limited feature, not a default. See §19.

3. **"Migraine mode" must not assume light is the trigger.** The brief already flags this;
   we implement reduced-stimulation as a set of independently toggled reductions (motion,
   contrast intensity, density, color saturation) rather than a single "dark = safe"
   assumption. See §18.

Everything else in the brief is adopted.

---

## 2. Product principles

These are ranked. When two conflict, the lower number wins.

> **Baseline assumed by every principle below — non-visual parity.** The product
> must be fully operable and understandable with a screen reader and keyboard,
> with no information carried by color or spatial position alone. Many people with
> Non-24 are totally blind, so screen-reader use is a *primary* mode, not an
> accommodation. See §18 and [`accessibility.md`](accessibility.md).

1. **One-glance truth.** The top of every primary screen answers that screen's main
   question in one sentence a tired person can parse. Everything else is secondary.
2. **Civil time is the spine.** Real clock time and date are always present and primary.
   Rhythm-relative context ("4 h since wake") supplements; it never replaces the clock.
3. **Uncertainty is shown, never laundered.** A weak forecast must *look* weak. We never
   render an estimate as a hard line, an exact minute, or a percentage we didn't earn.
4. **No punishment.** No "failed," "missed," "noncompliant," no streaks, no scores, no
   rings, no nagging. Lateness is information, not a verdict.
5. **The user is the authority.** They can confirm or correct any inferred sleep/wake, lock
   the current state, reject a task move, and revoke any share — and the system visibly
   obeys, immediately.
6. **Automation explains itself.** Every proposed window and every moved reminder carries a
   plain-language reason and a one-tap undo. If we can't explain it, we don't do it.
7. **Progressive disclosure.** Status first; actograms, source conflicts, and statistics
   live deeper. Depth is opt-in, never the landing experience.
8. **Privacy is the default, not a setting.** Especially the trusted view: deny by default,
   minimize always, expire by design.
9. **Refusal is a first-class result.** "Too uncertain to schedule" and "not enough data"
   are correct, expected outcomes shown plainly — not errors, not blanks, not guesses.
10. **No medical improvisation.** We display clinician-defined rules when they exist; we
    invent none. We describe what was observed, never what should be done.

---

## 3. Personas and jobs-to-be-done

### 3.1 Maya — severe, free-running Non-24 (primary user)

Drifts ~50 min later per cycle; no stable day/night. Often reads the app mid-fatigue.
- **JTBD:** "When I surface, tell me *plainly* whether I'm in a usable window and how long I
  likely have before sleep, so I can decide what's worth doing right now."
- **JTBD:** "When my rhythm shifts after I fix a bad night's data, show me what changed
  without making me re-learn the whole screen."
- **Needs:** giant legible status, confirm-wake in one tap, undo on everything, zero blame.
- **Fails if:** the screen looks like a dashboard, the status flickers between states, or a
  wrong inferred wake silently drives a wrong forecast.

### 3.2 David — trusted family member

Maya's father. Wants to know *when it's OK to call* without nagging or surveilling.
- **JTBD:** "Tell me if now is a bad time and when a good time is likely, on my phone, in
  one screen, without an app or a login."
- **Needs:** "best contact window," "likely unavailable now," an urgent path if it's truly
  urgent. **Must not** see sleep history, medications, location, or a diagnosis.
- **Fails if:** the page is complicated, or it leaks the underlying pattern.

### 3.3 Priya — friend / collaborator

Only needs availability for planning a call or a shared task.
- **JTBD:** "Give me a few candidate windows over the next days so I can propose a time that
  isn't cruel."
- **Needs:** coarse availability, expiry, nothing personal. Lower trust tier than David.

### 3.4 Dr. Okafor — clinician

Reviews longitudinal pattern and medication context across weeks; time-boxed.
- **JTBD:** "Show me the drift trend, the actogram, and when medication was actually taken
  relative to the moving rhythm — with provenance — so I can reason about it."
- **Needs:** export, date ranges, provenance, the ability to define a rule the app then
  *displays* (not invents). **Must not** receive marketing-grade certainty.
- **Fails if:** the app implies it diagnosed or dosed anything.

### 3.5 Sam — incomplete-sensor user

No wearable; only phone + occasional manual logs. Lots of gaps and false sleep detections.
- **JTBD:** "Let me correct what the app got wrong quickly, and tell me honestly when it
  doesn't have enough to say anything."
- **Needs:** fast manual correction that doesn't destroy source data; honest "insufficient
  data"; clear sense of what each source adds.
- **Fails if:** the app pretends to know, or correcting feels like data-entry punishment.

### 3.6 Noor — totally blind Non-24 user (co-primary; screen-reader only)

Blind since birth; uses the app entirely via a screen reader + keyboard (desktop) and
TalkBack (Android), never seeing a chart. Blindness is a leading cause of Non-24, so Noor is
as central as Maya — not an edge case.
- **JTBD:** "Read me, out loud and in order, whether I'm likely awake, how long until my next
  predicted sleep window, and what's pending — without me ever needing to see a chart."
- **Needs:** every chart as a navigable table; an accessible name on every control; spoken
  state changes; complete keyboard operation; civil times announced explicitly.
- **Fails if:** any actogram/drift/calendar is visual-only, a control is unlabeled, meaning is
  carried by color or position, or a state change happens silently.

---

## 4. Information architecture

### 4.1 Shared object model (mirror of `contracts/v1`)

The UI is a thin, honest projection of these objects. The same vocabulary appears in copy,
component names, and the handoff.

| Object | Source contract | UI meaning |
|---|---|---|
| Observation | `observation-set.schema.json` | An imported/recorded interval (`sleep_episode` or `activity_interval`) with provenance. **Append-only.** |
| Correction | `correction-set.schema.json` | A user edit layered over an observation. Original is never overwritten. |
| Effective observation | (computed) | What the timeline actually shows: observation + valid correction chain. |
| Estimate | `phase-estimate.schema.json` (`status:"estimated"`) | Drift/cycle, median sleep duration, support counts, **confidence {low\|medium\|high}**, and forecast windows. |
| Refusal | `phase-estimate.schema.json` (`status:"refused"`) | Typed reason: `insufficient_data`, `ambiguous_cycle_index`, `conflicting_observations`, `unsupported_input`. A first-class screen state, not an error toast. |
| Forecast window | `common.schema.json#uncertainWindow` | `earliest_at..latest_at` (+ zone). **Always a range, never a point.** |
| Fixed event | `schedule-request.schema.json#fixed_events` | Immutable constraint. Never auto-moved. |
| Task | `schedule-request.schema.json#tasks` | duration, `earliest_at..latest_at`, `preference ∈ {predicted_waking_window, daytime, any_available}`. |
| Proposal | `schedule-proposals.schema.json#proposals` | A placed window + `explanation_codes` + confidence. |
| Unplaced | `schedule-proposals.schema.json#unplaced` | Why a task couldn't be placed: `no_available_interval`, `outside_forecast_horizon`, `estimate_unavailable`, `invalid_constraints`. |
| Medication event | `OverviewDTO.MedicationEventDTO` + core `medication` | Dose at civil time, `time since wake`, `time before predicted sleep`, confidence, source. No advice. |
| Share profile | `share-profile.schema.json` | Per-recipient allowlist of `{predicted_sleep_window, predicted_waking_window, confidence, availability}`, lifecycle `{active, revoked, expired}`, expiry. |
| Trusted view | `trusted-view.schema.json` | The minimized DTO actually sent out. Carries the fixed `notice` string. |

### 4.2 Provenance vocabulary (used everywhere)

From `common.schema.json#provenance`. The UI shows two independent axes — never collapse
them into one word:

- **acquisition_method:** `manual`, `health_connect`, `os_activity`, `file_import`,
  `synthetic` → rendered as a **source icon**.
- **evidence_status:** `directly_observed`, `user_reported`, `inferred` → rendered as a
  **confirmation marker** (anchor = confirmed, dotted = inferred).

### 4.3 Feature areas

```
Overview      "What's true right now."        (status, time-since-wake, next sleep, next window, next event, data warnings, shares)
Calendar      "How obligations sit in my rhythm."   (civil grid + predicted sleep/wake overlay + burden)
Tasks         "Put flexible work in good windows."  (capture, constraints, proposals, undo/lock)
Timeline      "What actually happened + corrections."  (multi-track actogram, edit without destroying source)
Medications   "When I took things, relative to my rhythm."  (log, civil + relative timing, no judgment)
Sharing       "Who can see what, for how long."     (profiles, preview, expiry, revoke, access log)
Data Sources  "Where data comes from and conflicts."  (status, last import, priority, conflicts)
Reports/Export "Summaries for me and my clinician."  (actogram, drift, provenance, redaction, CSV/JSON)
Settings      "Make it usable for me."              (themes, reduced-stimulation, notifications, units, account)
```

---

## 5. Desktop navigation

Wails shell, system-tray resident. Left rail (collapsible to icons), persistent top status
strip, content pane.

```
┌──────────────────────────────────────────────────────────────────────────┐
│ [≡]  ZeitBoard            ● Likely awake · 4h 12m since wake   [◐][?] │  ← global status strip
├───────────────┬────────────────────────────────────────────────────────────┤
│ ◎ Overview    │                                                            │
│ ▦ Calendar    │                  CONTENT PANE                              │
│ ✓ Tasks       │                                                            │
│ ∿ Timeline    │                                                            │
│ ⊕ Medications │                                                            │
│ ⇄ Sharing     │                                                            │
│ ⛁ Data Sources│                                                            │
│ ⤓ Reports     │                                                            │
│ ⚙ Settings    │                                                            │
│ ───────────── │                                                            │
│ ◐ Reduce      │                                                            │
└───────────────┴────────────────────────────────────────────────────────────┘
```

- **Global status strip** is always visible and is the desktop's "one-glance truth." It
  mirrors `OverviewDTO.CurrentEstimatedState` + `TimeSinceWake`. Clicking it jumps to
  Overview. It is the only place status lives outside Overview, so it can't contradict the
  body.
- **Tray menu:** current status line, "Confirm I'm awake now," "Log a dose," "Open
  Overview," "Reduce stimulation," "Pause data collection," "Quit." (No data values beyond
  the one status line — the tray is glanceable, not a leak surface.)
- **Critique of the brief's 9-area desktop list:** keep all nine, but **Reports and Export
  are one area** (export is a verb inside Reports), and **Data Sources owns conflict
  resolution** rather than spawning a tenth area. Confirmed.

---

## 6. Android navigation

Five-item bottom bar, touch-first, one-handed reachable. Designed to be usable at 3 a.m. on
4 hours of sleep.

```
        ┌─────────────────────────────┐
        │   ● Likely awake            │   ← Home hero
        │     4h since wake           │
        │                             │
        │   [ Good window now ]       │
        │   Until ~21:40              │
        │                             │
        │   Next sleep likely         │
        │   ~21:40–22:40 (±)          │
        └─────────────────────────────┘
        ┌────┬────┬─────┬────┬────────┐
        │Home│Time│  +  │Task│ More   │
        │ ◎  │ ∿  │ Log │ ✓  │  ⋯     │
        └────┴────┴─────┴────┴────────┘
```

- **Home** — status hero + next sleep + next window + next event. (Overview, slimmed.)
- **Timeline** — read-mostly actogram; tap to correct.
- **+ Quick Log** — center FAB-style action: confirm wake, log sleep/nap, log dose,
  reject false sleep. The single most important Android action; reachable by thumb.
- **Tasks** — list + proposals + undo.
- **More** — Medications history, Sharing, Data Sources, Settings, Reports.

**Critique of the brief's Android list:** the brief's five (Home, Timeline, Quick Log,
Tasks, Settings) is good, but **Settings should not be a top-level tab** — it's rarely
used at 3 a.m. Promote **Quick Log** to the center and demote Settings into **More**. This
is the single most-used surface on mobile and must be a thumb-tap from anywhere.

---

## 7. Trusted-website structure

One page. No navigation, no login, no app. Renders **only** a `trusted-view.json` DTO
(already minimized server/desktop-side). The page literally cannot request more.

```
single route:  /v/{opaque-token}          (token = capability; no user id in URL)
optional gate: passcode entry → then content
states:        valid · expired · revoked · passcode-required · invalid/unknown
```

Content blocks are **conditional on `granted_fields`** and nothing else:

- Headline availability ("Best reached: …" / "Probably unavailable now")
- Best contact window (from `predicted_waking_window`, relabeled, coarsened)
- Several-day outlook (only if `availability` granted; opt-in, see §19)
- Urgent contact instruction (static text the user wrote)
- The fixed `notice` string, always.

Never present: exact computer/phone activity, medications, diagnosis, location, time-zone
ID, raw sleep history, drift numbers, or provenance.

---

## 8. End-to-end user flows

Notation: `→` step, `⟳` loop/optional, `⊘` refusal/limit path.

### 8.1 First run / onboarding (Maya, desktop)

```
Welcome (what we can/can't infer)
  → Local-only confirmed (sync = "not in this version", shown as deliberate, not missing)
  → Connect sources (skippable, each independently):
        Health Connect (Android) ⟳  | Wearable ⟳ | Windows activity ⟳ | "I'll add data manually"
  → Privacy explainer (what stays on device; what a share link reveals)
  → Trusted-sharing explainer (default-deny; you choose every field)
  → Baseline period set: "We'll watch for ~7–14 cycles before forecasting. Until then we
     show what we see, not predictions."  ⊘ enters Insufficient-data Overview by design
  → Land on Overview (honest empty/low-data state)
```

Design rule: onboarding **cannot** end in a state that implies the app already knows the
user's rhythm. Day one Overview says "Still learning your rhythm," not a confident forecast.

### 8.2 Daily check (Maya, Android, fatigued)

```
Open app → Home hero answers status in <3s
  → if status inferred & user disagrees: tap "Not quite—I woke at…" → corrects wake
        → forecast recomputes → "Updated. Your next sleep window moved ~40 min later."
  → "Good window now?" → yes/coarse → optional: open Tasks for what fits
```

### 8.3 Capture + place a task (Maya)

```
Tasks → "+ New" → natural language "email Dr. Okafor, ~30 min, before Friday, daytime ok"
  → parsed into structured fields (user can edit every field)
  → choose: [Let the app suggest a window]  or  [I'll place it]
  → if suggest: proposal shows window + reasons (explanation_codes) + confidence
        accept → placed (with undo)   | reject → stays unplaced, app won't nag
  ⊘ if unplaced: show reason in plain words (no_available_interval / outside_forecast_horizon /
        estimate_unavailable / invalid_constraints) + the one constraint to relax
```

### 8.4 Fixed appointment inside predicted sleep (Maya + David context)

```
Calendar shows event overlapping predicted sleep band
  → burden chip = "high" + plain reason
  → app offers lower-burden alternative TIMES (never auto-moves) → "Suggest to me" copies a
     proposed time to clipboard / draft; the user decides
  → if event truly can't move: app switches to support mode — "Set a gentle pre-event
     reminder?" + "This is likely to cost you sleep; that's information, not a failure."
```

### 8.5 Create + preview + revoke a share (Maya → David)

```
Sharing → New profile → pick relationship template (Family/Friend/Clinician/Collaborator)
  → toggle exactly which of {sleep window, wake window, confidence, availability} David sees
        (all default OFF)
  → set expiry (required) + optional passcode
  → PREVIEW exact recipient view (renders the real trusted-view.json)
  → create link → copy → send out-of-band
  ⟳ later: view access log · extend/shorten expiry · REVOKE (one tap, immediate, confirmed)
```

### 8.6 Correct a false wearable sleep (Sam)

```
Timeline → tap the suspect sleep block (source: wearable, status: directly_observed)
  → "This wasn't sleep" → choose: [quiet wakefulness] [I was using my phone] [delete from estimate]
  → creates a correction (source block stays, shown faded with a "corrected" marker)
  → "This changed your estimate." → before/after forecast diff, with undo
```

---

## 9. Detailed screen specifications

Each screen: **Purpose · Hierarchy · Key data · Actions · States · Errors.**

### 9.1 Onboarding

- **Purpose:** Set honest expectations and connect sources without pressure.
- **Hierarchy:** one idea per step; a single primary button; "Skip / do this later" always
  available and never styled as a mistake.
- **Key data:** capability copy, per-source permission status, baseline length.
- **Actions:** Connect each source, Skip, Continue, Set local-only.
- **States:** not-started · permission-pending · granted · denied (explained, retryable) ·
  unsupported (e.g., Health Connect < API 28 → "Your device can't share this; add data
  manually").
- **Errors:** permission denied → never a dead end; route to manual entry.

### 9.2 Overview (the heart of the product)

- **Purpose:** Answer "what's true right now" in one glance; offer the few next actions.
- **Hierarchy (top→bottom, strict):**
  1. **Status line** — `CurrentEstimatedState` ("Likely awake") + provenance tag
     (inferred/confirmed) + `TimeSinceWake`. Largest type on the screen.
  2. **Now-strip** — a single horizontal mini-timeline: now-marker, the current functional
     window, and the **predicted next sleep window** as a widening band.
  3. **Three quiet facts** (not cards-in-a-grid; a calm stacked list):
     `Next useful task window`, `Next fixed appointment + burden`, `Recent drift` (e.g.
     "+50 min/cycle, settling later").
  4. **Confidence row** — ordinal meter (low/med/high) + first reason; tap → all reasons.
  5. **Attention area** (appears only when relevant): data-quality warning, active share
     links count, "estimate paused."
- **Key data (from `OverviewDTO`):** `currentEstimatedState`, `timeSinceWake`,
  `predictedNextSleepWindow`, `driftEstimate`, `confidence` + `confidenceReasons`,
  `nextUsefulTaskWindow`, `sharingStatus`, `medicationEvents`, `fixtureMode`, `disclaimer`.
- **Actions:** Confirm wake · Correct wake · Log dose · Lock current state · "Why?" on any
  fact.
- **States:** confirmed-wake · inferred-wake · uncertain (status reads "Uncertain — likely
  in transition") · insufficient-data ("Still learning your rhythm") · estimate-paused ·
  offline · `fixtureMode` banner.
- **Errors:** estimate refusal → the three-facts block is replaced by a single honest
  panel (see §20), not a broken layout.
- **Anti-pattern guard:** **no more than one accent color visible at rest.** No grid of
  metric cards. If it starts to look like a flight deck, cut a fact.

### 9.3 Calendar

- **Purpose:** Show how fixed events sit against predicted sleep/wake; surface burden;
  suggest gentler times.
- **Hierarchy:** civil-time grid is primary and visually normal; predicted sleep/wake are
  an **overlay** (translucent bands), not the grid itself.
- **Key data:** fixed events (immutable), predicted sleep/wake windows with widening
  uncertainty bands, per-event burden indicator, alternative-time suggestions.
- **Views:** Day · Week (default) · Month (month shows burden dots only — bands get too
  noisy at month scale; this is intentional simplification).
- **Actions:** create/edit *flexible* event, request alternative times for a fixed event
  (suggestion only), confirm any change via explicit dialog.
- **Burden indicator:** ordinal `low / moderate / high`, derived from overlap with
  predicted sleep + forecast confidence; always paired with a one-line reason.
- **Special handling:**
  - **DST:** events render at stored UTC instant; a small "DST change this week" note
    appears; we never silently shift an event's wall-clock label without flagging it.
  - **Travel/zone change:** show both zones for affected days; forecast confidence drops
    and says why ("recent time-zone change widens uncertainty").
- **States:** normal · low-confidence (bands wide/faded) · refusal (no overlay, civil grid
  still fully usable) · conflict (event overlaps both predicted sleep and a "can't move"
  flag → support mode).
- **Errors:** never block the civil calendar because the estimate failed. Civil time is the
  spine; the overlay is optional.

### 9.4 Tasks

- **Purpose:** Put flexible work into windows the app actually trusts; explain placement;
  make undo trivial.
- **Hierarchy:** capture box on top; then **Today's placed/suggested**, then **Unplaced
  (with reason)**, then **Someday/no-constraint**.
- **Key data per task:** title, `duration_minutes`, `earliest_at..latest_at` (deadline +
  business-hours), `preference ∈ {predicted_waking_window, daytime, any_available}`,
  cognitive/physical demand (UI-level tag), `RequiresApproval`, lock flag.
- **Actions:** capture (NL or structured), let-app-place / place-myself, accept/reject
  proposal, **lock** (pin to a time; app won't move it), **undo** (always), review moved
  tasks.
- **Proposal display:** window + confidence + reasons rendered from `explanation_codes`:
  - `within_predicted_waking_window` → "Lands in a window you're likely awake."
  - `avoids_fixed_event` → "Doesn't collide with a fixed appointment."
  - `within_task_bounds` → "Inside your deadline and earliest-start."
  - `uncertainty_buffer_applied` → "Kept off the edges of the window, in case timing shifts."
- **Unplaced display (plain words + the one lever to pull):**
  - `no_available_interval` → "No window fits before your deadline. Loosen the deadline or
    shorten the task?"
  - `outside_forecast_horizon` → "That's further out than we can predict yet. We'll place
    it as the date nears."
  - `estimate_unavailable` → "We don't have a confident rhythm estimate right now, so we're
    not guessing. You can place it manually."
  - `invalid_constraints` → "Earliest start is after the deadline. Fix the dates?"
- **States:** empty · placed · suggested-pending · unplaced · locked · moved-since-you-saw-it
  (badged, with a review list).
- **Errors:** NL parse fail → fall back to structured form pre-filled with what parsed; never
  lose the user's text.

### 9.5 Timeline

- **Purpose:** Show what actually happened across sources and let the user correct it
  **without destroying source data.**
- **Hierarchy:** a vertically scrolling, civil-time **actogram** (double-plotted optional)
  with stacked tracks:
  ```
  Track 1: Imported sleep (Health Connect / wearable)   ▓▓▓ solid, source icon
  Track 2: Inferred sleep (estimator)                   ░░░ faded, dotted edge
  Track 3: Manual corrections                           ╱╱╱ overlay marker
  Track 4: Phone activity                               · · ·
  Track 5: Desktop active/idle                          · · ·
  Track 6: Medication events                            ⊕ pins
  Track 7: Fixed appointments                           ▮ markers
  ```
- **Key data:** effective observations + their source provenance + confidence + corrections.
- **Edit/correction interaction (critical, non-destructive):**
  - Tapping a block opens an inspector showing **source value (read-only)** and **effective
    value (editable via correction)**.
  - Editing creates a **Correction** record (append-only). The original block remains,
    rendered faded with a "corrected" marker; a toggle reveals "show original."
  - Every correction shows "what this changed" (forecast diff) and offers undo (which
    appends a superseding correction, never a hard delete).
- **Actions:** correct sleep start/end, add nap, mark quiet wakefulness, reject false
  sleep, confirm an inferred wake.
- **States:** rich · sparse · conflict (two sources overlap → both shown, conflict glyph) ·
  stale (a track greyed with "last updated …").
- **Errors:** if corrections form an invalid chain (per core), surface "this edit conflicts
  with a later one" and offer to view the chain — fail closed, never silently drop.

### 9.6 Medications

- **Purpose:** Record doses against both civil time and rhythm, for the user and clinician —
  **observation, not judgment.**
- **Hierarchy:** big "Log a dose" action; then today's doses; then history.
- **Key data per event:** medication label (user's own text), civil timestamp,
  **time since wake**, **predicted time until sleep**, confidence, scheduled-vs-as-needed,
  optional effect/adverse-effect note, source of entry, clinician rule (only if one exists).
- **Actions:** quick log (one tap → now), backdate, add note, mark scheduled/as-needed.
- **Critical styling rule:** **timing difference is shown in neutral ink, never red.** Red
  is reserved for genuine errors and destructive confirmations. "Taken 9 h after wake" is a
  fact in normal color; a clinician rule, if present, may add a neutral "outside your
  clinician's note" tag — informational, not alarming, and clearly attributed to the
  clinician, not the app.
- **States:** logged · relative-timing-known · relative-timing-unknown (`ConfidenceUnknown`
  → "We couldn't tie this to a wake or sleep window") · clinician-rule-present.
- **Errors:** never invent a dose, schedule, or recommendation. If no estimate exists, show
  civil time only and say relative timing is unavailable.

### 9.7 Sharing

- **Purpose:** Let the user grant *minimal, expiring, revocable* visibility per person.
- **Hierarchy:** list of profiles (status + expiry + last access) → profile editor →
  **preview** → link management.
- **Key data:** per-profile `permissions {predicted_sleep_window, predicted_waking_window,
  confidence, availability}` (all default false), `state`, `expires_at`, passcode flag,
  access log, relationship template.
- **Actions:** create, edit fields, set/extend expiry, set passcode, **preview exact
  recipient view**, copy link, **revoke**, view access history.
- **Templates** (pre-set safe defaults; user can still toggle):
  - **Family:** availability + best-contact window. No sleep history, no numbers.
  - **Friend:** availability only.
  - **Clinician:** wake/sleep windows + confidence (still no diagnosis/meds in the *web*
    view — clinicians get detail through Reports/Export, not the public link).
  - **Collaborator:** availability only, short expiry default.
- **States:** active · expiring-soon · expired · revoked · never-accessed · accessed.
- **Errors:** creating a link with **zero** fields granted → allowed, but warned ("This link
  will show only your urgent-contact note"). Expiry is **required** (no permanent links by
  default).

### 9.8 Trusted-person website

- **Purpose:** The recipient's entire experience. Calm, instant, minimal, phone-first.
- **Hierarchy:** one headline answer; one good-window line; optional outlook; urgent path;
  notice.
- **Key data:** only `granted_fields` from `trusted-view.json`.
- **Actions:** none beyond "reveal outlook" (if granted) and the urgent contact action the
  user defined. No accounts, no settings, no history.
- **States:** valid · passcode-required · expired ("This link has expired. Contact {name}
  directly.") · revoked (identical copy to expired — do **not** distinguish, to avoid
  signaling) · invalid token.
- **Errors:** any failure → the safe, contentless "link unavailable" page. Never a stack
  trace, never partial private data.

### 9.9 Data Sources

- **Purpose:** Make provenance and conflicts legible; let the user set source priority.
- **Hierarchy:** per-source rows (status, last import, permission) → conflicts → duplicates
  → missing-data summary.
- **Key data:** connection status, last successful import time, permission scope,
  source-conflict list, duplicate records, gaps, manual priority order, one-line "what this
  source contributes."
- **Actions:** connect/disconnect, re-authorize, set priority, resolve a conflict (which
  writes a correction, not a deletion), pause a source.
- **States:** connected/healthy · degraded (stale) · permission-revoked · unsupported ·
  paused · conflicting.
- **Errors:** revoked permission → explain exactly what stops working and the one-tap path
  to re-grant; never silently keep showing stale data as if live.

### 9.10 Manual corrections (cross-cutting surface, reachable from Timeline, Overview, Quick Log)

- Correct sleep start/end · add nap · mark quiet wakefulness · reject false wearable sleep ·
  confirm inferred wake.
- Always shows the forecast effect ("This moved your next predicted sleep ~40 min later")
  and always undoable. Never destroys source data (see §9.5).

### 9.11 Reports and export

- **Purpose:** Human-readable summaries for the user and the clinician, with provenance and
  redaction.
- **Hierarchy:** date-range picker → report type → preview → export.
- **Key data:** user summary, clinician summary, actogram, drift estimate, provenance,
  medication context, redaction toggles.
- **Actions:** choose range, choose audience (self/clinician), toggle redactions (e.g.,
  hide medication notes), export CSV/JSON, copy summary text.
- **States:** ready · insufficient-data-for-range · partial (gaps flagged, not hidden).
- **Errors:** export never silently drops fields; redactions are explicit and listed in the
  export header.

---

## 10. Low-fidelity wireframes

### 10.1 Overview — desktop, confirmed wake, medium confidence

```
┌──────────────────────────────────────────────────────────────────────────┐
│ ● Likely awake — confirmed                                                 │
│   4 h 12 m since you woke           Mon Mar 15 · 5:48 PM EDT               │
│                                                                            │
│   now ▌                                                                    │
│   ├───────── functional window ─────────┊▒▒▒ predicted sleep ▒▒▒┊──────►   │
│   12pm        4pm        8pm     ~9:40pm│      ~10:40pm                     │
│                                                                            │
│   Next useful task window   →  now until ~9:40 PM                          │
│   Next fixed appointment    →  Wed 2:00 PM · burden: moderate   [why?]     │
│   Recent drift              →  +50 min per cycle, settling later           │
│                                                                            │
│   Confidence ▮▮▯ medium   “10 recent sleeps; uncertainty grows ahead” [▾]  │
│                                                                            │
│   [ Confirm I'm awake ]  [ Correct wake ]  [ Log a dose ]  [ Lock state ]  │
└──────────────────────────────────────────────────────────────────────────┘
```

### 10.2 Overview — insufficient data (day 2)

```
┌──────────────────────────────────────────────────────────────────────────┐
│ ◐ Still learning your rhythm                                               │
│   We need a few more sleep periods before we forecast. So far we're just   │
│   showing what we see.                                                     │
│                                                                            │
│   What we have:  3 sleep periods · 1 source (phone activity)               │
│   What helps:    confirm last night's wake · connect a wearable            │
│                                                                            │
│   [ Confirm last wake ]   [ Add sleep manually ]   [ Connect a source ]    │
└──────────────────────────────────────────────────────────────────────────┘
```

### 10.3 Calendar — week, appointment inside predicted sleep

```
        Mon        Tue        Wed        Thu        Fri
 6a    ▒▒▒▒▒      ░░░░░       ....       ....       ....     ▒ predicted sleep (confident)
 9a    ▒▒▒▒▒      ░░░░░      [Dr 2p]                         ░ predicted sleep (low conf)
12p     ....       ....     ┊▒▒▒▒┊◀ appt overlaps sleep      . predicted wake
 3p    [team]                ▒▒▒▒        ....                ▮ fixed event
 6p     ....       ....       ....      [call]
        ─────────────────────────────────────────
 Wed 2:00 PM “Dr. Okafor” — burden: HIGH
   Reason: overlaps your predicted sleep (10a–4p, medium confidence).
   [ Suggest gentler times ]   [ Keep + set gentle reminder ]
```

### 10.4 Tasks — proposal accepted + one unplaced

```
┌ New task ─────────────────────────────────────────────────────────────────┐
│ "email Dr. Okafor, ~30m, before Fri, daytime ok"        [ Parse ] [ Form ] │
└────────────────────────────────────────────────────────────────────────────┘
 SUGGESTED
  ✓ Email Dr. Okafor · Tue ~3:10–3:40 PM · confidence ▮▮▯
      Lands in a likely-awake window · avoids your 2 PM appt · inside deadline
      [ Accept ]   [ Reject ]   [ Place myself ]
 UNPLACED
  ⊘ Tax documents (90m)
      We don't have a confident estimate that far out yet. We'll place it as
      the deadline nears, or you can place it now.   [ Place myself ]
```

### 10.5 Timeline — correction inspector

```
┌ Sleep block · Sat ──────────────────────────────────────────────────────┐
│ Source (wearable, directly observed):  1:10 AM → 9:30 AM   [show original]│
│ Effective (your correction):           2:00 AM → 9:30 AM                  │
│                                                                           │
│  This wasn't sleep:  [ quiet wakefulness ]  [ phone use ]  [ remove ]     │
│                                                                           │
│  Effect: your next predicted sleep moves ~25 min later.   [ Undo ]        │
└───────────────────────────────────────────────────────────────────────────┘
```

### 10.6 Trusted website — mobile, Family template

```
┌──────────────────────────┐
│  Maya — availability     │
│                          │
│  ● Probably resting now  │
│                          │
│  Best time to reach:     │
│  today ~7:00–9:30 PM     │
│  (approximate)           │
│                          │
│  [ Show next few days ]  │
│                          │
│  Urgent? Text first, then│
│  call. — Maya            │
│                          │
│  Estimated windows are   │
│  uncertain and are not   │
│  medical advice.         │
└──────────────────────────┘
```

### 10.7 Sharing — preview-before-create

```
┌ New share · Family ───────────────────────────────────────────────────────┐
│ Recipient can see:                                                          │
│   [✓] Best contact / wake window     [ ] Predicted sleep window             │
│   [✓] Availability (a few days)      [ ] Confidence level                   │
│ Expires:  [ in 30 days ▾ ]   Passcode: [✓] 4-digit                          │
│                                                                             │
│   PREVIEW (exactly what David will see):                                    │
│   ┌────────────────────────────┐                                           │
│   │ ● Probably resting now     │                                           │
│   │ Best: today ~7–9:30 PM     │                                           │
│   └────────────────────────────┘                                           │
│                                                                             │
│   [ Create link ]                          [ Cancel ]                       │
└────────────────────────────────────────────────────────────────────────────┘
```

---

## 11. Responsive behavior

Three product surfaces, distinct interaction models — *not* one responsive layout stretched
three ways.

**Breakpoints (apply to desktop React + trusted web; Android uses Compose window-size
classes):**

| Token | Range | Layout |
|---|---|---|
| `xs` | < 480 px | Single column; bottom bar (Android) / stacked (web). Trusted-web target. |
| `sm` | 480–767 px | Single column, larger touch targets. |
| `md` | 768–1023 px | Two-pane where useful (list + detail); desktop rail collapses to icons. |
| `lg` | 1024–1439 px | Desktop default: rail + content. |
| `xl` | ≥ 1440 px | Content max-width capped (~1200 px); never full-bleed dense. |

- **Desktop:** rail (lg+) → icon-rail (md) → top tabs (sm, e.g. narrow window). Status strip
  persists at all widths. Timeline actogram gains a second day-plot column at xl.
- **Android:** Compose `WindowSizeClass`. Compact = bottom bar. Medium/Expanded (tablet/fold)
  = navigation rail + two-pane Timeline/Tasks. Quick Log stays thumb-reachable.
- **Trusted web:** designed at `xs` first; on `md+` it simply centers a max-380 px column —
  it must never grow into a dashboard on a desktop browser.

---

## 12. Component inventory

Shared in spirit across platforms; implemented per-stack (React components, Compose
composables, plain web for trusted view). Names are the contract between this doc and code.

**Status & rhythm**
- `StatusBadge` — primary state (Awake/Asleep/Uncertain) + provenance tag.
- `TimeSinceWake` — civil-anchored relative duration.
- `NowStrip` — compact horizontal timeline (now-marker, current window, next sleep band).
- `PredictedWindowBand` — the widening uncertainty band (range, not point).
- `ConfidenceMeter` — ordinal low/med/high + reasons disclosure.
- `DriftIndicator` — "+50 min/cycle" with direction/trend.
- `ProvenanceTag` — source icon (acquisition) + confirmation marker (evidence).

**Scheduling**
- `BurdenChip` — ordinal low/moderate/high + reason.
- `Actogram` — multi-track, double-plot optional, civil-time.
- `CalendarGrid` — day/week/month civil grid with overlay slot.
- `TaskCard` — task + state (placed/suggested/unplaced/locked/moved).
- `ProposalCard` — window + `explanation_codes` reasons + confidence + accept/reject.
- `UnplacedNotice` — reason + single suggested lever.
- `CorrectionInspector` — source (read-only) vs effective (editable) + forecast diff.

**Sharing & privacy**
- `ShareProfileEditor` — per-field allowlist toggles + expiry + passcode.
- `TrustedViewPreview` — renders the real `trusted-view` DTO.
- `AccessLogList`, `ExpiryControl`, `RevokeButton` (confirm).

**Medication**
- `DoseQuickLog`, `DoseRow` (civil + relative + confidence + source), `ClinicianRuleNote`.

**Primitives**
- `Button` (primary/secondary/quiet/destructive), `Toggle`, `Segmented`, `Stepper`,
  `Field`, `Sheet`/`Dialog`, `Banner` (info/attention/offline), `EmptyState`,
  `RefusalPanel`, `UndoToast`, `SourceIcon`, `ConfirmDialog`.

---

## 13. Component states and variants

Universal state vocabulary (every data-bearing component must declare which it's in):

```
loading · empty · ok · low-confidence · inferred · confirmed · conflicting ·
stale · refused · offline · permission-revoked · paused · error
```

Key components:

- **`StatusBadge`**
  - Primary value ∈ `{Awake, Asleep, Uncertain}` (only three the user reads).
  - Provenance ∈ `{confirmed, inferred, clinician-set, locked}` rendered as a marker, not a
    color.
  - The 11 brief "product states" map to **secondary descriptor + provenance**, never to a
    new primary value:
    | Brief state | Primary | Descriptor / source |
    |---|---|---|
    | free-running | Awake/Asleep | "rhythm drifting later" (inferred) |
    | temporarily aligned | Awake/Asleep | "aligned to daytime for now" (inferred) |
    | attempting entrainment | Awake/Asleep | "following a plan" (clinician-set or user-set) |
    | possible loss of entrainment | Uncertain | "rhythm may be slipping" (inferred) |
    | forced appointment disruption | Awake | "schedule disrupted by appointment" (inferred) |
    | recovery | Awake/Asleep | "settling after disruption" (inferred) |
    | research baseline | any | "baseline logging" badge (user-set) |
    | insufficient data | Uncertain | "still learning" (system) |
    | conflicting data | Uncertain | "sources disagree" (system) + conflict glyph |
    | offline | (unchanged) | "offline — last updated …" banner |
    | permissions revoked | (unchanged) | "a source is disconnected" banner |
  - **The user never has to pick a state.** They may *lock* the primary (forces a known
    state and pauses inference, clearly labeled). Source tag always shows whether the state
    is inferred, user-set, or clinician-set.

- **`ConfidenceMeter`** — `low | medium | high` only (matches contract). Three filled
  segments + text label + reasons. **Never** a percentage, never a needle gauge.

- **`PredictedWindowBand`** — variants: confident (tighter, solid-ish), uncertain (wider,
  faded/hatched), refused (absent + inline note), stale (greyed + timestamp).

- **`ProposalCard`** — suggested · accepted · rejected · superseded. Reasons always present.

- **`BurdenChip`** — low/moderate/high; always carries reason; never red for "high" (uses
  attention-amber + label + icon).

---

## 14. Design tokens

Single source of truth, consumed by all three surfaces. Suggested file:
`contracts/` is for data; put tokens at `apps/shared-tokens/tokens.json` and generate:
CSS custom properties (desktop React + trusted web) and a Compose `Theme` (Android).

```jsonc
{
  "color": {
    "light": {
      "bg":            "#F7F5F1",   // warm paper, not stark white (less glare)
      "surface":       "#FFFFFF",
      "surface-sunken":"#EFEDE8",
      "ink":           "#1F2328",   // primary text
      "ink-secondary": "#5A6169",
      "ink-muted":     "#8A9099",
      "line":          "#DEDAD2",
      "accent":        "#2E6E6A",   // calm teal — the ONLY accent at rest
      "accent-ink":    "#1B4744",
      "attention":     "#9A6B16",   // amber — uncertainty / burden / needs-attention
      "attention-bg":  "#F6ECD6",
      "danger":        "#9B2C2C",   // RESERVED: errors + destructive confirm only
      "asleep":        "#445A7A",   // muted indigo for sleep bands
      "awake":         "#2E6E6A",
      "uncertain":     "#8A7CA8",   // soft violet, used WITH hatching, never alone
      "focus-ring":    "#1457C9"
    },
    "dark": {
      "bg":            "#14171A",   // soft charcoal, not pure black (less halation)
      "surface":       "#1C2024",
      "surface-sunken":"#101316",
      "ink":           "#E7E9EC",
      "ink-secondary": "#AEB4BB",
      "ink-muted":     "#7C838B",
      "line":          "#2C3238",
      "accent":        "#6FC2BB",
      "accent-ink":    "#BFEAE5",
      "attention":     "#E0B560",
      "attention-bg":  "#33291300",
      "danger":        "#E8847E",
      "asleep":        "#8AA0C2",
      "awake":         "#6FC2BB",
      "uncertain":     "#B3A6CE",
      "focus-ring":    "#7FA8FF"
    }
  },
  "space":  { "0":"0", "1":"4px","2":"8px","3":"12px","4":"16px","5":"24px","6":"32px","7":"48px","8":"64px" },
  "radius": { "sm":"6px","md":"10px","lg":"16px","pill":"999px" },
  "elevation": {
    "0":"none",
    "1":"0 1px 2px rgba(20,23,26,.06)",
    "2":"0 2px 8px rgba(20,23,26,.08)",
    "3":"0 8px 24px rgba(20,23,26,.10)"   // dialogs/sheets only
  },
  "type": {
    "family-sans": "Inter, 'Segoe UI Variable', system-ui, sans-serif",
    "family-mono": "'IBM Plex Mono', ui-monospace, monospace",
    "scale": { "xs":"12px","sm":"14px","base":"16px","lg":"20px","xl":"26px","2xl":"34px","3xl":"44px" },
    "leading": { "tight":"1.2","normal":"1.5","loose":"1.65" },
    "weight": { "regular":"400","medium":"500","semibold":"600" }
  },
  "motion": {
    "fast":"120ms","base":"200ms","slow":"320ms",
    "ease":"cubic-bezier(.2,.0,.2,1)",
    "reduced":"0ms"     // reduced-motion → instant, no transform animation
  },
  "touch": { "min":"44px", "comfortable":"48px" },
  "uncertainty": {
    "band-opacity-confident":"0.85",
    "band-opacity-uncertain":"0.45",
    "hatch-angle":"45deg",
    "hatch-gap":"6px"
  },
  "z": { "content":"0","strip":"10","sheet":"100","dialog":"200","toast":"300" }
}
```

Token rules:
- **Exactly one accent at rest.** Amber = attention/uncertainty; danger-red is *only* errors
  and destructive confirmations. A "high burden" or "late" state never uses danger-red.
- Light theme uses **warm paper, not pure white**; dark theme uses **soft charcoal, not pure
  black** — both reduce glare/halation for fatigued or photophobic users without assuming
  light is a trigger.

---

## 15. Typography and icons

- **Sans:** Inter (or Segoe UI Variable on Windows for native feel). Base **16 px**, body
  line-height **1.5**. The Overview status line is `2xl–3xl` (34–44 px).
- **Mono:** IBM Plex Mono for timestamps, durations, and numeric drift, so digits align and
  don't "jump" as values change (reduces re-reading cost when tired).
- **Numerals:** tabular figures everywhere a value updates.
- **Min body size:** never below 14 px; supports OS font scaling to ≥ 200% without clipping
  (text reflows, no fixed-height clipping).
- **Icon style:** single-weight (1.75 px stroke), rounded joins, geometric, no fills, no
  duotone, no glow. 24 px default, 20 px dense, 28 px touch. Icons **always** accompany a
  text label for status-bearing meaning (never icon-only for state).
- **Source icons (acquisition_method):** distinct glyphs for `manual` (pencil),
  `health_connect` (heart-link), `os_activity` (monitor/phone), `file_import` (file),
  `synthetic` (beaker — used only in `fixtureMode`).
- **Confirmation markers (evidence_status):** anchor = `directly_observed`, person =
  `user_reported`, dotted-circle = `inferred`.

---

## 16. Light and dark themes

- Both ship at parity (no feature is light-only).
- **Auto / Light / Dark** + an independent **Reduce stimulation** toggle (see §18) — dark is
  *not* assumed to be the calm mode.
- Contrast: body text ≥ 4.5:1, large text/UI ≥ 3:1 in both themes (the token palette above
  is chosen to clear this). Sleep/wake/uncertain band colors are differentiated by **hue +
  pattern + label**, so they survive grayscale and color-blindness.
- Dark theme avoids pure-black surfaces and pure-white text (`#E7E9EC` on `#14171A`) to cut
  halation that worsens reading for migraine-prone users.
- Elevation in dark theme is conveyed by **lighter surface + hairline**, not heavy shadow.

---

## 17. Uncertainty visualization system

This is the product's spine. Uncertainty must be visible **without relying on color** and
must never read as false precision.

### 17.1 The five things we must show

| Concept | Encoding (redundant, non-color-only) | Plain label |
|---|---|---|
| **Confidence: high** | full 3-seg meter, tighter band, solid edge | "high confidence" |
| **Confidence: moderate** | 2 segs, medium band, soft edge | "medium confidence" |
| **Confidence: low** | 1 seg, wide hatched band, dashed edge | "low confidence" |
| **Widening future** | band visibly **fans out** with horizon (right side wider) | "less certain further ahead" |
| **Conflicting sensors** | split two-tone block + ⚠-conflict glyph + count | "2 sources disagree" |
| **Confirmed vs inferred** | anchor marker vs dotted-circle marker | "confirmed" / "estimated" |
| **Insufficient data** | neutral grey placeholder, no band | "not enough data yet" |
| **Stale** | desaturated + timestamp | "last updated 6 h ago" |

### 17.2 Toolkit
- **Ranges, never points.** A predicted sleep "time" is always a band
  (`earliest_at..latest_at`), labeled "~9:40–10:40 PM," never "9:53 PM."
- **Hatching/fading** for uncertainty (works in grayscale, for color-blind users, and on
  cheap phone screens).
- **Plain-language labels** accompany every visual ("approximate," "estimated," "we're not
  sure yet").
- **Source icons + confirmation markers** on every data block.
- **Short "why"** disclosure backed by `confidence.reasons` from the contract.

### 17.3 Good vs bad

```
GOOD:  Next sleep:  ~9:40–10:40 PM   ▮▮▯ medium   “timing has drifted ~50 min/day”
       [band fans wider toward tomorrow]

BAD:   Next sleep:  9:53 PM           ← false precision, single number
BAD:   Sleep score: 87 / 100          ← invented metric, gamified, judgmental
BAD:   ●●●●○ 80% confidence           ← fake percentage we never computed
BAD:   [solid red] You'll be asleep!  ← color-only + alarmist + overclaiming
GOOD:  ◐ Uncertain — likely in transition (sources disagree) [why?]
```

### 17.4 Hard rules
- Never upgrade a `refused` estimate into a guessed line "to fill the space."
- Never animate a band tightening to imply growing certainty it doesn't have.
- A point-like value is allowed **only** for civil-time facts the app *knows* (the current
  clock, a logged dose timestamp, a fixed appointment) — never for predictions.

---

## 18. Accessibility specification

Target **WCAG 2.2 AA**. This product's users are disproportionately likely to be fatigued,
photophobic, or cognitively impaired *at the moment of use*, so accessibility is core, not
polish.

**Non-visual use is a primary mode.** A large share of people with Non-24 are totally blind
(the rhythm free-runs without light reaching the circadian clock), so screen-reader + keyboard
operation is a core path, not an accommodation. Every screen here must be fully usable with no
sighted vision — the commitment and non-negotiables live in [`accessibility.md`](accessibility.md),
and the blind co-primary persona is §3.6.

- **Keyboard (desktop):** full operability; logical focus order top-to-bottom matching
  visual hierarchy; visible focus ring (`focus-ring` token, ≥ 3:1); `Esc` closes
  sheets/dialogs; no keyboard traps; shortcuts for Confirm-wake and Log-dose.
- **Screen readers (primary path):** semantic roles + an accessible name on every control,
  icon-only button, tab, and input. **Every chart/visual ships a table or text equivalent** —
  actogram, drift chart, calendar overlay, confidence meter, status dot — e.g. "Sat: sleep
  2:00 AM–9:30 AM, source wearable, corrected"; a chart with no text equivalent is incomplete.
  Meaningful state changes (current rhythm state, an approval applied, a refusal) announce via
  `aria-live="polite"` without stealing focus, and civil times are spoken explicitly (never
  inferred from chart position). Android: full TalkBack labels, `stateDescription` for status,
  content descriptions for every source/confirmation marker. Detail: [`accessibility.md`](accessibility.md).
- **Non-color status:** every status carries icon + text + (often) shape/pattern. The app
  is fully usable in grayscale.
- **Contrast & scaling:** ≥ 4.5:1 text; supports OS font scale to 200%+ with reflow; no
  fixed-height text clipping; respects Windows/Android display scaling.
- **Touch targets:** ≥ 44×44 px (`touch.min`); primary Android actions 48 px; spacing
  prevents mis-taps for tremor/fatigue.
- **One-handed Android:** primary actions in the lower-third; Quick Log center; destructive
  actions never in the thumb-swipe path.
- **Motion sensitivity:** honor `prefers-reduced-motion` / Android "remove animations" →
  `motion.reduced` (instant, no transform/parallax). No essential information is conveyed by
  motion alone.
- **Reduced-stimulation / "Migraine mode":** an independent toggle (and quick-access in
  tray/Quick Log) that composes several reductions the user can also set individually:
  - lower contrast intensity (not lower legibility),
  - reduce color saturation toward near-monochrome,
  - increase whitespace / reduce density,
  - disable all non-essential motion,
  - dim non-critical chrome.
  **It does not force dark mode and does not assume bright light is the trigger** — the user
  chooses light or dark within reduced-stimulation.
- **Fatigue-error tolerance:** destructive or externally-visible actions (revoke? no —
  *create* share, delete data, export) require explicit confirmation; everything reversible
  offers Undo; the app never auto-submits on blur; no time-pressured dialogs.
- **Plain language:** reading level kept low; no jargon ("DLMO," "actogram") in primary UI —
  jargon allowed only in Reports/clinician contexts, with a glossary link.

**Accessibility acceptance criteria** are enumerated in §26.

---

## 19. Privacy and sharing UX

This is where the design is most opinionated, because the trusted-web link is the only
exposed surface and Non-24 availability data is health-revealing.

### 19.1 Principles in practice
- **Default-deny everywhere** (matches `share-profile` defaults: all permissions `false`,
  and ADR-0003). A new profile shows the recipient *nothing* but the urgent note until the
  user opts each field in.
- **Minimization is structural, not cosmetic:** the trusted view is the `trusted-view.json`
  DTO, which by construction excludes identifiers, provenance, location, zone ID, raw
  observations, health detail, and calendar text. The web app **cannot** request more — it
  only renders what it's given.
- **Expiry is mandatory.** No indefinite links by default; expiry is a required field with a
  conservative default (e.g., 30 days; collaborator template shorter).
- **Passcode option** for any link; recommended (and defaulted on) when `availability`
  (multi-day) is granted.

### 19.2 The leaked-link threat → safe degradation
- The token is an opaque capability in the path; **no user identity in the URL**.
- Expired and revoked render **identically** ("link unavailable / expired — contact {name}
  directly") so a leaker can't tell whether they were specifically cut off.
- **Multi-day availability is the highest-risk grant** (it can reveal the drift pattern).
  Therefore it is: opt-in, passcode-gated by default, **horizon-limited** (e.g., max 3–5
  days), and **coarsened** to broad windows — never minute-precise, never showing the
  underlying nightly drift slope. We **do not** add fake jitter (that would mislead a
  well-meaning recipient); we reduce resolution honestly instead.
- **Preview is mandatory before creating** any link: the user sees the *exact* recipient
  view (the rendered DTO), so there are no surprises.
- **Access log** shows when (not detailed who) a link was opened; one-tap **revoke** is
  always present and takes effect immediately.

### 19.3 What clinicians get
The public trusted link is **not** the clinician's detail channel. Clinicians receive depth
(actogram, drift, medication context, provenance) only through **Reports/Export** that the
user generates and hands over deliberately — keeping the network-exposed surface minimal.

---

## 20. Error, offline, and missing-data states

Refusals and gaps are **designed states**, never blank screens or red toasts.

| Situation | What the UI does | Copy tone |
|---|---|---|
| **Estimate refused** (`insufficient_data`) | Overview swaps facts for a "still learning" panel + what-helps actions | calm, instructive |
| **Estimate refused** (`conflicting_observations`) | Uncertain status + "sources disagree" + link to Data Sources conflict | neutral |
| **Estimate refused** (`ambiguous_cycle_index`) | "We can't line up your cycles confidently right now" + manual confirm prompt | neutral |
| **Too uncertain to auto-schedule** | Tasks shows `estimate_unavailable`/`outside_forecast_horizon` notice; offers manual placement | honest |
| **Offline** | Banner "Offline — last updated {time}"; cached values shown but marked stale; no spinners that imply live | reassuring |
| **Health Connect unavailable / < API 28** | "Your device can't share this. Add data manually or connect a wearable." | non-blaming |
| **Android permission revoked** | Data Sources row → "disconnected"; Overview banner "a source is paused"; one-tap re-grant | non-blaming |
| **Windows agent stopped** | Data Sources shows desktop activity stale; status leans on other sources or goes Uncertain | factual |
| **All devices idle but user awake** | App does **not** assert "asleep"; shows "looks quiet — are you up?" with Confirm-awake | gentle, never presumptuous |
| **Wearable false sleep** | Timeline flags reject-able block; correcting it is one step (see §9.5) | supportive |
| **Forecast changed a lot after a correction** | Explicit before/after diff + "you can undo this" | transparent |
| **Appointment in predicted sleep, can't move** | Support mode: gentle reminder + "this is information, not a failure" | compassionate |
| **Impossible task deadline** | `invalid_constraints`/`no_available_interval` + the single lever to relax | practical |
| **Long time since sleep** | Neutral note "you've been up ~22 h" — **no alarm, no medical claim**, no red | strictly neutral |

Global error rules: never show a stack trace; never lose user input; never present stale as
live; the trusted-web error page is contentless and safe.

---

## 21. Microcopy examples

**Status**
- ✅ "Likely awake — confirmed · 4 h 12 m since you woke"
- ✅ "Uncertain — likely in transition" / "Still learning your rhythm"
- ❌ "You are awake." (overclaims) · ❌ "Sleep debt: 3.2 h" (invented + judgmental)

**Forecast**
- ✅ "Next sleep likely ~9:40–10:40 PM (approximate)."
- ❌ "You will sleep at 9:53 PM."

**Tasks**
- ✅ "Suggested Tue ~3:10 PM — lands in a likely-awake window, avoids your 2 PM appointment."
- ✅ "We're not guessing this far out yet. We'll place it as the deadline nears."
- ❌ "Optimal productivity slot detected!" · ❌ "You missed your task window."

**Medication**
- ✅ "Logged 3:48 PM · 4 h after wake · ~6 h before predicted sleep."
- ✅ "We couldn't tie this dose to a wake or sleep window." (when confidence unknown)
- ❌ Anything implying a dose was right/wrong/late in a medical sense; ❌ red styling on timing.

**Sharing**
- ✅ "David will see only this. Nothing else leaves your device."
- ✅ "This link has expired. Contact Maya directly." (same for revoked)
- ❌ "Share your sleep data!" (it isn't sleep data; it's coarse availability)

**Disruption / compassion**
- ✅ "This appointment overlaps your predicted sleep. That's information, not a failure —
  want a gentle reminder?"
- ❌ "Warning: schedule conflict!" with red.

**Empty/insufficient**
- ✅ "Nothing to show yet. Confirm last night's wake to get started."
- ❌ "No data." (dead end)

Use the contract's fixed notice verbatim wherever a medical disclaimer is needed:
> "Estimated windows are uncertain and are not medical advice."

---

## 22. Notification strategy

Default posture: **quiet, few, never punitive, never during predicted sleep unless the user
explicitly asked.**

- **Categories (each independently toggleable, all off-able):**
  1. **Window-opening** (opt-in): "A good window for {task} is starting." Quiet, no badge
     spam.
  2. **Gentle pre-event** (opt-in per event): for appointments, especially ones in/near
     predicted sleep.
  3. **Dose reminders** (opt-in, only if the user set a schedule — the app invents none):
     "Reminder you set: {label}." Phrased as the user's own reminder, never as medical
     instruction.
  4. **Data attention** (low priority): "A source disconnected." Never urgent-styled.
  5. **Share activity** (optional): "Your family link was opened" / "expires tomorrow."
- **Quiet by rhythm:** notifications respect the *predicted sleep window*, not civil night —
  the app suppresses non-urgent alerts during likely sleep and offers a "catch me when I
  wake" digest.
- **No streaks, no "you haven't logged today," no nagging, no re-engagement growth hacks.**
- **Android:** uses notification channels mapped to the categories above so the OS gives the
  user native per-category control; respects Do Not Disturb; never full-screen/high-priority
  except a user-defined urgent case.
- **Desktop/tray:** status reflected passively in the tray; active notifications are rare and
  low-priority by default.

---

## 23. Usability-testing plan

Constraint from `AGENTS.md`/roadmap: research uses **synthetic or participant-controlled
data only**, conservative language, no health-data collection by the test harness.

- **Phase A — comprehension (unmoderated, 5–8 users incl. lived-experience Non-24):**
  - 3-second test on Overview: "What does this screen tell you right now?" Target: ≥ 80%
    correctly state awake/asleep/uncertain + that it's an estimate.
  - Uncertainty read: show a low-confidence forecast; ask "how sure is the app?" Target: no
    one reads it as exact.
- **Phase B — fatigue simulation (moderated):** ask participants to complete core tasks
  under cognitive load (e.g., late session, dual-task distraction) — confirm wake, log a
  dose, reject false sleep, find next good window. Measure errors and recovery (undo use).
- **Phase C — sharing safety (moderated, the user + a stand-in "trusted person"):**
  - Can the user predict exactly what the recipient sees *before* creating the link? (Preview
    efficacy.) Target: 100% no surprise post-create.
  - Recipient comprehension on a phone in < 10 s. Can they tell if "now" is a bad time?
  - Leak scenario walkthrough: does the user understand expiry/revoke?
- **Phase D — clinician review (1–3 clinicians):** can they extract drift trend + medication
  timing context from Reports without the app implying diagnosis/dosing?
- **Metrics:** task success, time-on-task under fatigue, comprehension accuracy, undo rate
  (high undo on automation = explanation failure), and a "felt judged?" Likert (target:
  strongly disagree).
- **Red-flag triggers (stop-ship):** any participant believes the app diagnosed them,
  prescribed, or guaranteed a time; any participant exposes more in a share than they
  intended.

---

## 24. MVP vs later prioritization

Aligned to the repo roadmap (phase one scaffold → phase two usability → phase three
interop).

**MVP (phase one/early two) — the honest core loop:**
- Overview (status, time-since-wake, predicted next sleep band, confidence, next event,
  data warning) — fed by existing `OverviewDTO`.
- Manual corrections + confirm-wake (non-destructive) — the trust anchor.
- Timeline (read + correct), single/multi source.
- Tasks with proposals + explanation codes + unplaced reasons + undo/lock.
- Calendar week/day with predicted overlay + burden.
- Medications quick log (civil + relative timing, no judgment).
- Sharing with default-deny, preview, expiry, revoke; trusted-web rendering
  `trusted-view.json`.
- Data Sources status + conflicts.
- Full uncertainty system, accessibility AA, light/dark + reduced-stimulation.

**Later (phase two/three):**
- Natural-language task capture (structured form is MVP; NL is enhancement).
- Month calendar burden view; double-plotted actogram refinements.
- Reports/Export polish, clinician summary, redaction controls.
- Travel/multi-zone richness; DST edge polish.
- Access-log detail; passcode flows hardening.
- Android tablet/fold two-pane; sync (explicitly deferred per roadmap).
- Any relay/remote sharing — only after separate security review (roadmap phase three).

**Explicitly deferred / out of scope (do not build):** exact circadian phase/DLMO,
autonomous health recommendations, background hidden collection, cloud upload by default,
gamification.

---

## 25. Design risks and bad ideas to avoid

Stated bluntly, as the brief demands.

1. **Cockpit Overview.** The `OverviewDTO` has ~10 fields; rendering all as equal cards
   creates a dashboard a tired user can't parse. **Mitigation:** strict hierarchy (§9.2),
   ≤ 3 quiet facts visible at rest, one accent color.
2. **State soup.** Eleven surfaced states will read as instability. **Mitigation:** 3 primary
   values + descriptors (§13).
3. **False precision creep.** The single biggest safety risk: a band quietly becoming a line,
   or confidence becoming a percentage. **Mitigation:** §17.4 hard rules; ranges only;
   ordinal confidence only.
4. **Trusted-link over-share.** Multi-day availability can leak the diagnosis-revealing drift
   pattern. **Mitigation:** opt-in, passcode, horizon-limit, coarsen, identical
   expired/revoked page (§19).
5. **Punitive medication styling.** Red "late dose" UI would imply medical judgment the app
   must not make. **Mitigation:** neutral ink; clinician rules attributed, not enforced
   (§9.6).
6. **Over-automation.** Auto-moving fixed events, or scheduling into an untrusted forecast.
   **Mitigation:** fixed events immutable; refusal is first-class; automation explains +
   undoes (§8.4, §9.4).
7. **Destructive corrections.** Overwriting source data on edit would break auditability and
   trust. **Mitigation:** append-only corrections; "show original" (§9.5).
8. **"Dark = calm" assumption.** Forcing dark for migraine mode is wrong for many.
   **Mitigation:** reduced-stimulation is independent of theme (§18).
9. **Notification nagging / streaks.** Re-engagement mechanics are actively harmful here.
   **Mitigation:** quiet, opt-in, rhythm-aware, no streaks (§22).
10. **Civil-time loss.** If rhythm-relative time ever replaces the clock, users get
    disoriented. **Mitigation:** civil time always primary (principle 2).
11. **Pretending to know when idle.** Treating "all devices idle" as "asleep" produces false
    confident wrong states. **Mitigation:** ask, don't assert (§20).
12. **Decorative charts.** Charts with no decision attached add load. **Mitigation:** every
    chart maps to an action or an answer.

---

## 26. Codex implementation handoff

Concrete, buildable. Where a value comes from a contract, the contract path is named.

### 26.1 Screen list

| ID | Surface(s) | Route / location |
|---|---|---|
| `onboarding` | desktop, android | first-run flow |
| `overview` | desktop, android(home) | `/overview`, Android Home |
| `calendar` | desktop, (android read-only later) | `/calendar` |
| `tasks` | desktop, android | `/tasks` |
| `timeline` | desktop, android | `/timeline` |
| `medications` | desktop, android(quick log + more) | `/medications` |
| `sharing` | desktop, android(more) | `/sharing` |
| `data-sources` | desktop, android(more) | `/data-sources` |
| `reports` | desktop | `/reports` |
| `settings` | desktop, android(more) | `/settings` |
| `quick-log` | android | center tab |
| `trusted-view` | web | `/v/{token}` |

### 26.2 Navigation map

```
DESKTOP (left rail): Overview · Calendar · Tasks · Timeline · Medications ·
                     Sharing · Data Sources · Reports · Settings   (+ persistent status strip + tray)
ANDROID (bottom):    Home · Timeline · [Quick Log] · Tasks · More{Medications, Sharing, Data Sources, Reports, Settings}
WEB:                 single route /v/{token}  → {valid | passcode | expired/revoked | invalid}
```

### 26.3 Reusable components & inputs

Inputs reference contract fields directly. `?` = optional.

```
StatusBadge(primary: 'awake'|'asleep'|'uncertain',
            provenance: 'confirmed'|'inferred'|'clinician-set'|'locked',
            descriptor?: string)
TimeSinceWake(wakeAt: timestamp, confirmed: boolean, now: timestamp)
NowStrip(now, currentWindow: {start,end}, nextSleep: uncertainWindow)
PredictedWindowBand(window: uncertainWindow, confidence: 'low'|'medium'|'high', horizonDays: number)
ConfidenceMeter(level: 'low'|'medium'|'high', reasons: string[])      // common.schema#confidence
DriftIndicator(minutesPerCycle: number, trend?: 'later'|'earlier'|'settling')
ProvenanceTag(acquisition: 'manual'|'health_connect'|'os_activity'|'file_import'|'synthetic',
              evidence: 'directly_observed'|'user_reported'|'inferred')
BurdenChip(level: 'low'|'moderate'|'high', reason: string)
ProposalCard(window:{start,end}, confidence, explanationCodes: Array<
   'within_predicted_waking_window'|'avoids_fixed_event'|'within_task_bounds'|'uncertainty_buffer_applied'>,
   onAccept, onReject, onPlaceManually)
UnplacedNotice(reason: 'no_available_interval'|'outside_forecast_horizon'|'estimate_unavailable'|'invalid_constraints')
TaskCard(task:{title,durationMinutes,earliestAt,latestAt,
   preference:'predicted_waking_window'|'daytime'|'any_available'},
   state:'placed'|'suggested'|'unplaced'|'locked'|'moved', demand?:'low'|'med'|'high')
Actogram(tracks: Track[], range:{start,end}, doublePlot?: boolean)   // exposes text alternative
CorrectionInspector(source:{start,end,readonly:true}, effective:{start,end},
   onCorrect(kind:'sleep_start'|'sleep_end'|'nap'|'quiet_wake'|'reject_sleep'|'confirm_wake'),
   forecastDiff?: string)
DoseRow(label, civilAt, timeSinceWake?, timeBeforePredictedSleep?, confidence:'low'|'medium'|'high'|'unknown',
   source: string, clinicianRule?: string)
ShareProfileEditor(permissions:{predicted_sleep_window,predicted_waking_window,confidence,availability}=all false,
   expiresAt(required), passcode?: boolean, template:'family'|'friend'|'clinician'|'collaborator')
TrustedViewPreview(view: trusted-view.json)
RefusalPanel(code:'insufficient_data'|'ambiguous_cycle_index'|'conflicting_observations'|'unsupported_input',
   message: string, helpfulActions: Action[])
Banner(kind:'info'|'attention'|'offline'|'fixture')
UndoToast(message, onUndo)
ConfirmDialog(action, destructive?: boolean, externallyVisible?: boolean)
```

### 26.4 Component states (universal enum)

`loading · empty · ok · low-confidence · inferred · confirmed · conflicting · stale ·
refused · offline · permission-revoked · paused · error` — every data component declares one.

### 26.5 Responsive breakpoints

`xs<480 · sm 480–767 · md 768–1023 · lg 1024–1439 · xl≥1440` (web/desktop).
Android uses Compose `WindowSizeClass` {Compact, Medium, Expanded}. Trusted web is `xs`-first,
max content width 380 px even on large screens.

### 26.6 Android-specific variations
- Bottom nav (5), Quick Log centered & thumb-reachable; Settings under More.
- Compose `WindowSizeClass`; Medium/Expanded → nav rail + two-pane Timeline/Tasks.
- Notification **channels** map 1:1 to §22 categories; respect DND.
- Health Connect gated behind availability + permission boundary, **API 28+**; API 26
  fixture repositories must run with no live data (matches architecture).
- TalkBack labels + `stateDescription` for status; reduced-motion honors system setting.
- Android does **not** reimplement estimation; consumes contracts/repositories.

### 26.7 Desktop-specific variations
- Persistent status strip (mirrors `OverviewDTO.CurrentEstimatedState`+`TimeSinceWake`).
- System-tray with status line + quick actions; `HideWindow` exists but tray reopen path is
  deferred (per `app.go` comment) — design tray menu to not depend on it yet.
- `fixtureMode:true` from `OverviewDTO` → show a non-alarming "Sample data" banner
  (synthetic source/beaker icon), never styled as an error.
- Keyboard shortcuts; mouse + touch; window down to `sm` shows top tabs instead of rail.

### 26.8 Trusted-site variations
- Renders **only** `trusted-view.json`; cannot fetch private domain.
- Blocks shown strictly per `granted_fields ⊆ {predicted_sleep_window, predicted_waking_window,
  confidence, availability}`.
- Always renders the exact `notice` string.
- Expired and revoked render identical "link unavailable" content.
- `availability` (multi-day) coarsened + horizon-capped; passcode default-on when granted.

### 26.9 Accessibility acceptance criteria (testable)
1. All text ≥ 4.5:1 (large/UI ≥ 3:1) in light and dark; verified by automated contrast check.
2. Every status conveyed by text + icon (and pattern where a band) — passes a grayscale render
   test (no information lost).
3. Full keyboard operability on desktop; visible focus on every interactive element; no traps;
   `Esc` closes overlays.
4. Status badge changes announced via `aria-live="polite"` (web) / `stateDescription` (Compose)
   without focus theft.
5. Actogram exposes an equivalent text/table alternative readable by SR.
6. All touch targets ≥ 44×44 px; primary Android actions ≥ 48 px.
7. `prefers-reduced-motion` / Android remove-animations → zero transform animation; no
   info conveyed by motion alone.
8. Reduced-stimulation toggle works in **both** light and dark; does not force a theme.
9. OS font scaling to 200% reflows without clipping or horizontal scroll of body text.
10. Every destructive or externally-visible action (delete data, create/expand a share,
    export) requires explicit confirmation; every reversible action offers Undo.
11. No primary-UI use of clinical jargon ("DLMO," "actogram") without a plain-language label
    or glossary link.

### 26.10 Suggested fixture data
Use the repo's synthetic fixtures verbatim; they already cover the core cases:
- `testdata/v1/phase-estimate.json` — `status:"estimated"`, drift +50 min/cycle, median 480
  min, **medium** confidence, two forecast cycles → drives Overview + NowStrip + bands.
- Build a refusal fixture (`status:"refused"`, code `insufficient_data`) → Overview
  "still learning" + Tasks `estimate_unavailable`.
- `testdata/v1/schedule-proposals.json` — proposals + `unplaced` → Tasks screen (cover each
  explanation + unplaced reason code).
- `testdata/v1/observations.json` + `corrections.json` → Timeline (source vs effective,
  faded original, conflict case).
- `testdata/v1/trusted-view.json` (granted: sleep+wake+confidence) and
  `trusted-view-default-deny.json` (nothing granted) → web `valid` vs minimal.
- `testdata/v1/share-profile-allowlisted.json` + `share-profile-default-deny.json` → Sharing
  editor states + preview.
- Medication fixture from `OverviewDTO.MedicationEvents` (dose 2 h after wake) → Medications
  + relative-timing-unknown variant (no anchor).
- `OverviewDTO.fixtureMode = true` → "Sample data" banner on desktop.
- **Add edge fixtures** for: conflicting sources (Timeline conflict glyph), stale source
  (Data Sources degraded), zone change (Calendar dual-zone), offline (cached + stale banner).

### 26.11 Testable UI acceptance criteria
1. Overview renders the correct primary status from `currentEstimatedState` and never shows a
   point time for any prediction (assert predictions render as ranges).
2. `ConfidenceMeter` renders exactly the contract level (`low|medium|high`) and shows
   `confidence.reasons`; no percentage string ever appears.
3. A `refused` estimate produces `RefusalPanel` with the right code + helpful actions, and
   **no** forecast band is drawn.
4. `ProposalCard` shows one human reason per present `explanation_code`; `UnplacedNotice`
   shows the mapped reason + exactly one suggested lever.
5. Editing a Timeline block creates a correction; the original remains visible via "show
   original"; a forecast-diff string and Undo are present (no source mutation).
6. Sharing preview output is byte-equivalent to the `trusted-view` DTO that the link will
   serve for the same profile (preview == reality).
7. Trusted web renders only blocks whose key is in `granted_fields`; with default-deny it
   shows only the urgent note + `notice`.
8. Expired and revoked trusted views render identical markup/text.
9. Medication timing differences render in neutral (non-danger) color; danger color appears
   only on genuine errors and destructive-confirm dialogs (assert no `danger` token on
   medication/burden components).
10. Fixed events expose no edit/move affordance from scheduling; alternative-time suggestions
    are copy-out only.
11. With `fixtureMode:true`, the "Sample data" banner is present and uses the synthetic source
    icon, not an error style.
12. All status colors pass a grayscale snapshot test (state still identifiable).

### 26.12 Unresolved product questions
1. **Trusted-link transport.** Phase-one web is a *static fixture* with no live endpoint
   (`OverviewDTO.SharingStatus` says so). How does a real link get a fresh `trusted-view.json`
   without a server? (Roadmap defers relay to phase three pending security review.) Until
   then, what exactly does "create link" do — local file? QR of a signed snapshot? **Needs
   product+security decision; UI should present sharing honestly as not-yet-live in MVP.**
2. **Locking the current state** — does locking pause inference entirely, or only freeze the
   displayed badge while estimation continues underneath? (Affects copy + Timeline behavior.)
3. **Cognitive/physical demand on tasks** — UI-level only, or should it enter the
   `schedule-request` contract? Currently no contract field exists; scheduler can't honor it
   yet. **Either add to contract or label it as advisory-only in UI.**
4. **Burden score derivation** — `low/moderate/high` needs a defined, explainable rule
   (overlap × confidence?) so the "why" copy is truthful. Owner: core.
5. **Multi-day availability coarsening parameters** — exact horizon cap and rounding for the
   trusted view (privacy-critical). Needs a number + threat review.
6. **Natural-language task parsing** — deferred to phase two; confirm structured form is the
   MVP contract and NL is additive.
7. **Wake-anchor confidence in UI** — `medication` core can attach `ConfidenceUnknown`; confirm
   the copy for "couldn't tie to a wake/sleep window" and where it surfaces beyond Medications.
8. **Notification timing source** — suppress by *predicted sleep window* requires a live
   estimate; define behavior when the estimate is refused (fall back to civil-night quiet hours?).

---

*End of specification. This document should evolve with the v1 contracts; if a screen needs a
field the contract doesn't expose, raise a contract change (and ADR) rather than inventing UI
data.*
