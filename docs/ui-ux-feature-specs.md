# ZeitBoard — Feature UI/UX Specifications

**Calendar Sync + Approval Queue · Sleep Chart Visualizer · Conversational Assistant**

Companion to [`ui-ux-design.md`](ui-ux-design.md) (master product design) and
[`roadmap.md`](roadmap.md). This document is written for direct implementation.
It is **placement-first**: every component carries an explicit `Placement` block
and a measured wireframe, because layout is where interpretation goes wrong.

> Read §1 before building anything. It defines the coordinate system, units, and
> the exact zones every later component refers to. Do not place an element "near"
> or "beside" something — place it in a named zone, at a grid span or pixel size,
> with margins given in spacing tokens.

---

## 0. Source-of-truth values (use these literally)

These are the **implemented** desktop values from `apps/desktop/frontend/src/styles.css`.
New screens MUST reuse them so they visually match the existing app. Do not invent
new colors or radii.

```
Color (CSS vars already defined in :root)
  --ink #24302d   --muted #68736f   --subtle #8c9591   --line #dedfd8
  --paper #fffefa --canvas #f5f3ee
  --sage #55766b  --sage-dark #3e5f55  --sage-soft #e4ece7
  --blue-soft #e9eff2   --amber-soft #f5ecda
  --shadow 0 12px 30px rgba(39,52,48,.06)
Confidence (existing classes)
  low    text #714d4a  bg #f1e1df
  medium text #695528  bg #f5ecda   (amber = uncertainty/attention)
  high   text #315c49  bg #e4ece7
Status tones (from ui-ux-design.md): awake=--sage · asleep #445a7a · uncertain #8a7ca8
Panel: background --paper; border 1px solid --line; border-radius 11px; box-shadow --shadow
Spacing scale (px): 4, 8, 12, 16, 20, 24, 32, 48, 64   (refer to these as s4…s64)
Type scale (px): 10(kicker) 11 12 13 14 16 19 26 34 44
Reserved colors: red (#9b2c2c family) ONLY for errors + destructive confirms.
```

New tokens introduced by this document (add to `:root`):

```
--rail-right: 360px;     /* width of the right utility rail (assistant / queue) */
--rail-left: 232px;      /* existing left nav rail width, named for reference */
--row-pitch: 28px;       /* actogram day-row pitch */
--z-strip: 10; --z-rail: 20; --z-scrim: 90; --z-drawer: 100; --z-dialog: 200; --z-toast: 300;
```

---

## 1. Layout & placement system

### 1.1 Desktop zones

The desktop is **three vertical zones**. Zones Z1 and Z2 exist today; Z3 is new.

```
 viewport width →
┌───────────┬───────────────────────────────────────┬──────────────────┐
│ Z1 nav    │ Z2 main content                        │ Z3 utility rail  │
│ rail      │ (PageHeader + .screen-grid)            │ (assistant /     │
│ 232px     │ fluid, min 600px                       │  compact queue)  │
│ fixed     │                                        │ 360px, toggle    │
└───────────┴───────────────────────────────────────┴──────────────────┘
   --rail-left            minmax(600px, 1fr)               --rail-right
```

`.app-shell` grid columns:

| State | `grid-template-columns` |
|---|---|
| Rail closed (default, today) | `232px minmax(0, 1fr)` |
| Rail open, viewport ≥ 1200px | `232px minmax(600px, 1fr) 360px` |
| Rail open, viewport < 1200px | `232px minmax(0, 1fr)` **+ rail becomes an overlay drawer** (Z3 leaves the grid; see §1.5) |

**Z2 main content** keeps the existing `.main-content` rules: `max-width:1440px;
padding:38px clamp(28px,4vw,64px) 60px;`. All screen bodies render **inside** a
`.screen-grid` (defined next), below the existing `PageHeader`.

### 1.2 The 12-column content grid (Z2)

Every screen body uses one grid so panel placement is exact and consistent:

```css
.screen-grid {
  display: grid;
  grid-template-columns: repeat(12, minmax(0, 1fr));
  column-gap: 20px;   /* s20 */
  row-gap: 20px;      /* s20 */
  align-items: start;
}
```

Panels declare a span with `grid-column: span N`. **Placement blocks in this doc
give N.** Reflow rules:

| Breakpoint | Grid columns | Rule |
|---|---|---|
| ≥ 1100px (Z2 inner) | 12 | use spans as written |
| 700–1099px | 12 | any `span ≥ 7` becomes `span 12`; `span 4–6` becomes `span 6` |
| < 700px | 1 | every panel becomes full width (single column stack) |

### 1.3 Vertical rhythm & panel interior

- Gap between stacked panels/sections: **s20** (the grid `row-gap`).
- Panel interior padding: **s24** all sides (matches existing `.panel` 25px≈s24).
- Panel header (title row) to body: **s20**.
- Within a body, related rows: **s12**; label→value: **s4**.
- Never use ad-hoc margins; only the scale in §0.

### 1.4 Z-index layers (use the named tokens)

`content 0 < sticky status strip (--z-strip 10) < docked utility rail (--z-rail 20)
< overlay scrim (--z-scrim 90) < drawer (--z-drawer 100) < dialog (--z-dialog 200)
< toast/undo (--z-toast 300)`.

### 1.5 Z3 utility rail: docked vs drawer

- **Docked** (viewport ≥ 1200px, rail open): part of `.app-shell` grid, third
  column `--rail-right` (360px), full height, `position: sticky; top: 0`, own
  vertical scroll, left border `1px solid --line`. Z2 simply narrows.
- **Drawer** (viewport < 1200px, rail open): `position: fixed; top:0; right:0;
  height:100vh; width: min(360px, 100vw);` at `--z-drawer`, slide-in from right
  (200ms; instant under reduced-motion). A scrim (`--z-scrim`, `rgba(39,52,48,.28)`)
  covers Z2; click/Esc closes.
