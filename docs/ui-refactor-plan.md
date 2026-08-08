# UI refactor plan: density, identity, and the theme manager

> Planning and implementation record. Complements `ui-ux-design.md` (which remains the master
> interaction spec); this plan revises the *visual system* and adds a theme
> architecture. Visual-first remains the constitution: accessibility features
> are added where they cost nothing visually (ADR-0005 stance).

## 1. Review findings (measured, 2026-07)

Screenshots reviewed: Overview, Tasks, Rhythm (light + dark), 1440×900.
Stylesheet audited quantitatively. The critique "generic flat card dashboard"
was confirmed by numbers, not taste. These are baseline findings from before
the structural follow-up described in section 5:

1. **Card soup.** `.panel` (paper bg + 1px border + 11px radius + shadow) is
   used **48×** across screens — every kind of content, from a one-line
   notice to the actogram, gets the identical floating tile. Nothing looks
   like anything in particular; hierarchy is conveyed only by position.
2. **Wasted space.** At 1440×900 the Overview fills ≈55% of the viewport and
   the bottom half is empty canvas; the three metric tiles each hold one
   label + one line inside ≥112px of card. The page header alone consumes
   ~110px (80px min-height + 30px margin) before any data. On Rhythm — the
   app's reason to exist — the actogram gets roughly a quarter of the
   screen and floats in dead space.
