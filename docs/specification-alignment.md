# Specification alignment

The user-supplied specifications in this directory refine the product beyond
the original phase-one scaffold. They are authoritative design and validation
inputs, subject to the versioned contracts. They are not a claim that every
described phase-two or research feature is already implemented.

## Phase-one implementation

- Conventional Go code owns normalization, correction application, rhythm
  estimation, forecasting, scheduling, medication-relative timing, and sharing
  projection. No language model is in the authoritative path.
- The estimator uses a documented Theil-Sen sleep-start trajectory, reports
  drift per observed sleep cycle, widens forecast ranges with horizon, and uses
  the v1 contract refusal codes.
- Imported observations remain immutable. Manual corrections are append-only
  and reversible through superseding records.
- Fixed events are immutable scheduler inputs. Flexible work receives proposals
  with explanations rather than silent movement.
- Trusted views are static synthetic projections, render only allowlisted
  fields, always show the contract notice, and disclose no private source data.
- Windows collection is deliberately minimal and privacy-safe. Android requests
  only Health Connect sleep access and retains a fixture repository fallback.

## Deferred analysis work

- Probabilistic multi-source sleep/wake inference and calibrated boundary-error
  metrics.
- Explicit missingness records, source reliability learning, conflict scoring,
  forced-schedule qualification, and travel/illness/intervention markers.
- Multi-window change-point classification and operating-state history.
- State-space, particle-filter, Bayesian, physiological-signal, or
  biomarker-calibrated estimation.
- Optional language-model summaries, task parsing, voice extraction, and
  clinician-report drafting. These require the safety evaluation suite before
  they can be enabled.

## Deferred product and validation work

- Full onboarding, Reports, correction inspectors, proposal acceptance/undo,
  dark theme, reduced-stimulation controls, and the complete accessibility
  acceptance matrix from `ui-ux-design.md`.
- A real sharing relay, passcodes, access logs, and remote revocation transport.
- Emulator/device validation, real-world pilot validation, calibration plots,
  benchmark reporting, and the complete 20-scenario synthetic generator.

Phase two should implement the trust loop first: source conflict visibility,
manual correction with forecast diff and undo, refusal UI, and deterministic
task proposal review. This preserves the specifications' central safety rule:
the software must decline to guess when evidence is inadequate.
