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
  walk-forward point-error / hit-rate / calibration measurement.
- Observations are immutable and corrections are append-only and reversible
  through superseding records — locally (desktop SQLite, ADR-0013), on the
  self-hosted backend (sync log, ADR-0009), and in every read path
  (`ApplySleepCorrections` + overlap resolution).
- The desktop runs on the user's real entered sleep data with honest
  empty/refusal states; export and hard erasure are implemented (ADR-0014);
  opt-in backend sync round-trips contract-shaped records (ADR-0015).
- Fixed events are immutable scheduler inputs; proposals carry contract
  explanation codes and honest unplaced reasons.
- Trusted views are static synthetic projections, render only allowlisted
  fields, and disclose no private source data (unchanged since phase one).
- Windows collection remains deliberately minimal; Android requests only
  Health Connect sleep access and retains a fixture fallback.

## Partially implemented (UI ahead of or behind data)

- **Approvals:** the desktop queue is engine-backed but in-session; the
  backend persists proposals with one-use approval tokens. The two are not yet
  unified (roadmap slice 1).
- **Rhythm "Sources" tab and Data Sources:** still render synthetic
  conflict/correction-preview/refusal fixtures beside real data (roadmap
  slice 5).
- **Tasks:** user-owned tasks are real (contract, local CRUD, real Tasks
  screen; the scheduler plans only stored open tasks — ADR-0018), but they are
  per-device: task sync and write-back of approved placements remain future
  work.

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
  clinician-report drafting beyond the implemented propose-only assistant;
  these require the safety evaluation suite before they can be enabled.

## Deferred product and validation work

- Full onboarding, Reports, the clinical actogram + clinician PDF export
  (§3.6), and the complete accessibility acceptance matrix from
  `ui-ux-design.md`.
- A real trusted-view sharing transport (passcodes, access logs, remote
  revocation); the former relay design survives only as input to that path.
- Local desktop DB encryption at rest and OS credential storage.
- Emulator/device validation, real-world pilot validation, calibration plots,
  benchmark reporting, and the complete 20-scenario synthetic generator.

The specifications' central safety rule is preserved everywhere: the software
declines to guess when evidence is inadequate, and every automation path ends
in a recorded human approval.
