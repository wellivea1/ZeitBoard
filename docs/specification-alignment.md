# Specification alignment

The user-supplied specifications in this directory refine the product beyond
the original scaffold. They are authoritative design and validation inputs,
subject to the versioned contracts. They are not a claim that every described
feature is already implemented. Current build status in one place is
[`roadmap.md`](roadmap.md) ("Where things stand"); this file maps the *analysis
and UI specifications* to what implements them.

## Implemented

- Conventional Go code owns normalization, correction application, rhythm
  estimation, forecasting, scheduling, medication-relative timing, and sharing
  projection. No language model is in the authoritative path: the assistant
  and agent layers emit allowlisted actions that the server resolves into
  pending proposals (ADR-0010/0012).
- The estimator uses a documented Theil-Sen sleep-start trajectory, reports
  drift per observed sleep cycle, widens forecast ranges with horizon, and
  uses the v1 contract refusal codes. `estimation.Backtest` provides
  walk-forward point-error / hit-rate / calibration measurement, and
  `core/simulate` implements the validation plan's seeded synthetic generator
  (latent truth retained) with a 12-scenario estimator validation suite and
  benchmark table (ADR-0019).
- Observations are immutable and corrections are append-only and reversible
  through superseding records — locally (desktop SQLite, ADR-0013), on the
  self-hosted backend (sync log, ADR-0009), and in every read path
  (`ApplySleepCorrections` + overlap resolution).
- The desktop runs on the user's real entered sleep data with honest
  empty/refusal states; export and hard erasure are implemented (ADR-0014);
  opt-in backend sync round-trips contract-shaped records (ADR-0015), and
  erasure propagates through server tombstones (ADR-0017).
- The desktop UI implements the visual refactor's U-A..U-C boundary: one-surface
  Overview with a source-matched cycle strip, a full-width Rhythm visual,
  semantic theme tokens, and an appearance manager with Auto plus five presets.
  Screen/data/style boundaries are enforced as described in
  [`frontend-architecture.md`](frontend-architecture.md).
- Fixed events are immutable scheduler inputs; proposals carry contract
  explanation codes and honest unplaced reasons.
- Trusted views are static synthetic projections, render only allowlisted
  fields, and disclose no private source data (unchanged since phase one).
- Windows collection remains deliberately minimal; Android requests only
  Health Connect sleep access and retains a fixture fallback.

## Partially implemented (UI ahead of or behind data)

- **Approvals:** local scheduler proposals and backend assistant/agent
  proposals are both real, and the desktop decides backend proposals with
  one-use tokens. They still appear as distinct queue sections; batch review,
  surfaced expiry/history, and approved-placement write-back remain open.
- **Rhythm "Sources" tab and Data Sources:** driven by real local data in the
  desktop app (real refusal, real correction history, real per-source
  composition, real sync status); synthetic previews remain only in the
  labeled browser-preview fixture mode. A real cross-source conflict list
  awaits an engine-surfaced overlap DTO once multiple sources exist.
- **Tasks:** user-owned tasks are real and synced (contract, local CRUD, real
  Tasks screen, scheduler plans only stored open tasks — ADR-0018; cross-device
  revision sync with erasure-grade deletion — ADR-0020). Write-back of
  approved placements remains future work (Phase 3c).
- **Appearance:** U-A through U-C are implemented. U-D rhythm-linked preset
  switching and the agent-readable direct display action remain deferred until
  their explicit ADR defines that reversible-action boundary.

## Deferred analysis work

- Probabilistic multi-source sleep/wake inference and calibrated
  boundary-error metrics (first consumer: Takeout/"My Activity" import,
  roadmap slice 7 — gated by the backtest).
- Explicit missingness records, source reliability learning, conflict scoring,
  forced-schedule qualification, and travel/illness/intervention markers.
- Multi-window change-point classification and operating-state history; an
  explicit linear-misfit signal and phase-dependent sleep duration (pursued as
  real-history backtest results justify, roadmap slice 3).
- State-space, particle-filter, Bayesian, physiological-signal, or
  biomarker-calibrated estimation.
- Optional language-model summaries, task parsing, voice extraction, and
  clinician-report drafting beyond the implemented propose-only assistant
  (whose desktop chat surface now ships — §4 rail with redacted context and
  provider disclosure); these require the safety evaluation suite before they
  can be enabled.

## Deferred product and validation work

- Full onboarding, Reports, the clinical actogram + clinician PDF export
  (§3.6), and the complete accessibility acceptance matrix from
  `ui-ux-design.md`.
- A real trusted-view sharing transport (passcodes, access logs, remote
  revocation); the former relay design survives only as input to that path.
- Local desktop DB encryption at rest and OS credential storage.
- Emulator/device validation, real-world pilot validation, and calibration
  plots. Of the plan's 20 synthetic scenarios, the 12 sleep-timing scenarios
  plus boundary-level corruption checks are implemented with benchmark
  reporting (ADR-0019); scenarios 11–15 and 18–19 await the multi-source
  streams of roadmap slice 7.

The specifications' central safety rule is preserved everywhere: the software
declines to guess when evidence is inadequate, and every automation path ends
in a recorded human approval.
