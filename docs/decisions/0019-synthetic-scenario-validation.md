# ADR 0019: Synthetic scenario validation (simulated Non-24 patterns)

- Status: accepted
- Date: 2026-07-09
- Implements the sleep-timing portion of the validation plan
  (`docs/Circadian Analysis Validation and Synthetic Data Plan.docx` §3–4);
  interim stand-in for roadmap slice 3 (real 2021–2023 history) and the
  validation gate for slice 7 (activity→sleep inference).

## Context

The estimator's accuracy was tested against two hand-built histories (clean
drift, one phase jump). The repo's own validation plan specifies far more: a
reproducible synthetic-data generator that **retains true latent state** and
twenty scenarios with expected behaviors. Real-history validation (slice 3)
needs the owner's chart transcription and interactive time; artificial data
can pay down most of the validation risk now.

Prior art considered: the sleepdiary / Zeitlog–Zeitdex docs include a sleep
pattern *simulator* — a browser teaching widget where a JS loop calls
`add_diary_entry` with normal-distribution jitter, constant daily advance, and
weekday-alarm patterns. Its generative-loop shape is right and informed this
design, but it is a Vue docs component, not a library: no latent-truth
retention, no naps/fragmentation/missingness/deprivation, no zone or DST
handling, and JavaScript while the estimator under test is Go. Porting it
would have tested a re-implementation rather than the real engine.

## Decision

1. **`core/simulate`: a deterministic, seeded Non-24 generator in the Go
   core.** One `Params` value describes a history: latent tau **segments**
   (change points), normal onset/duration jitter, naps, fragmented main
   sleep, forced civil wake with weekend rebound, deprivation events,
   unlogged cycles, and zone shifts. Output is the `[]domain.SleepSession`
   the app would see **plus the latent truth** (per-cycle latent onsets and
   tau), so scores compare estimates to truth, not to noisy observations.
2. **A scenario catalog** encodes the plan's sleep-timing scenarios (1–10,
   16, 17) as named constructors with the plan's parameters.
3. **A validation suite runs the real estimator** (`Estimate` + `Backtest`)
   over every scenario and asserts the plan's expected behaviors as
   measurable claims, with thresholds set as **regression floors calibrated
   to current behavior** — documentation of what is achieved, not
   aspiration. A benchmark-table test logs the full accuracy table.
4. **Scenarios 11–15 and 18–19 are deferred to slice 7**, when multi-source
   streams (wearable/phone/desktop conflict, calendar disruption) exist for a
   consumer to read. Scenario 20 (corruption) is asserted at the boundary:
   duplicates and impossible intervals produce typed refusals, never
   estimates — corruption is rejected, not simulated as history.

## Measured results (2026-07-09, seeds fixed)

| Scenario | Drift error | Backtest median | Hit rate | Confidence |
|---|---|---|---|---|
| S1 stable 24h | 3 s/cycle | 0.25 h | 0.93 | high |
| S2 noisy entrained | 26 s/cycle | 1.05 h | 0.71 | medium |
| S3 tau 24.2 | 26 s/cycle | 0.32 h | 1.00 | high |
| S4 tau 25 | 59 s/cycle | 0.49 h | 0.91 | high |
| S5 tau 26 | 37 s/cycle | 0.26 h | 1.00 | high |
| S6 temporary alignment | n/a (misfit) | 2.66 h | 0.57 | **low** |
| S7 forced wake | 41 s/cycle | 0.20 h | 1.00 | high |
| S8 deprivation | 52 s/cycle | 0.48 h | 1.00 | high |
| S9 naps ×2 | 8 s/cycle | 0.15 h | 1.00 | high |
| S10 fragmented | 27 s/cycle | 0.21 h | 1.00 | high |
| S16 travel (−3 h zone) | 54 s/cycle | 0.16 h | 1.00 | high |
| S17 DST spring-forward | 27 s/cycle | 0.34 h | 1.00 | high |

Reading: on every pattern the linear model *can* describe, drift is recovered
within a minute per cycle — including under naps, fragmentation, a skipped
night, an alarm masking wake times, travel, and DST. On the one pattern it
cannot describe (S6 change points), the failure is honest: error is large,
hit-rate drops, and the estimator itself reports **low** confidence. That
honest-degradation property is now locked by tests.

## Consequences

- Slice 7's "gated by the backtest" criterion has a concrete, reproducible
  gate; slice 3 keeps its own value (real behavior — forced schedules, log
  gaps, transcription noise — is exactly what synthetic data cannot prove).
- The change-point weakness (S6) is now measured, visible, and bounded — the
  deferred multi-window/change-point work has a benchmark to justify itself
  against, per the roadmap's "pursued as real-history backtest results
  justify".
- The generator is UI-free and Wails-free; anything (future import tooling,
  demo seeding, inference validation) can reuse it.
- Simulated sessions are labeled `SourceLabel: "simulated"`; nothing routes
  them into user storage today, and any future seeding path must keep them
  distinguishable from real observations.
