# Accessibility — visual-first, accessible wherever it doesn't cost the visuals

ZeitBoard is **visual-first**. Its primary audience is people with **sighted**
Non-24-hour sleep-wake disorder, and the visual feedback — the actogram, the drift
chart, the calendar overlays — is the heart of the product. **Visual feedback is
never sacrificed for accessibility.**

At the same time, blindness is a leading *cause* of Non-24 (when little or no light
reaches the circadian clock the rhythm free-runs), so blind and low-vision users are
real and worth supporting. The rule is simple: **every element that can reasonably be
made accessible should be — wherever doing so does not compromise aesthetics or
functionality.** Accessibility is a strong default, not a veto over the visual design.

The detailed spec is [`ui-ux-design.md` §18 and §3.6](ui-ux-design.md); the binding
repository rules are in [`../AGENTS.md`](../AGENTS.md).

## What we do wherever it's reasonable (and it almost always is)

- **Keyboard-operable.** Every action is reachable and operable by keyboard, in a
  logical focus order, with a visible focus indicator and no traps. (This costs the
  visuals nothing and is expected on every surface.)
- **Named.** Every control, icon-only button, input, tab, and chart has an accessible
  name (label / `aria-label` / content description). No unlabeled affordances.
- **Never color alone.** Status, confidence, conflict, and origin are also carried by
  text and/or shape, so the UI is usable in grayscale and by color-blind users — this
  improves the visual design rather than competing with it.
- **A text equivalent for charts where it doesn't degrade the design.** Charts ship a
  screen-reader table or text alternative carrying the same information (e.g. the
  actogram and drift `sr-table`s). These are visually hidden, so they add non-visual
  access at no cost to the visuals — keep doing this.
- **Announce meaningful changes.** State changes the user causes or needs to know (an
  approval applied, a refusal) are announced via polite live regions, without stealing
  focus.
- **WCAG 2.2 AA** for contrast, target size, and reflow — enforced where computable
  (the desktop contrast regression test,
  `apps/desktop/frontend/src/theme/contrast.test.ts`).
- **Android** mirrors these with Compose semantics and content descriptions (TalkBack).

## The trade-off rule

When an accessibility affordance would force a worse-looking or worse-functioning
visual experience, **the visual experience wins**, and the accessible path is provided
by another means (a text equivalent, a description) rather than by compromising the
chart. Do not gate shipping a visual feature on full non-visual parity. Do add the
accessible equivalents that don't cost aesthetics or functionality — which, in
practice, is most of them.

## Definition of done (per change)

A UI change includes a keyboard pass and the accessible affordances above wherever they
don't compromise the visuals (accessible names, non-color-only cues, and a chart text
equivalent when a chart is added). It is not blocked on full screen-reader parity for
inherently visual features.