- **Toggle control:** a button in the global status strip top-right (see §4.2),
  label "Assistant", with a count badge when there are unread assistant messages.
  The rail can host **Assistant** (default) or the **compact Approvals queue**
  via a 2-tab switch at the rail top.

### 1.6 Navigation / IA changes

Left nav (`AppShell.tsx` `primaryNavigation`) — final order and additions:

```
◎ Overview   ▦ Calendar   ✓ Tasks   �queue Approvals(•n)   ∿ Rhythm
⊕ Medications   ⇄ Sharing   ⛁ Data Sources        [footer] ⚙ Settings
```

- **Rename** the existing `Timeline` route → **`Rhythm`** (`id:"rhythm"`), with
  three in-page tabs: **Actogram · Drift · Sources** (Sources = today's
  observation timeline). Keep `timeline` as a redirect to `rhythm#sources`.
- **Add** `Approvals` (`id:"approvals"`) between Tasks and Rhythm, with a count
  badge (pending proposals). Badge style: pill, `bg --amber-soft`, text #695528,
  `font-size:10px`, min-width 16px, right-aligned in the nav item.
- **Assistant** is NOT a left-nav item on desktop; it is the Z3 rail toggle.

**Android** (5-slot bottom bar): `Home · Rhythm · [Quick Log] · Assistant · More`.
`More` contains Calendar, Tasks, Approvals, Medications, Sharing, Data Sources,
Settings. When pending approvals > 0, Home shows an Approvals banner (§2.6) and
the More tab shows a badge.

---

## 2. Feature A — Auto-syncing calendar + approval queue

**Principle restated:** imported and fixed events are immutable; the app only
ever proposes moving *flexible tasks* and *app-created reminders*; **nothing is
applied without an explicit approval**. The approval queue is the single gate for
all schedule changes, whether proposed by the scheduler or the assistant.

### 2.1 Data/contract additions (name the fields; add schemas + ADR)

- `calendar-source` (new): `{source_id, kind: 'ics'|'caldav'|'google_ro', state:
  'connected'|'degraded'|'permission_revoked'|'paused', last_sync_at, scope:
  'read_only'|'read_write'}`.
- `change-proposal` (new, the queue item): `{proposal_id, origin:
  'scheduler'|'assistant'|'sync_conflict', kind: 'move'|'place'|'reminder_shift',
  task_id, from:{start_at,end_at}?, to:{start_at,end_at}, zone_id, confidence
  (common confidence), explanation_codes[], created_at, expires_at, status:
  'pending'|'approved'|'rejected'|'expired'}`. Reuse the existing
  `explanation_codes` enum from `schedule-proposals.schema.json`.

### 2.2 Calendar screen (extends `ui-ux-design.md` §9.3)

Adds a sync-status strip and an "alternatives" affordance. Layout:

```
PageHeader: "Calendar"                         [Day][Week*][Month]  [Add event]
┌─ .screen-grid ─────────────────────────────────────────────────────────────┐
│ Panel: Sync status                                              span 12      │
│  ● Google (read-only) · synced 12 min ago   ● ICS work · paused   [Manage]   │
├──────────────────────────────────────────────────────────────────────────────┤
│ Panel: Calendar board                                          span 12      │
│  (civil grid + predicted sleep/wake overlay bands, as §9.3)                  │
│  Fixed/imported events: solid blocks, NOT draggable.                         │
│  Flexible tasks: blocks with a small ⠿ handle; dragging opens a PROPOSAL     │
│  (never moves in place).                                                     │
└──────────────────────────────────────────────────────────────────────────────┘
```

**Placement — Sync status panel**
- Container: `.screen-grid` · Grid: `span 12` · Position: first child.
- Height: auto, min 56px. Padding s16 horizontal, s12 vertical.
- Contents: a left-aligned flex row, gap s20, of `SourceStatusChip`s; `[Manage]`
  secondary button pinned right (`margin-left:auto`).
- Responsive: <700px the chips wrap; `[Manage]` drops to its own row below.

**Placement — Calendar board panel**
- Container: `.screen-grid` · Grid: `span 12` · below sync panel (row-gap s20).
- Internal: existing `.calendar-hours` axis + `.calendar-days` rows (reuse).

**Interaction:** attempting to move a flexible task (drag, or the event's "Move…"
menu) does NOT change the calendar. It opens a **Proposal confirm sheet** (§2.4)
and, on confirm, adds a pending item to Approvals. Fixed/imported events expose
only "Suggest gentler times" (copy-out), never move.

### 2.3 Approvals screen (the queue)

This is the most placement-sensitive surface. Exact structure:

```
PageHeader: "Approvals"   "3 pending · 1 expiring soon"   [Approve all low-risk ▾]
┌─ .screen-grid ─────────────────────────────────────────────────────────────┐
│ Filter bar (Panel, span 12, 48px tall): [All 3][Scheduler 2][Assistant 1]   │
│                                          [Sync 0]            ↳ right: sort ▾  │
├──────────────────────────────────────────────────────────────────────────────┤
│ ProposalCard  (span 8, centered: grid-column 3 / span 8 on ≥1100px)          │
│ ┌──────────────────────────────────────────────────────────────────────┐    │
│ │▍ ⟲  Move · "Email Dr. Okafor"                          confidence ▮▮▯ │    │
│ │   From  Tue 11:40 AM   →   To  Tue 3:10–3:40 PM (≈4h after wake)       │    │
│ │   Why: lands in a likely-awake window · avoids your 2 PM appointment   │    │
│ │   Proposed by Scheduler · expires in 18 h                              │    │
│ │                                   [ Reject ]  [ Modify ]  [ Approve ]  │    │
│ └──────────────────────────────────────────────────────────────────────┘    │
│ ProposalCard … (next, row-gap s16)                                           │
└──────────────────────────────────────────────────────────────────────────────┘
```

**Placement — Filter bar**
- Grid `span 12`, height 48px, padding 0 s16, flex row align-center.
- Left: segmented filter buttons (gap s8). Right (`margin-left:auto`): sort menu.