3. **Accent monoculture.** `var(--sage*)` appears **54×**; the already-defined
   semantic hues are almost unused (asleep blue 2×, uncertain amber 3×,
   assistant purple 2×). One green does branding, buttons, focus, meters,
   chips, dots, and charts — so it stops meaning anything, and the page
   reads as "green everywhere" (the user's exact complaint).
4. **Hollow type scale.** 84 of ~113 font-size declarations are ≤13px
   (10px alone appears 31×), then the scale jumps to a 29–38px H1. There is
   no confident middle (15–22px) for data display; numbers that matter
   (drift, windows, confidence) render at label sizes.
5. **Token drift.** Six ad-hoc radii (3/4/6/8/9/11), pad values from 4 to
   48px with no scale, shadows on everything at equal elevation.

## 2. Refactor principles (de-vibecoding)

1. **One surface per screen; cards only for objects.** The screen itself is
   the surface; sections divide by rules and whitespace rhythm, not by
   floating tiles. Cards are reserved for things that *are* objects with
   lifecycle — a proposal, a sleep entry, an action card in chat. Shadows
   exist only at true overlay elevation (rail, dialogs, menus).
2. **Data-first density.** Kill the empty-tile pattern: a metric is a
   number+label row, not a card. Overview becomes a **status band** (phase
   state, time since wake, next windows — one dense strip) above a **large
   "today in your cycle" timeline**, with the trust-loop and quality rows
   compacted beneath. Fill-or-shrink rule: no container may be more than
   ~40% padding at rest.
3. **The actogram is the identity.** It is the one visual no other planner
   has — give it the hero budget everywhere it appears: full content width,
   taller day rows, the current cycle visually emphasized, and a compact
   "cycle strip" variant reused on Overview. Charts get vertical priority
   over chrome on every screen.

   The compact cycle strip shows the estimated waking span and predicted sleep.
   The exact useful-task window stays textual until the DTO exposes structured
   bounds; formatted prose is not parsed into a more precise chart claim.
4. **Semantic color, not brand color.** Sage retracts to two jobs: primary
   action and "awake" state. Sleep bands/forecasts move to the asleep blue
   family; uncertainty and caution live in amber; agent origin stays purple;
   reason chips and metadata go neutral (outline/ink). Rule of thumb: color
   answers "what state is this?", never "which app is this?".
5. **Type scale with a middle.** 11 / 13 / 15 base, 18 / 22 for data
   display (tabular numerals for times and durations), 28 for the page
   title (down from 38 — the H1 is not the content). Micro-caps labels
   (10px) survive only as table headers and kickers.
6. **Token discipline.** Radii: 4 (controls) / 8 (cards) / 12 (overlays).
   Spacing on a 4pt scale with named steps. Every component consumes tokens;
   no literal px colors/radii in component CSS.

## 3. Theme manager (the user-facing feature)

### Why this is a circadian feature, not a skin

Amber is a compatibility and accessibility preset for users who already
choose dark-amber eyewear or a blue-minimized display. It is **not** a
treatment recommendation. The AASM guideline makes population-specific,
mostly weak recommendations for selected circadian-disorder treatments and
does not establish amber lenses as standard N24SWD/DSWPD care
([guideline](https://pubmed.ncbi.nlm.nih.gov/26414986/)). Small randomized
amber-lens trials studied insomnia symptoms, not Non-24
([trial](https://pubmed.ncbi.nlm.nih.gov/29101797/)).

Lens transmission and display spectra vary by product. CSS cannot guarantee a
wavelength cutoff: it controls RGB subpixels, not emitted spectra. The preset
therefore makes the narrower engineering promise that its palette minimizes
the commanded blue channel and remains legible under a conservative simulated
dark-amber filter. Consequences for UI seen through many such lenses:

- Blue elements can become very dark; green may lose substantial luminance;
  white dims toward amber. Information must never depend on blue-vs-green.
- The highest through-lens contrast available is **bright amber/orange
  (≈590–610nm) on true black** — the classic amber terminal, which is why
  that aesthetic exists.

Define a simulated **through-lens luminance** heuristic for regression testing: `L' = 0.2126·R +
0.7152·(g·G) + 0.0722·(b·B)` with dark-amber transmission factors `g≈0.25,
b≈0.02`. This is not spectroscopy or clinical validation. The Amber preset
must hold **≥7:1 simulated through-lens contrast** for body
text and ≥3:1 for secondary text — automated in the existing contrast
regression test alongside the normal WCAG check (amber-on-black passes both:
#ffb000 on #000 is ≈11:1 unfiltered and ≈10:1 through-lens).

### Presets

| Preset | Base | Intent | Key rules |
|---|---|---|---|
| **Paper** | current light, refined per §2 | daytime default | unchanged semantics |
| **Dark** | current dark, refined | evening general | shadows off, borders carry structure |
| **Pitch black** | `#000` canvas, `#0a0a0a` surfaces | OLED, minimal photon flux, night logging | no shadows; hairline `#1c1c1c` dividers; desaturated ink `#d9dcd8`; accents dimmed one step |
| **Amber (glasses)** | `#000` canvas | compatibility for dark-amber eyewear; also a user-selectable blue-minimized display | long-wavelength-dominant RGB palette: text `#ffb000`, dim text `#b37c00`, action `#ff8c42`, danger `#ff5c33`; the token set minimizes the commanded blue channel, while confidence/status re-encode as luminance steps + the existing band *patterns* (solid/striped/dashed) |
| **High contrast** | dark-based | low-vision / prefers-contrast | pure `#fff` on `#000`, ≥7:1 everywhere, 2px focus rings, underlined links, patterns mandatory |

`Reduce stimulation` stays an orthogonal modifier (works with every preset),
as does the OS-follow Auto option. Existing `[data-theme]` token plumbing
extends from 2 values to 5 — the architecture already supports it; the work
is defining disciplined token sets, not re-plumbing.

### Rhythm-linked switching (circadian tie-in)

Opt-in rule, off by default: "switch to *Amber* starting N hours before my
predicted sleep onset; return to *Auto* after wake." This is the one theme
feature only ZeitBoard can do — the trigger is the estimator's forecast, so
it tracks the user's drift instead of a fixed clock. Honesty rules apply: if
the estimate is refused/empty, fall back to a fixed civil-time schedule (the
user sets it) and say so in Settings. Switching is a local, reversible
display action — it does **not** go through the approval queue (it is not a
schedule/health mutation; an ADR will record this boundary explicitly).

### Agentic surface

Theme state and the active rule become part of the agent-readable settings
projection ("amber mode is on until ~06:40 predicted wake"), and a voice
user may say "switch to amber mode" — allowlisted as a direct local display
action per the same ADR (reversible, non-health), with the audit line kept.
Everything else stays propose-only.

## 4. Slices

| Slice | Scope | Notes |
|---|---|---|
| **U-A Token consolidation** | radius/spacing/type-scale tokens; semantic color role reassignment (sage retracts; blue/amber/purple activate); dark-theme parity | pure refactor, contrast test must stay green |
| **U-B Density pass** | Overview status band + cycle strip; kill empty tiles; compact page headers; data-display type sizes; actogram hero budget on Rhythm | biggest visible payoff |
| **U-C Theme manager** | preset registry (5 presets + reduced modifier), Settings UI with live preview swatches, Pitch black + Amber + High contrast token sets, through-lens contrast test | ships the user-visible feature |
| **U-D Rhythm-linked switching + agent surface** | forecast-triggered preset rule with civil-time fallback; settings projection + allowlisted display action | needs the small ADR (direct-action boundary) |

Sequencing: U-A first (everything else builds on tokens), U-B and U-C can
interleave, U-D last. Each slice keeps the WCAG regression test green and
extends it (U-C adds the simulated dark-amber assertions; U-B adds a "no
container over 40% padding" review checklist item rather than an automated
rule).

## 5. Implementation audit (2026-07-18)

- **U-A delivered.** Shared spacing, radius, and type tokens now drive the
  reworked route styles; status colors have semantic jobs; obsolete Overview
  card rules were removed.
- **U-B delivered after structural remediation.** The initial pass changed
  CSS density but left the old metric-card composition in place. The follow-up
  rebuilt Overview as one surface with a dominant state band, source-matched
  cycle strip, three fact rows, confidence, conditional attention, and trust.
  Rhythm's actogram is a full-width visual rather than another generic panel.
- **U-C delivered.** Settings exposes Auto plus visual Paper, Dark, Pitch
  black, Amber, and High contrast choices with live swatches and an independent
  reduced-stimulation control. Token-level normal contrast, simulated
  dark-amber contrast, and blue-channel-limit checks cover the presets.
- **Architecture remediation delivered.** The former 2,512-line multi-screen
  module was split by route, reusable proposal/rhythm components were
  extracted, Wails lookup was centralized in the data layer, and repository
  lint now enforces those boundaries. See `frontend-architecture.md`.
- **U-D delivered (ADR-0021).** Rhythm-linked switching uses the structured
  forecast with an honest civil-time fallback. Appearance changes are direct,
  reversible local display actions; the agent-readable endpoint remains
  separately deferred as recorded by the ADR.

## 6. Acceptance

1. Overview at 1440×900 shows rhythm data above the fold occupying ≥70% of
   the content column; no tile is emptier than it is full.
2. Sage appears only as primary-action and awake-state color; sleep visuals
   are blue-family; a screenshot diff of Rhythm shows three distinguishable
   semantic hues plus neutrals.
3. All five presets pass the standard contrast regression; Amber and High
   contrast additionally pass the simulated through-lens assertion; Amber's
   computed token colors retain the existing blue-channel limit. No spectral
   cutoff or treatment effect is claimed from CSS values.
4. Pitch black emits no non-black canvas pixels at rest beyond hairlines and
   content ink.
5. Reduced stimulation composes with every preset; charts stay readable in
   all ten combinations (5 presets × reduced on/off) — screenshot matrix
   reviewed manually, patterns carry state everywhere hue cannot.

The structural acceptance check measured the 1440×900 Overview surface at
approximately 693px high (about 77% of the viewport) with one visible primary
container and no nested `.panel` or `.metric-card`. The Rhythm actogram measured
approximately 568px high at the same viewport with no generic panel above the
fold. Final verification commands and environment results are recorded in
`verification.md`.

## 7. Slice U-E (delivered 2026-07-22): chronological hover time probe

Every chronological surface answers "what exact time is under my cursor."
Hovering the actogram, the drift chart, the Overview cycle strip, or a
calendar timeline shows a **time probe**: a theme-token hairline at the
cursor's time position plus a small chip with the exact civil time in
tabular numerals (minute precision — the charts' native resolution).

### Interaction

- **Actogram (double plot):** cursor x maps through the row's 48 h scale;
  the chip reads `Sat Jul 18 · 03:24` — the *second* plot resolves to the
  next civil day, exactly as the double plot implies. Hovering a forecast
  row appends the qualifier `predicted`; the probe never turns a range into
  a point claim beyond the cursor position itself.
- **Drift chart:** a vertical hairline snaps to the nearest cycle; the chip
  reads the cycle's date + observed onset (and fitted onset when they
  differ), e.g. `Jul 12 · onset 02:47 (fit 02:52)`.
- **Cycle strip / calendar:** same mapping at their own scales; the strip
  adds `~` before times inside predicted spans.
- The probe follows `pointermove`, disappears on leave, and renders on an
  overlay layer (no band re-render; CSS transform positioning only).
  Reduced stimulation keeps the hairline and chip but drops any transition.
- Touch (Android later): press-and-hold summons the probe; release hides.
- Keyboard/non-visual: the probe is a pointer affordance; exact values
  remain available in the existing sr-tables (per the visual-first stance,
  the probe adds precision for sighted users without becoming the only
  path). Focused-chart arrow-key stepping is a possible follow-up, not in
  scope.

### Data honesty requirement

The chip's time must come from **structured row data, never parsed labels**:
actogram/strip rows gain an ISO civil date (+ zone) in the DTO so
`timeAtCursor(x, row)` is arithmetic on real instants. Formatted prose is
not reverse-engineered (the §2.3 rule Codex's audit codified). Forecast
rows keep their `predicted` qualifier; zone is the row's own display zone.
The network-facing server/MCP projection remains an explicit allowlist rather
than serializing the full core presentation struct. It retains the ISO civil
date needed for chronology but excludes raw zone identifiers; synced clients
therefore do not invent a zone-dependent marker when that field is redacted.

### Acceptance

1. On each surface, a probe chip appears within one frame of hover and
   tracks the cursor at minute precision; values match the sr-table row for
   the same position.
2. Second-plot hours (24–48) resolve to the following civil day.
3. Forecast/predicted positions are qualified in the chip text.
4. Works in all five presets (chip uses paper/ink tokens; hairline uses the
   chart grid token) and under reduced stimulation (no motion).
5. No layout shift, no band re-render on pointermove (verified via React
   profiler or render counters in tests).

### Delivery evidence

- Core projection rows and the current-time marker carry validated ISO civil
  dates and row-specific IANA zones. Frontend normalization rejects missing or
  malformed local anchors; synced projections require civil dates while
  preserving the server/MCP zone redaction.
- Overview, actogram, drift, and calendar probes use one imperative overlay
  controller. CSS transforms move the hairline, activating one probe dismisses
  the prior probe, and React profiler tests show no pointer-move render.
- Actogram tests cover the 24–48 h following-day mapping and forecast
  qualification. Drift snaps to the nearest observed cycle and reports both
  observed and fitted onset when they differ.
- Runtime review covered 1440×900 and a 390×844 narrow override, edge chips,
  internal actogram scrolling, Paper, Dark, Pitch black, Amber, High contrast,
  and reduced stimulation. Paint containment and fixed-layout hidden tables
  prevent chart or screen-reader content from widening the narrow page.

## 8. Slice U-F (delivered 2026-07-22): residual ruled surfaces

The earlier `U-B delivered` label applied to Overview and the primary Rhythm
visuals, not to every route. U-F completed the residual structural remediation
identified by the follow-up audit:

- **Data Sources is source-first.** A two-column provenance ledger now precedes
  entry tools, reports current local/sync state, and points calendar ownership
  to the delivered Calendar workspace instead of calling it future work. Manual
  entry and contract import share one divided workspace; the sleep ledger stays
  ruled. There is no generic `.panel` wrapper in the route.
- **Sharing is honest and dense.** The route first says that no trusted view is
  being shared and that transport is not connected. Relationship templates are
  explicitly examples, not fake people or active links. A compact ledger and a
  guardrail column replace circular avatars and profile cards. Medication,
  diagnosis, raw activity, location, private calendar text, and rhythm-marker
  notes remain outside the trusted-link boundary.
- **Android sections are ruled, not card-wrapped.** Status, latest sleep,
  Health Connect, correction, medication, display, data-source, and privacy
  sections now use top/bottom rules with content-level spacing. The generic
  `Panel` composable is gone; primary actions retain the 48 dp Android target
  and other controls retain at least 44 dp. The large-width review also found
  and fixed the `fillMaxWidth().widthIn(...)` modifier ordering bug, so the
  intended 680 dp content cap now measures 2040 physical pixels at 480 dpi.
- **Narrow chrome is quiet.** Eight desktop route icons remain horizontally
  reachable below 700 px without exposing a browser scrollbar.

Repository UI lint now rejects a generic panel in Data Sources or Sharing,
rejects avatar/active-link presentation in the sharing preview, and rejects a
return of Android's generic `Panel` wrapper. Focused rendering tests verify
source-first ordering, truthful Calendar status, template semantics, and the
absence of the retired structures.

Runtime review covered the desktop routes at the native 1280x720 viewport and
390x844 in Paper, system Dark, Amber, and High contrast, with no page-level
horizontal overflow. An exact 1440x900 layout measurement also found no
overflowing main-content descendants. The named Pixel 10 Pro XL API 36.1
emulator covered all four routes in portrait and Settings at the large
landscape width after a fresh APK install; no app crash, ANR, clipping, or
obscured control was observed.

This closes the specifically recorded residual-surface slice. It does not claim
that deferred product capabilities are present: trusted-link transport,
Android sync and More destinations, tablet/fold two-pane navigation, and a
full TalkBack/screen-reader participant walkthrough remain separate roadmap
work.

## 9. Slice U-G (delivered 2026-07-22): proposal and compact-control density

A runtime follow-up found that U-F's residual-route closure did not cover two
shared desktop surfaces. Tasks nested a proposal card inside an approval card
and placed the related no-safe-window state in a separate card. Approvals
centered individually rounded proposal cards in an otherwise full-width queue.
At 390x844, the first Tasks proposal measured 431px high and the two Approvals
proposals measured 355px and 331px. The phone-width Settings appearance picker
also consumed 821px before any other setting. Finally, the icon-only narrow
navigation hid its text labels from the accessibility tree.

The follow-up applies the same one-surface rule to these shared components:

- Approvals is one bounded queue with ruled proposal rows. Origin retains a
  semantic edge color, confidence is written as text as well as a three-part
  meter, reasons are neutral inline facts rather than boxed chips, and actions
  occupy a dedicated column on wide screens or one compact row on narrow
  screens.
- Tasks keeps the current proposal and the related not-proposed explanation in
  one approval surface. Approvals renders no-safe-window tasks as a ruled
  title/reason/next-action ledger without adding punctuation to backend copy.
- The phone appearance selector uses a two-column compact preview grid while
  preserving every preset's hint and radio semantics. Its measured height is
  347px at the same narrow viewport.
- Every primary-navigation link now has an explicit accessible name even when
  its visible label is hidden; the Approvals name includes the current pending
  count. Native hover titles identify compact desktop-rail icons.

After the change, the first Tasks proposal measured 299px high, the two
Approvals rows measured 220px and 217px at 390x844, and the same rows measured
144px and 143px at 1440x900. Both viewports retained zero page-level horizontal
overflow. The named Pixel 10 Pro XL emulator was rebuilt and rechecked across
all four portrait routes plus large-width Settings; this slice changes no
Android source.

## U-H — navigation consolidation (planned)

From the 2026-08-04 UI guideline review
([disposition](ui-guideline-review-2026-08-04.md), finding 6). The desktop
exposes eight equal-weight primary destinations plus Settings and the assistant
rail. That is too much undifferentiated navigation for someone operating under
fatigue, which is the condition the product is for.

Target shape: **Home, Plan, Rhythm, Log, Sharing**, with Data Sources, Reports,
Settings, Assistant, and Help in a utility group.

- **Plan** absorbs Calendar, Tasks, and Approvals as tabs. Approvals keeps a
  pending badge rather than a permanent destination — it is empty most of the
  time, and a destination that is usually empty trains people to skip it.
- **Log** absorbs sleep/wake, medication, context markers, and corrections.
- **Rhythm** keeps actogram, drift, and sources as tabs, with statistics behind
  a details disclosure.

**Inherited from ADR-0034 (2026-08-08).** The operational view landed on
Overview rather than as a ninth destination, precisely to avoid widening the
navigation this slice is meant to narrow. It replaced Overview's "useful task
window" tile, which it subsumes. When U-H lands, Overview becomes **Home** and
the view goes with it; it is the answer to "what is my day", not "what is
booked", so it does not belong under Plan.

Sequencing note: this is deliberately *not* bundled with a correctness change.
It touches every screen module, the UI-standards lint, and most of the frontend
suite, and mixing it with a behavioural fix would make both harder to review.

Not in scope: full user rearrangement of navigation, which the guideline also
advises against, and the pinnable secondary destination, which is declined until
this slice settles the shape.
