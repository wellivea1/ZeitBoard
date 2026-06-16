# Accessibility — screen-reader and non-visual use is a priority

Non-visual operation is a **primary mode** of ZeitBoard, not an add-on. A large
share of people with Non-24-hour sleep-wake disorder are **totally blind** — when
little or no light reaches the circadian clock, the sleep-wake rhythm free-runs —
so blind, screen-reader users are a core audience, right alongside sighted users
reading the app while fatigued. If a screen only works visually, it is broken for
the people who most need this tool.

This document states the commitment and the non-negotiables. The detailed spec is
[`ui-ux-design.md` §18 and §26](ui-ux-design.md); the binding repository rules are
in [`../AGENTS.md`](../AGENTS.md).

## Non-negotiables (every surface, every platform)

- **Keyboard-complete.** Every action is reachable and operable by keyboard alone,
  in a logical focus order, with a visible focus indicator and no traps.
- **Named.** Every control, icon-only button, input, tab, and chart has an
  accessible name (label / `aria-label` / content description). No unlabeled
  affordances.
- **A text equivalent for every visual.** Each chart or visual-only element —
  actogram, drift chart, calendar overlay, status dot, confidence meter — ships a
  screen-reader table or text alternative carrying the same information. A raster
  or positioned-`<div>` chart with no text equivalent is incomplete, not done.
- **Never color or position alone.** Status, confidence, conflict, and origin are
  also carried by text and/or shape; the app is fully usable in grayscale and when
  read linearly by a screen reader.
- **Announce meaningful changes.** State changes the user causes or needs to know
  (current rhythm state, an approval applied, a refusal) are announced via polite
  live regions, without stealing focus.
- **Civil time spoken explicitly.** Times are announced as civil clock times, not
  inferred from visual position on a chart.
- **Android = TalkBack-complete.** Compose semantics, `stateDescription`, and
  content descriptions mirror the desktop guarantees.
- **WCAG 2.2 AA** for contrast, target size, reflow, and the above — enforced where
  computable (e.g. the desktop contrast regression test,
  `apps/desktop/frontend/src/theme/contrast.test.ts`).

## Definition of done (per change)

A UI change is not done until it is operable and understandable with a screen
reader and keyboard, and it ships its non-visual equivalents in the same change.
Every review includes at least a keyboard-only pass; periodic screen-reader
walkthroughs (NVDA / VoiceOver / TalkBack) validate real assistive-technology
behavior that automated checks cannot fully cover.