**Placement — ProposalCard (the queue item)**
- Container: `.screen-grid`. Grid: **`grid-column: 3 / span 8`** on ≥1100px
  (centered, comfortable reading width ≈ 760px); `span 12` at 700–1099px;
  full-width at <700px. Stacked cards separated by **row-gap s16** (override the
  grid row-gap for the card list by wrapping cards in a sub-grid).
- Card box: `.panel` (paper, --line, radius 11), padding **s20**, `position:relative`.
- **Internal layout (CSS grid, 3 rows):**
  - Row 1 (header): `grid-template-columns: 4px 28px 1fr auto;`
    - col1 = status stripe (`width:4px; height:100%; position:absolute; left:0;
      top:0; border-radius:11px 0 0 11px`). Color by origin: scheduler=--sage,
      assistant=#8a7ca8, sync_conflict=#695528 (amber). Never red.
    - col2 = kind icon (move=⟲, place=＋, reminder=⏰), 20px, color --sage-dark.
    - col3 = title, `font-size:14px; font-weight:650; color:--ink`.
    - col4 = `ConfidenceMeter` (reuse from Overview), right-aligned.
  - Row 2 (change): the **before→after** line. `From <civil>` muted, an arrow
    `→` (s12 margins), `To <civil range>` in --ink, then the rhythm context in
    parentheses muted (`≈4h after wake`). Font 13px. Predictions are RANGES.
  - Row 3a (why): reason chips from `explanation_codes`, wrapping, gap s8; chip
    style = `.task-chip` (sage-soft). One human phrase per code (see
    `ui-ux-design.md` §9.4 mapping).
  - Row 3b (meta): `font-size:11px; color:--subtle` — "Proposed by … · expires …".
  - Row 4 (actions): flex row, `justify-content:flex-end; gap:s12; margin-top:s16`.
    Order left→right: **Reject** (quiet/secondary), **Modify** (secondary),
    **Approve** (primary `.button.primary`). Buttons min-height **44px** (touch).
- **Undo:** approving/rejecting shows an `UndoToast` (`--z-toast`, bottom-center,
  6s) — "Moved Email Dr. Okafor. [Undo]".

**Batch:** `[Approve all low-risk ▾]` (header, right) approves only proposals with
`confidence:high` AND `avoids_fixed_event` present; it lists what it will do in a
confirm dialog first. Each card also has an optional left checkbox (appears on
hover/focus, 16px, at the stripe's right edge) for manual multi-select → a sticky
action bar slides up from the bottom (`--z-strip`) with `[Approve n] [Reject n]`.

**States:** `pending` (default) · `expiring-soon` (<6h: meta row text amber, a
small clock glyph) · `empty` (no pending → centered EmptyState: "Nothing waiting
for your approval." + one-line "Proposals from the planner and assistant show up
here.") · `applied`/`rejected` (card collapses with a 200ms height fade, replaced
by a one-line confirmation that auto-dismisses).

**Compact queue (Z3 rail variant):** same card, but `span` = full rail width,
padding s16, Row 1 drops the kind icon, actions become two icon-buttons (✓/✕,
44px) with the full action set behind a "⋯" → opens the full Approvals screen.

### 2.4 Proposal confirm sheet (when a user-initiated move would create one)

A centered dialog (`--z-dialog`, width min(520px, 92vw)). Shows the same
before→after + reason + confidence, with `[Cancel]` (left) and `[Add to
Approvals]` (right, primary). Rationale: even user-dragged moves are queued, so
there is one consistent apply path and audit trail.

### 2.5 Data Sources additions

Add calendar sources to the existing Data Sources list (`ui-ux-design.md` §9.9):
one row per calendar with `SourceStatusChip` (state), last-sync time, scope badge
(`read-only` sage / `read-write` amber), and `[Manage]` (re-auth, set
priority, pause, disconnect). Write scope is OFF by default and shows a one-line
consent explainer before enabling.

### 2.6 Microcopy, errors, acceptance

Microcopy:
- ✅ "Proposed — nothing changes until you approve." / "Approved. Undo?"
- ✅ "This appointment is fixed; I can suggest gentler times instead."
- ❌ never "Auto-moved", "Optimized your day", or red styling on a normal move.

Errors/edge: sync failure → source chip `degraded` + "last synced …" (cached
shown, marked stale); permission revoked → `permission_revoked` chip + one-tap
re-auth; conflict (external edit vs pending proposal) → the proposal becomes a
`sync_conflict` card explaining both sides, never auto-resolved.

Acceptance criteria (testable):
1. No code path mutates a calendar item or task time without a recorded `approved`
   proposal.
2. Fixed/imported events expose no move affordance (only "suggest times").
3. Each ProposalCard renders before→after as civil ranges + one chip per present
   `explanation_code` + a `ConfidenceMeter`; never a single-point predicted time.
4. Approve/Reject always offer Undo; expired proposals are not actionable.
5. `read-write` scope defaults off; enabling requires explicit consent.

---

## 3. Feature B — Sleep chart visualizer (Rhythm)

A read-only projection of local observations, corrections, and the current
estimate. Reference for the actogram: Zeitlog's drifting sleep-band mark (the
free-running diagonal). The existing `.actogram` CSS in `styles.css` is the
starting point — extend it; do not start over.

### 3.1 Rhythm screen shell

```
PageHeader: "Rhythm"   "Estimated from your recent sleep — not exact phase"
Tabs (Panel, span 12, 44px):  [ Actogram* ] [ Drift ] [ Sources ]
┌─ .screen-grid ─────────────────────────────────────────────────────────────┐
│  <active tab panel>  span 12                                                 │
└──────────────────────────────────────────────────────────────────────────────┘
```

**Placement — Tab bar:** Grid `span 12`, height 44px, a `.panel` with role
`tablist`; tabs are buttons, gap s8, active tab = sage-soft bg + sage-dark text
(reuse `.filter.active`). Keyboard: ←/→ move tabs, Home/End jump.

