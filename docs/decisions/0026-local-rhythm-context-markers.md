# ADR-0026: Local rhythm context markers

- Status: accepted
- Date: 2026-07-22

## Context

The disease-management phase needs context for travel, illness, interrupted
sleep, and externally forced schedules before it can compare medication
adherence with rhythm. Without separately visible context, an association view
could invite a causal reading that the observations do not support.

The AASM actigraphy guideline conditionally supports actigraphy as an estimate
of sleep parameters in circadian rhythm sleep-wake disorders; it does not turn
an actogram into a diagnosis or causal analysis. The Consensus Sleep Diary
supports standardized prospective self-report, while STROBE explains that
confounding can produce associations that are not causally interpretable.
ZeitBoard therefore needs a narrow self-report annotation, not another
estimator input or a diagnostic record.

## Decision

1. A rhythm marker is an immutable local record with exactly one kind:
   `travel`, `illness`, `disruption`, or `forced_schedule`. Product labels say
   that the context is self-reported. No kind represents a diagnosis,
   treatment, medication effect, or inferred event.
2. Each marker owns a start instant, optional end instant, explicit IANA zone,
   optional private note of at most 500 characters, and manual/user-reported
   provenance. The desktop accepts only present or retrospective records.
   Nonexistent civil times are rejected and a repeated civil time resolves to
   its first occurrence.
3. Records cannot be updated. A mistaken marker is permanently erased and a
   replacement may be appended. Erasure requires exact typed `DELETE`, removes
   the SQLite row, checkpoints and truncates the WAL, and vacuums free pages.
   This is distinct from append-only suppression of sleep or medication
   evidence.
4. Markers are not passed to estimation, scheduling, reminder, calendar,
   server sync, trusted-view, MCP, assistant, or logging paths. They cannot
   alter a forecast. The only interchange surface is an owner-initiated strict
   `rhythm-marker-set` v1 JSON export.
5. The Rhythm screen has a dense Context ledger with no generic card wrappers.
   The actogram renders shape-coded marker starts only on rows with the same
   civil date and explicit IANA zone, duplicates them on the second plot, lists
   only marker kinds present in the plotted rows, and includes full context in
   its screen-reader table. Markers outside the plotted date/zone coordinates
   are counted rather than silently dropped.
6. Forecast rows are hidden by default. Marker copy states that the records do
   not change the estimate, establish cause, diagnose, or recommend treatment.
7. M-C may use marker records only as an explicit confounder list beside an
   observational adherence-versus-rhythm table. It must not say or imply that
   a medication or marker caused a rhythm change.

## Consequences

- The later adherence view has explicit user-controlled context before any
  comparison is shown.
- Private notes remain on the local device unless the owner deliberately
  exports them.
- Hard erasure is byte-tested against both the SQLite database and WAL.
- An interval currently produces a start marker plus its textual range; a
  future longitudinal report may add a duration overlay without changing the
  record contract.
- Medication dose markers and the clinician report remain M-C work.

[AASM actigraphy guideline]: https://pmc.ncbi.nlm.nih.gov/articles/PMC6040807/
[Consensus Sleep Diary]: https://pmc.ncbi.nlm.nih.gov/articles/PMC3250369/
[STROBE explanation]: https://pmc.ncbi.nlm.nih.gov/articles/PMC2020496/
