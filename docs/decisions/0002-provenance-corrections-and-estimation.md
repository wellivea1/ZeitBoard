# ADR 0002: Provenance, corrections, and initial estimation

- Status: accepted
- Date: 2026-06-15

## Decision

Store source observations append-only. Model acquisition method separately from evidence status. Manual corrections target existing entities or boundaries and create supersession records; they never rewrite imported data.

The initial estimator selects recent principal sleep episodes, excludes naps, indexes identifiable cycles across gaps, and applies Theil-Sen regression to absolute UTC sleep-start instants. It reports observed sleep-start drift relative to 24 hours, not intrinsic circadian period. Median sleep duration and robust residual dispersion form forecast windows that widen with horizon.

Estimation returns a typed refusal when support is insufficient, cycle indexing is ambiguous, or conflicts prevent a defensible result.

## Consequences

- Effective values can change while source evidence remains auditable.
- DST and civil-date boundaries cannot create artificial drift.
- The estimator is conventional, deterministic, replaceable, and explicit about uncertainty.