### 3.2 Actogram (double-plot) — exact spec

> This is the in-app Rhythm actogram. For the **clinical longitudinal actogram**
> (one row per calendar date over months/years, a 6 pm/noon day-start anchor,
> single-plot, intervention annotations, and clinician PDF export) modelled on the
> real reference sleep logs, see **§3.6**.

```
┌ Panel (span 12, min-height 560px, padding s24) ───────────────────────────────┐
│ Header row (height 32, margin-bottom s20):                                     │
│   left:  "Last 30 cycles"        right: [14][30*][60][90]d  ⌧ double-plot      │
│                                         source ▾   ☑ show forecast   legend    │
│ Axis row (height 28):  0   6   12   18   0(24)   6   12   18   0(48)           │
│ Plot rows (one per day; --row-pitch 28px):                                     │
│   ┌label 64px┐┌──────────────── plot area (fluid, 48h wide) ─────────────────┐ │
│   │ Jun 15   ││            ▓▓▓▓▓▓▓ (sleep)                                    │ │
│   │ Jun 14   ││         ▓▓▓▓▓▓▓                                              │ │
│   │ Jun 13   ││      ░░░░░░░ (inferred, hatched)                            │ │
│   │ ───────── now line (1px --sage, label "now") ───────────────────────── │ │
│   │ Jun 16   ││ forecast: ▒▒▒▒▒▒▒▒▒  (widening hatched band)               │ │
│   │ Jun 17   ││ forecast: ▒▒▒▒▒▒▒▒▒▒▒  (wider)                             │ │
│   └──────────┘└──────────────────────────────────────────────────────────┘ │
│ Footer: legend chips + "Approximate. Forecast widens with time."              │
└────────────────────────────────────────────────────────────────────────────────┘
```

**Placement & geometry (exact):**
- Panel: Grid `span 12`. Min-height 560px (grows with row count).
- **Day-label gutter:** fixed **64px** left column. Labels right-aligned, 11px,
  `--muted`, vertically centered in the row.
- **Plot area:** the remaining width, representing **48 hours** (double-plot) when
  the toggle is on, else 24h. X position of any time = `((tMinutes − rowStartMin)
  / span) * 100%` where `span = 2880` (48h) or `1440` (24h).
- **Row:** height 22px band track inside a `--row-pitch` (28px) row; 6px vertical
  gap. **Newest day on top** by default (toggle to flip).
- **Gridlines:** vertical lines every 6h (`--line`, 1px); heavier line at 24h and
  48h (midnight, `#cdd3cf`).
- **Sleep band:** height 14px, vertically centered, `border-radius 3px`.
  - confirmed (`directly_observed`/`user_reported`): solid `#6f8c82` (existing).
  - inferred: hatched (existing `data-quality="Estimated"` repeating gradient).
  - nap: height 8px, same x-rules.
  - corrected: a 2px left cap in `--sage-dark` + a small dot marker; original
    shown faded behind at 0.35 opacity (toggle "show original").
- **Now line:** full-width horizontal 1px `--sage` between the last past row and
  the first forecast row, with a right-edge label "now" (11px, sage-dark). A
  vertical 1px `--sage` tick at the current civil time in the top (today) row.
- **Forecast rows:** below the now line. Predicted sleep window = a band from
  `earliest_at` to `latest_at` (NOT a point), rendered hatched at opacity 0.45,
  **widening with each further cycle** (the contract already widens the window).
- **Drift guide (optional, off by default):** a faint 1px `--sage` polyline
  through consecutive sleep-onset x-positions, illustrating the diagonal.

**Controls (header right, in order, gap s12):**
- Range selector `[14][30][60][90]d` (segmented; reuse `.filter`).
- `double-plot` toggle (default ON).
- `source ▾` (filter: all / wearable / phone / manual / health connect).
- `show forecast` checkbox (default ON).
- `legend` (inline chips: ▓ observed · ░ inferred · ▒ predicted · | now).

**Tooltip (hover/focus a band):** small popover (`--z-dialog`-1), shows civil
interval (`Sat 1:10 AM – 9:30 AM`), duration (`8h 20m`), source icon + label,
confidence. Anchored above the band; flips below near the top edge.

**Accessibility (required):**
- Panel has `role="img"` + `aria-label` summarizing drift + range.
- A visually-hidden `<table>` immediately after the chart: columns **Day · Sleep
  start · Wake · Duration · Source · Confidence**, one row per day; this is the
  screen-reader equivalent. Each band is also a focusable element exposing the
  same as its `aria-label`.
- Color is never the only signal: pattern (solid/hatched) + the table carry it.
- Reduced-motion: no transitions on band render.

### 3.3 Drift / phase trend chart

```
┌ Panel (span 12, height 420, padding s24) ─────────────────────────────────────┐
│ "Sleep-onset drift"            +50 min per cycle (medium confidence)  legend   │
│  y: onset │                                          • observed onset          │
│   (clock) │            •                              — Theil–Sen fit          │
│           │       •  •      ⟍ fit + ±band                                      │
│           │  •  •      •  ⟍                                                    │
│           └─────────────────────────────────────────────  x: date →           │
└────────────────────────────────────────────────────────────────────────────────┘
```

- Grid `span 12` (or `span 7` if paired with a stats panel `span 5` on ≥1100px).
- **Y axis** = sleep-onset clock time, **unwrapped** so the free-running trend is
  a straight line (do not let it wrap at midnight and zigzag); left axis 56px,
  ticks every 3h, secondary right labels show civil time-of-day.
- **X axis** = date over the selected range; bottom axis 28px.
- Points = observed onsets (4px dots, `--sage-dark`). Line = Theil-Sen fit
  (2px `--sage`). Band = ± uncertainty (sage at 0.15 alpha). Slope annotation
  top-left, paired with the ordinal confidence (never a fake R²/percentage).
