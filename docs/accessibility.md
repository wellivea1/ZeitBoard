# Accessibility — visual-first, with equivalent function for everyone

ZeitBoard is **visual-first**. Its primary audience is people with **sighted**
Non-24-hour sleep-wake disorder, and the visual feedback — the actogram, the drift
chart, the calendar overlays — is the heart of the product. That stays true: this
document does not ask for a flattened, lowest-common-denominator interface.

At the same time, blindness is a leading *cause* of Non-24 (when little or no light
reaches the circadian clock the rhythm free-runs), so blind and low-vision users are
real and worth supporting.

The standard is **equivalent function, not identical presentation**:

> Preserve the strength of the visual experience while providing equivalent access
> and equivalent control through semantics, keyboard support, structured tables, text
> alternatives, and agent-accessible functions.

This replaces an earlier framing that read "visual feedback is never sacrificed for
accessibility". That sentence was written to protect the visuals and it does, but as
a rule it says only what accessibility may not do. The standard above says what it
must achieve, which is both clearer and harder to satisfy by accident. Two practical
consequences:

- A chart does not have to *look* different to be accessible. A screen-reader table
  carrying the same information is equivalent function at no visual cost, and that
  remains the preferred pattern.
- **Core tasks must be completable without sight, without a mouse, and without the
  assistant.** This is a slightly higher floor than "wherever it doesn't cost
  aesthetics", and it is adopted deliberately: a core task that is visually elegant
  and unreachable by keyboard is a defect, not a trade-off.

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

## The primary non-visual path: an agent + live voice

Screen-reader tables of a chart are a serviceable *fallback*, not the best non-visual
experience. The intended primary path for blind users is a **conversational agent with
live voice** — "when am I likely awake tomorrow? move my tax block to after I wake,"
spoken back and queued — driving ZeitBoard through a structured, agent-operable
capability layer rather than reading pixels.

This is why it resolves the visual-first tension cleanly: the non-visual experience is a
*separate modality*, so the visual UI never has to be compromised to serve it. The
design constraint that follows is **every feature must be operable by an agent
non-visually** (perceive its state, perform its actions), through the same allowlisted,
*propose-only*, redacted interface the in-app assistant uses (mutations go through the
approval queue; nothing auto-applies; medical-advice refusal holds). Delivery is a local
MCP connector and/or a Claude/ChatGPT skill; a cloud agent is opt-in, off by default,
and gated like any connected backend. ZeitBoard ships no speech stack — voice is the
agent client's job. See [`decisions/0006-agent-accessible-interface.md`](decisions/0006-agent-accessible-interface.md).

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
