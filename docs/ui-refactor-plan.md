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

Amber/orange blue-blocking glasses are standard evening practice in Non-24
and DSPD management: dark amber lenses transmit long wavelengths nearly
fully but cut blue (~400–490nm) to near zero and attenuate most green
(~495–550nm) heavily. Consequences for UI seen through them:

- Blue elements go **black** (invisible); green loses most luminance;
  white dims into amber. Any information encoded as blue-vs-anything or
  green-vs-anything **disappears**.
- The highest through-lens contrast available is **bright amber/orange
  (≈590–610nm) on true black** — the classic amber terminal, which is why
  that aesthetic exists.

Define **through-lens luminance** for testing: `L' = 0.2126·R +
0.7152·(g·G) + 0.0722·(b·B)` with dark-amber transmission factors `g≈0.25,
b≈0.02`. The Amber preset must hold **≥7:1 through-lens contrast** for body
text and ≥3:1 for secondary text — automated in the existing contrast
regression test alongside the normal WCAG check (amber-on-black passes both:
#ffb000 on #000 is ≈11:1 unfiltered and ≈10:1 through-lens).

### Presets

| Preset | Base | Intent | Key rules |
|---|---|---|---|
| **Paper** | current light, refined per §2 | daytime default | unchanged semantics |
| **Dark** | current dark, refined | evening general | shadows off, borders carry structure |
| **Pitch black** | `#000` canvas, `#0a0a0a` surfaces | OLED, minimal photon flux, night logging | no shadows; hairline `#1c1c1c` dividers; desaturated ink `#d9dcd8`; accents dimmed one step |
| **Amber (glasses)** | `#000` canvas | worn-glasses evenings; also a zero-blue-emission mode on its own | all foregrounds ≥570nm: text `#ffb000`, dim text `#b37c00`, action `#ff8c42`, danger `#ff5c33`; **no blue or green pixels anywhere**, including charts — confidence/status re-encode as luminance steps + the existing band *patterns* (solid/striped/dashed), which were designed exactly for hue-free reading |
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
extends it (U-C adds the through-lens assertions; U-B adds a "no container
over 40% padding" review checklist item rather than an automated rule).

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
  reduced-stimulation control. Token-level normal, through-lens, and zero-blue
  checks cover the presets.
- **Architecture remediation delivered.** The former 2,512-line multi-screen
  module was split by route, reusable proposal/rhythm components were
  extracted, Wails lookup was centralized in the data layer, and repository
  lint now enforces those boundaries. See `frontend-architecture.md`.
- **U-D remains open.** Rhythm-linked switching and agent-readable display
  actions require the planned direct-display-action ADR and are not implied by
  U-A..U-C completion.

## 6. Acceptance

1. Overview at 1440×900 shows rhythm data above the fold occupying ≥70% of
   the content column; no tile is emptier than it is full.
2. Sage appears only as primary-action and awake-state color; sleep visuals
   are blue-family; a screenshot diff of Rhythm shows three distinguishable
   semantic hues plus neutrals.
3. All five presets pass the standard contrast regression; Amber and High
   contrast additionally pass the through-lens assertion; Amber renders zero
   sub-570nm pixels (assert: computed styles contain no color with B>10% or
   G-dominant channels in the amber token set).
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