- Tooltip per point: date + civil onset + source. Same table a11y alternative.

### 3.4 Distributions (later; phase 2 stretch / phase 3)

Optional small-multiples panel (`span 6` each): sleep-duration histogram and
onset-time histogram. Bars `--sage-soft` fill, `--sage` outline. Defer if time-
constrained; the actogram + drift are the MVP of this feature.

### 3.5 States & acceptance

States: rich · sparse ("Not enough cycles yet to chart drift — add or confirm a
few sleeps.") · refused (no estimate → actogram still shows observed sleep; drift
tab shows the sparse message) · stale (greyed + "last updated …").

Acceptance:
1. Predicted windows render as widening bands; no hard predicted lines anywhere.
2. Every chart has a screen-reader table with the listed columns and passes a
   grayscale render test (state still distinguishable).
3. Actogram double-plot makes a constant-drift rhythm read as a straight diagonal.
4. The Rhythm screen makes no network request and reads only local data.

### 3.6 Automatic sleep charting (clinician actogram + export)

**Why this exists.** The canonical reference is a real patient record spanning
2021–2023 in two forms: (a) a hand-kept clinical **"Sleep Log"** (Raleigh
Neurology) — days as rows, **1-hour columns**, sleep shaded by hand, with a
hand-drawn legend (`✱` = sleep disruption; `melatonin 1 mg nightly 9 pm`;
`melatonin + morning light-therapy lamp`); and (b) a **computer-generated
actogram** ("Sleep Chart …") — blue sleep bands, one row per calendar day, a
**6 pm-anchored 24 h** axis, showing the free-running diagonal (SleepGraph /
sleepdiary style). The goal: ZeitBoard **auto-produces that clinical actogram from
imported data** so the user never hand-charts again, and **exports a printable
record for a sleep clinician**.

> **Privacy (hard rule):** that example is the user's private health data. Never
> commit it, screenshot it into the repo, or derive fixtures from it. All chart
> fixtures stay synthetic (AGENTS.md). The renders used to design this were deleted.

This reuses the §3.2 actogram engine with a **clinical longitudinal mode**,
**annotation overlays**, and **export**. Additions:

**1) Clinical orientation & calendar rows**
- **Day-start anchor** (setting): default **6:00 pm** (noon optional) so a
  nocturnal sleep period is never split at a row edge — the clinical convention in
  the reference. Single-plot **24 h** is the clinician default; the §3.2 double-plot
  stays available (it makes the Non-24 diagonal continuous).
- **One row per calendar date** across the whole selected range (not "last N
  cycles"): weekday label (`27 Wed`, weekends emphasized), **month-boundary
  separators** (`October 2023`), and a subtle background band on weekend/month-edge
  rows — matching the reference.
- **Ranges:** week / month / custom / **all** (the reference spans ~2 years);
  virtualize long ranges and paginate per month for print.

**2) Annotation overlays — auto from logged data, observations never advice**
- **Sleep disruptions / awakenings** (`✱`): a marker inside the sleep band, from
  wearable wake-after-sleep-onset or a manual "mark disruption."
- **Medication markers** (e.g. melatonin): a glyph at the dose's civil time on that
  day's row, from the existing medication log (ui-ux-design.md §9.6). Label is the
  user's own text. **No dosing or timing advice — recording only.**
- **Light-therapy / other intervention markers**: a glyph from a *user-logged*
  intervention event. ZeitBoard records that the user did it; it never recommends
  light, melatonin, meals, or exercise (AGENTS.md).
- Each chart shows a legend listing **exactly the annotation types present** (mirrors
  the hand-drawn legend). Inferred sleep stays hatched; confidence stays ordinal.
- **Forecast off by default in clinical mode** — a clinician wants observed history,
  not predictions, unless explicitly turned on.

**3) Automatic pipeline (data → chart; read-only, local)**
- Sources: Health Connect / wearable (Fitbit etc.) / manual / file-import sleep
  episodes → effective observations (after corrections) → bands. Same import +
  correction layers the app already has; the chart is a pure projection — **no new
  estimation**, gaps render as empty "no data" rows, never invented sleep.
- Reference implementation to mirror: **Zeitlog's SleepGraph** (the user's
  sleepdiary-derived actogram) produces exactly the digital example; ZeitBoard
  brings it native, fed by the same imports. The *automatic* part depends on the
  phase-2 import hardening already on the roadmap.

**4) Clinician export (the artifact's real purpose)**
- **Export to printable PDF** (and PNG): the longitudinal actogram, paginated by
  month, with the legend, date range, generation date, a provenance/units footer,
  and the standard "estimated windows are uncertain and not medical advice" notice.
  This is the "bring to your sleep doctor" deliverable; ties to Reports/Export
  (ui-ux-design.md §9.11) and the clinician persona.
- Optional **clinical sleep-log template** export: a blank or filled grid in the
  familiar days × 1-hour layout for clinicians who expect that form.
- **Redaction controls:** the export carries no diagnosis or location; medication
  markers may be included or redacted at the user's choice.
- Export is a deliberate, confirmed action (it leaves the device only when the user
  saves/shares); **never auto-upload**.

**5) Contract additions (name fields; add schemas + an ADR; fixtures synthetic)**
- Reuse `observation-set` `sleep_episode` for bands. Add an **annotation set**:
  `{kind: 'disruption'|'medication'|'light_therapy'|'note', at (timestamp+zone_id),
  label, provenance}` — disruptions/interventions as user-logged observations;
  medication markers map from existing medication events.
- A **chart-export request** DTO: `{range:{from,to}, orientation:'24h'|'48h',
  day_start_hour, include:{forecast,medication,disruption,light_therapy},
  redactions[]}`.

**6) Acceptance (testable)**
1. A multi-month synthetic fixture renders one row per calendar date with month
   separators and weekend emphasis; a constant-drift fixture shows the diagonal; the
   6 pm anchor keeps nocturnal sleep uncut (no band split at a row edge).
