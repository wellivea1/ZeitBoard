# ADR 0021: Display settings are direct local actions; rhythm-linked night mode

- Status: accepted
- Date: 2026-07-18
- Completes ui-refactor-plan.md slice U-D.

## Decision

1. **Appearance changes bypass the approval queue.** Switching presets or
   toggling the night rule is a local, instantly reversible display action on
   non-health state. The propose-only invariant (ADR-0003/0006) governs
   schedule and health mutations; extending it to display settings would make
   the queue noise. This ADR records that boundary explicitly.
2. **Rhythm-linked night mode.** An opt-in local rule: engage a night preset
   (Amber by default; Pitch black or Dark selectable) starting N hours before
   the **predicted** sleep onset and release it at the predicted wake. The
   desktop exposes `GetAppearanceClock` (structured onset/wake from the local
   estimate, honest status otherwise); a pure, tested evaluator applies the
   window. The trigger is the forecast, so the switch drifts with the user.
3. **Honest fallback.** When the estimator refuses or has no data, the rule
   falls back to user-set fixed civil times, or stays inactive — and Settings
   says which of these is happening. The stored preference is never mutated
   by the rule; the override is display-only and releases cleanly.
4. **Agent surface: deferred with rationale.** The MCP connector runs against
   the backend, which cannot reach a device's display. Agent-driven switching
   ("switch to amber mode" by voice) needs a desktop-local agent endpoint;
   the Wails bindings introduced here are that future surface's seed. Until
   it exists, agent access to appearance state is not implemented rather
   than half-implemented.

## Consequences

- The one theme feature only a circadian planner can offer ships: night mode
  that tracks a free-running rhythm instead of a wall clock.
- Evaluation is client-side and local; no new data leaves the device (the
  forecast times already exist locally).
- Residual: with the app closed nothing switches — this is a display rule,
  not an OS service.