2. Disruption / medication / light markers appear at the correct civil position from
   logged events; the per-chart legend lists exactly those present and omits absent
   types.
3. Gaps render as empty "no data" rows; inferred sleep stays visually distinct; no
   invented sleep.
4. Export produces a paginated PDF with legend + range + notice; redaction toggles
   remove the chosen layers; export requires explicit confirmation and makes no
   network call.
5. A screen-reader table covers every row including annotations; grayscale-safe.

---

## 4. Feature C — Conversational assistant (chatbox)

> **Backend default updated by [ADR-0007](decisions/0007-connected-cloud-architecture.md).**
> The product is now connected/cloud, so the **cloud LLM is the standard backend** — the
> "provider defaults to off/local" wording in §4.4 and §4.6 is superseded (a local/offline
> mode may remain as a *fallback*, not the default). Everything else in §4.4/§4.6 still
> holds and should be implemented as written: the model only emits allowlisted actions the
> **server** resolves (it mutates nothing), every change goes through the approval queue,
> context is redacted/role-scoped, and the active backend is always disclosed. Additionally,
> the cloud provider must not train on or retain the context (DPA / zero-retention tier).

A local-first assistant that **manages the schedule by creating approval-queue
proposals** (never applying changes) and **answers questions about local data**
(never medical advice). It reuses the proven local-first LLM-chat design already
shipping in **NoobBoard** (the user's local-first Go dashboard, whose
`docs/llm-policy.md` and `docs/opencode-evaluation.md` document the model): a
provider abstraction that defaults to off/local, strict-JSON **allowlisted
actions the server resolves** (the model itself mutates nothing), redacted
role-scoped context, and a Codex/OpenCode-style approval step. The transcript
shows inline action cards; the resulting changes land in the §2 approval queue.
The backend/safety contract is §4.6 — implement it, not just the visible chat.

### 4.1 Where it lives

- **Desktop:** the Z3 right utility rail (§1.5), 360px, toggled from the status
  strip. Docked ≥1200px, drawer below that.
- **Android:** a top-level `Assistant` tab (full screen).
- It is the same component in both; only the container differs.

### 4.2 Status-strip toggle (desktop)

```
[≡] ZeitBoard      ● Likely awake · 4h since wake        [◐ Reduce] [✦ Assistant •] [?]
```
- `[✦ Assistant]` button: top-right of the global status strip, height 32, padding
  0 s12, gap s8. A count badge (•n) when the assistant has unread output. Active
  (rail open) = sage-soft bg.

### 4.3 Assistant panel anatomy (exact)

```
┌ Assistant rail (width --rail-right 360px, full height) ──────────────┐
│ Header (height 52, padding 0 s16, border-bottom 1px --line):         │
│   "Assistant"            ● Local      [Approvals 3]   [✕]            │  ← backend dot + queue link + close
│ ┌ Tab switch (height 40): [ Chat* ] [ Queue ] ┐                      │
│ Transcript (flex:1, overflow-y:auto, padding s16, gap s12):          │
│   ┌ assistant msg (left, max-width 86%, paper, border --line) ────┐  │
│   │ You're likely awake for ~5 more hours (medium confidence).    │  │
│   └────────────────────────────────────────────────────────────┘  │
│   ┌ user msg (right, max-width 86%, bg --sage-soft) ─────────────┐  │
│   │ find 90 min for taxes before Friday, not right after I wake   │  │
│   └────────────────────────────────────────────────────────────┘  │
│   ┌ ACTION CARD (assistant, full width of bubble column) ────────┐  │
│   │ ⟲ Proposed: Taxes · Thu ~2:10–3:40 PM   ▮▮▯                  │  │
│   │ avoids fixed events · ≥3h after wake                          │  │
│   │           [ View in Approvals ]   [ Reject ]  [ Approve ]     │  │
│   └────────────────────────────────────────────────────────────┘  │
│ Composer (min-height 64, padding s12, border-top 1px --line):       │
│   ┌ textarea (auto-grow 1–5 lines, max 120px) ───────────┐ [ Send ] │
│   └──────────────────────────────────────────────────────┘         │
│   "Manages your schedule via approvals. Not medical advice."  (10px)│
└──────────────────────────────────────────────────────────────────┘
```

**Placement specifics:**
- **Header:** flex row, align-center. Left: title (14px, 600). Then the **backend
  indicator** (dot + text): `● Local` (dot `--sage`) or `● Connected: <name>`
  (dot #695528 amber). Right cluster (`margin-left:auto`, gap s8): `[Approvals n]`
  text-button linking to the Approvals screen, and `[✕]` close (44px hit area).
- **Tab switch:** `Chat` (transcript) / `Queue` (the compact Approvals list from
  §2.3). 40px, segmented, reuse `.filter`.
- **Transcript:** `flex:1; min-height:0; overflow-y:auto`. Message column gap
  **s12**. Auto-scroll to bottom on new message unless the user has scrolled up
  (then show a "↓ new" pill at bottom-right, `--z-strip`).
- **Message bubbles:** padding s12 s16; radius 11; user right-aligned
  (`margin-left:auto`, `bg --sage-soft`, ink text); assistant left-aligned
  (`bg --paper`, `border 1px --line`). Max-width 86% of the column. Timestamps on
  hover only (11px, --subtle).
- **Action card (inline proposal):** full bubble-column width, `.panel` style,
  padding s12, left status stripe 3px (#8a7ca8 assistant origin). Rows: title +
  ConfidenceMeter; reason chips; actions right-aligned `[View in Approvals]`
  (quiet) · `[Reject]` (secondary) · `[Approve]` (primary), each 44px. Approve/
  Reject here act on the queue item directly and reflect status inline.
- **Composer:** pinned bottom (not floating). Textarea auto-grows 1→5 lines
  (max-height 120px, then scroll). `Send` button right, 44×44, sage. Enter =
  send, Shift+Enter = newline. Disabled state while the assistant is responding,
  with a 3-dot typing indicator as the latest assistant bubble.
- **Disclaimer line:** always visible under the composer, 10px `--subtle`.

### 4.4 Behavior & safety (hard rules)

- **Proposals only.** The assistant creates `change-proposal` items (origin
  `assistant`); it MUST NOT mutate tasks/events/calendar directly. "Approve" in
  chat is just a shortcut to the same queue action (audited identically).
- **Medical refusal script** (consistent, neutral, not red): rendered as a normal
  assistant bubble — "I can't help with medical decisions like medication or
  dosing. I can show when you logged doses relative to your rhythm, or help you
  plan around appointments." No diagnosis, no treatment/light/melatonin timing.
- **Answer scope:** estimated phase, time-since-wake, drift, next predicted
  windows (as ranges + confidence), explanations of proposals, schedule queries.
  Always civil-time-primary, uncertainty-visible.
- **Backend disclosure:** the header dot is authoritative. Default `Local`
  (offline parsing; no health data leaves device). A `Connected` backend is
  opt-in in Settings, off by default, sends only minimal non-identifying context,
  and is gated by the privacy review + threat-model update (roadmap §4). If the
  user asks "where does my data go?", the assistant states the current backend.

### 4.5 States & acceptance

States: empty (first run → 2–3 example prompts as tappable chips: "What's my next
good window?", "Move my flexible tasks off tonight", "When did I last sleep?") ·
thinking (typing indicator; composer disabled) · proposal-made (action card) ·
refused-medical (script) · offline/backend-error ("I'm offline right now — I can
still answer from your local data." or a calm error) · backend=connected (amber
dot + a one-time "this sends limited context to <name>" note).

Acceptance:
1. The assistant has no API to apply a schedule change except creating a pending
   proposal; an automated test asserts no direct mutation path.
2. A medical prompt yields the refusal script and creates no proposal.
3. With the default backend, no network request carrying health data is made
   (assert in tests/CSP, mirroring the trusted-web network ban).
4. The backend indicator matches the actual backend; switching backends updates
   it and shows the consent note.
5. Composer: Enter sends, Shift+Enter newlines, 44px send target, disabled while
   responding.

### 4.6 Assistant backend & safety architecture (grounded in NoobBoard)

NoobBoard — the user's local-first Go dashboard — already ships a working LLM
chat with the exact guarantees ZeitBoard needs. Reuse its model (narrowed from
infra-repair to scheduling); the patterns below are written out so this spec is
self-contained.

**Pipeline** (adapt NoobBoard's `telemetry → collectors → rules → facts →
redaction → role-scoped context → strict JSON → audit`):

```
local observations + corrections + current estimate
  → redaction + PHI/role scoping (no med names, no raw behavioral timestamps,
    no calendar text beyond what the request needs)
  → bounded, prioritized context builder (the question's tasks/dates first)
  → LLM call (strict JSON: an allowlisted action + structured target, OR answer-only)
  → SERVER resolves the action id against ZeitBoard's action registry → change-proposal
  → approval queue (human)  [+ optional reviewer-model gate, see below]
  → audit
```

**Hard rules (copy NoobBoard's, narrowed to scheduling):**

- **Provider defaults to off/local.** With no cloud provider configured the
  assistant answers only from local data and never fabricates; cloud providers
  (OpenAI/Anthropic) are opt-in in Settings, called via plain HTTP (no SDK), and
  surfaced by the header dot (§4.3). Deterministic dev uses fixtures, never a mock
  answer generator.
- **The model mutates nothing.** It returns only a schema-allowlisted
  `recommended_action` ∈ `{propose_move_task, propose_place_task,
  propose_reminder_shift, answer_only}` plus a structured target (task id /
  window). The **server** resolves it into a `change-proposal`; unknown or
  unresolved targets become a non-actionable `unknown` result that opens no
  approval. (NoobBoard: `recommended_action_id` + `recommended_action_target` →
  server-resolved `agent_plan`.)
- **No tools touch raw data, credentials, files, or the network by default.** If a
  read-only "look up my schedule" tool is ever added, gate it like NoobBoard's
  read-only status tools: allowlisted names, a hard call budget, redaction on
  every result, fail-closed on unknown/oversized/invalid.
- **Signed, one-use approval token.** Approving a proposal (from chat or the
  Approvals screen) references a short-lived server-signed token (proposal id,
  action id, actor, resolved target, replay nonce) and is rate-limited and
  audited identically either way.
- **Optional reviewer gate — for any future auto-apply only.** If the user ever
  enables auto-apply for low-risk moves, a separate reviewer model must return
  PASS over the redacted state + allowlisted `docs/*` references before it runs;
  reviewer failure/denial fails closed and is audited. ZeitBoard's default stays
  human approval, so this is an extra layer, never the gate. (NoobBoard
  `action_auto_review`.)
- **Credentials never leave their lane.** Settings reads expose only booleans
  (e.g. `assistant_connected`); raw keys/tokens are never returned, logged,
  notified, or placed in LLM context. A connected backend proves a credential
  exists — it is NOT authorization to change the schedule.
- **Bounded, never-truncated-to-garbage context.** On overflow, compact/shrink
  older detail and retry one smaller request (OpenCode pattern); never split into
  chunks or emit invalid JSON.
- **No web access** in the assistant path (OpenCode's discovery/retrieval split
  stays off; a future research feature would be separate, allowlisted, audited).
- **Usage limits are calm, not crashes.** A provider quota/rate-limit error
  surfaces as a plain message ("the assistant's service hit its usage limit") and
  the assistant falls back to local answers.

**UI consequences (from NoobBoard `docs/ux-compact.md`):**

- **Omit, don't disable.** When no usable assistant backend exists, hide the
  assistant entry entirely — never show a dead/disabled chat box. (Local parsing
  for scheduling/Q&A counts as usable; cloud features are additive.)
- **Plain language, enforced like a banned-term audit.** Assistant output obeys
  the product language rules — rhythm-relative plain terms, civil time primary,
  never clinical jargon (`DLMO`, `circadian phase`) or medical advice.
- **44px** for Send and every action control (NoobBoard flagged a 40–42px send as
  a real bug).
- **Disclose the backend** truthfully; if asked "where does my data go?", answer
  from the active backend.

**Acceptance additions:**
6. With no cloud provider configured, the assistant still answers local questions
   and creates proposals via local parsing; zero network requests carry health data.
7. Model output is validated against the allowlisted action schema; any
   non-conforming/unknown action yields `answer_only`/`unknown` and creates no proposal.
8. Approving from chat consumes a one-use signed token, audited identically to the
   Approvals screen.
9. When no backend is usable, the assistant entry is absent (not a disabled box).

### 4.7 Agent-accessible interface (MCP connector / skill) + live voice

The §4.6 action registry is not assistant-only — it is the **agent-accessible capability
layer** that makes the whole app drivable non-visually, the intended primary path for blind
users ([ADR-0006](decisions/0006-agent-accessible-interface.md)). Same registry, same approval
gate, same redaction; the agent is just a different client over it.

- **Shape.** Expose two tool families over the existing registry: **read tools** returning the
  speakable projection DTOs the UI already uses (overview, rhythm summary + the chart
  `sr-table` data, pending proposals, conflicts, next windows — civil-time-primary,
  uncertainty-visible), and **propose-only action tools** (`propose_move_task`,
  `propose_place_task`, `propose_reminder_shift`, `log_sleep`, ...) the server resolves into
  `change-proposal`s. No tool mutates state directly, touches credentials/files/network by
  default, or returns the raw domain model — same fail-closed allowlist / call-budget /
  redaction rules as §4.6.
- **Delivery.** (a) a **local MCP server** *(leading option)* so an MCP client such as Claude
  Desktop drives ZeitBoard; the connector is local, so with a local agent no health data
  leaves the device. (b) a **Claude/ChatGPT skill** wrapping the same tools for a cloud
  assistant — opt-in, off by default, gated like any connected backend.
- **Voice.** Live voice is the *client's* (Claude/ChatGPT voice mode, or the OS); ZeitBoard
  ships no TTS/STT and just returns concise, speakable results. Loop: voice-in → agent →
  ZeitBoard tools → structured result → voice-out.
- **Backend disclosure & consent** identical to §4.4: the active agent/backend is always
  shown; a cloud agent triggers the one-time minimal-context consent note and needs its own
  privacy review + threat-model update.

**Acceptance additions:**
10. Every UI-exposed capability has a corresponding agent read or propose-only tool; a test
    asserts no action tool has a direct-mutation path (all route through the approval queue).
11. Agent read tools return only allowlisted projection fields (no raw domain model, med
    names, diagnosis, raw activity, or un-needed calendar text), asserted like the
    trusted-view projection tests.
12. With a local MCP client + local agent, zero network requests carry health data.

---

## 5. New components (inventory delta) + inputs

Reuse existing components (`StatusBadge`, `ConfidenceMeter`, `BurdenChip`,
`ProvenanceTag`, `UndoToast`, `.panel`, `.button`, `.filter`, `.task-chip`).
New:

```
UtilityRail(open: boolean, mode: 'assistant'|'queue', dock: 'docked'|'drawer')
SourceStatusChip(state, label, lastSyncAt?, scope?: 'read_only'|'read_write')
ProposalCard(proposal: change-proposal, variant: 'full'|'compact',
             onApprove, onReject, onModify)
BatchActionBar(selectedIds: string[], onApprove, onReject)
RhythmTabs(active: 'actogram'|'drift'|'sources')
Actogram(rows: DayRow[], range: 14|30|60|90, doublePlot: boolean,
         showForecast: boolean, sourceFilter, newestFirst: boolean)   // + hidden <table>
DriftChart(points: {date,onsetMin,source}[], fit:{slopeMinPerCycle,band},
           confidence)                                                 // + hidden <table>
AssistantPanel(messages: Message[], backend: 'local'|{name}, busy: boolean,
               onSend, onApprove, onReject)
MessageBubble(role: 'user'|'assistant', kind: 'text'|'action'|'refusal')
ActionCard(proposal, onApprove, onReject)   // inline proposal in transcript
ChatComposer(value, onChange, onSend, disabled)
```

Every data-bearing component declares one of the universal states from
`ui-ux-design.md` §13: `loading · empty · ok · low-confidence · inferred ·
confirmed · conflicting · stale · refused · offline · permission-revoked ·
paused · error`.

---

## 6. Codex build order & checklist

Build in this order so each layer rests on a finished one:

1. **Layout primitives** — `.screen-grid`, the Z3 rail (docked + drawer +
   scrim), the status-strip Assistant toggle, new `:root` tokens (§0/§1).
   *Done when:* an empty rail opens/closes/docks/drawers correctly at 1200/940/700.
2. **Rhythm/Actogram** (read-only, no new data) — tabs, actogram geometry, drift
   chart, hidden tables. *Done when:* §3.5 acceptance passes.
3. **Approvals queue** (proposal model + screen + compact rail variant + undo).
   *Done when:* §2.6 acceptance passes; user-dragged moves route through the queue.
4. **Calendar sync** (read-only sources, sync status, conflict cards).
   *Done when:* sources show status; conflicts become `sync_conflict` cards.
5. **Assistant** (panel, transcript, composer, action cards → queue; local
   backend; medical refusal). *Done when:* §4.5 acceptance passes.
6. **Write-back** (last; off by default; behind security review) — only after 1–5.

Global checks for every screen: uses `.screen-grid` spans as written; reuses the
§0 colors/radii/spacing tokens (no new ones); predictions render as ranges;
keyboard + screen-reader path exists; 44px interactive targets; reduced-motion
honored; no network on read-only surfaces.
