# ADR 0022: Local sleep import and real-history validation

- Status: accepted
- Date: 2026-07-19
- Builds on [ADR-0013](0013-desktop-local-data.md),
  [ADR-0014](0014-local-sleep-export-erasure.md),
  [ADR-0017](0017-server-erasure-tombstones.md), and
  [ADR-0019](0019-synthetic-scenario-validation.md).

## Context

The desktop could persist manual sleep observations and export or erase them,
but it could not safely ingest an owner-controlled history file. The estimator
had also only been measured on synthetic scenarios. Roadmap slice 3 requires a
strict local import path and a real-history backtest before estimator changes
can ship.

The source material includes overlapping Fitbit exports and handwritten charts.
The digital files can be converted deterministically. Handwritten chart bars
cannot be treated as machine-readable facts without source review and measured
uncertainty. Raw history and converted observation files are private and must
not enter source control, test fixtures, CI, or logs.

## Decision

### Import boundary

The SQLite store owns a strict v1 sleep importer with separate preview and
commit operations:

- JSON accepts an `observation-set` object; CSV accepts one exact canonical
  header. Both are limited to 8 MiB and 20,000 rows.
- Local sleep import accepts only `sleep_episode` observations classified as
  `principal` or `nap`, with acquisition method `file_import` and a non-empty
  `source_record_id`. It rejects unknown classifications rather than silently
  treating them as principal sleep.
- Every row receives an explicit `ready`, `duplicate`, `invalid`, or `imported`
  result. Any invalid row blocks every write. Commit repeats classification in
  one transaction instead of trusting an earlier UI preview.
- `source_record_id` is the idempotency key. An unchanged sleep/provenance
  payload is reported as an exact duplicate; changed timing, classification,
  evidence, or `recorded_at` is a conflict. Conflicts block the batch. A partial
  SQLite insert trigger enforces the same rule for new `file_import` records
  without making migration fail on a legacy store that already has duplicates.
  Such a legacy ambiguity is surfaced and blocks import until the user suppresses
  or erases the duplicate.
- Imported observations use the existing immutable local observation table.
  Existing correction, export, sync, per-record erasure, and erase-all paths
  therefore apply without a second storage model.

The Data Sources screen exposes file selection, automatic dry-run results,
document and per-row errors, and a separate commit action. It also distinguishes
manual and imported provenance in source summaries.

### Owner-assisted conversion

`apps/desktop/cmd/zeitboard-history` provides private local commands for:

- converting finalized `Fitbit Sleep Data*.csv` files while explicitly counting
  excluded superseded directories, out-of-range rows, and exact overlaps;
- generating either a header-only template or a date-populated review ledger,
  then converting only owner-confirmed rows;
- previewing or committing an import to a selected SQLite database; and
- running the baseline/candidate backtest matrix.

Converter IDs are stable SHA-256-derived identifiers. Fitbit civil timestamps
require an explicit IANA zone and reject ambiguous or nonexistent DST times.
Intervals under three hours are reported and classified as naps for review.
One explicit zone applies to each conversion run; travel periods require
owner-confirmed date/zone splits rather than inferred zone changes.
Transcribed records are `file_import` / `user_reported`; digital Fitbit records
are `file_import` / `directly_observed`. Ambiguous transcription times require
an RFC 3339 offset that agrees with the named zone. No handwriting recognition
or silent time inference is claimed. Every chart row must transition from
`needs_review` to `confirmed_sleep` or `confirmed_no_observation`; pending or
invalid rows block the whole conversion, and confirmed no-observation rows are
counted without becoming sleep observations.

For the owner-history run, the owner authorized a source-derived assisted pass
over the original high-resolution Sleep Diary charts. The official report
renderer's 18:00-to-18:00 grid was measured at five-minute increments, and
full-page overlays were visually reviewed. This is a chart transcription, not
OCR and not directly observed evidence. Every one of 241 chart rows is
represented: the complete ledger has 243 statuses because two rows contain two
episodes, with 223 confirmed sleep entries and 20 explicit no-new-episode
entries. Eight episodes overlapping finalized Fitbit records were retained as
calibration evidence rather than imported into the primary benchmark. Their 16
boundaries had 10.0 minutes median and 55.5 minutes P90 absolute error versus
Fitbit; mean error was 23.2 minutes and the maximum was 127 minutes.

Private outputs request mode `0600` and refuse overwrite unless `--force` is
explicit. On Windows, where Go file modes do not replace ACLs, the output must
also live in an owner-controlled directory. The documented workflow keeps files
in an ignored directory or outside the repository.

### Backtest gate

The walk-forward harness now evaluates each holdout independently. An ambiguous
gap refuses the affected holdout and is counted by refusal code instead of
aborting the whole history. Each holdout uses the same maximum-21 recent-episode
fit as the production estimator.

The baseline and two validation-only uncertainty-scale candidates were measured
on 952 principal episodes from the combined history. The import contains 738
Fitbit observations (one classified nap) plus 215 non-overlapping chart
observations:

| Candidate | Evaluations | Refusals | Median error | P90 error | Hit rate | Mean window |
|---|---:|---:|---:|---:|---:|---:|
| Baseline (1.00) | 809 | 136 | 1.71 h | 5.41 h | 0.78 | 14.71 h |
| Tighten to 0.75 | 809 | 136 | 1.71 h | 5.41 h | 0.72 | 13.31 h |
| Tighten to 0.50 | 809 | 136 | 1.71 h | 5.41 h | 0.66 | 11.91 h |

All 136 refusals were `ambiguous_cycle_index`; every eligible holdout was either
evaluated or refused.

**Estimator decision:** reject blanket forecast-window tightening. A 0.75 scale
saved 1.40 hours of mean width but lost 6 percentage points of hit rate; a 0.50
scale saved 2.80 hours but lost 12 points. Neither candidate changed point error.
The production default remains 1.00.

The baseline confidence buckets also justify a measured follow-up, not an
immediate model change: low-confidence median error was 2.19 hours versus 1.42
hours at medium confidence, while high confidence had a 0.61 hit rate across 28
evaluations versus 0.81 across 386 medium-confidence evaluations. An explicit
confidence-window calibration or misfit candidate may be developed next, but it
must beat this committed combined baseline before shipping.
Phase-dependent duration remains deferred until the harness scores duration.

## Consequences

- Local imports are append-only and idempotent without weakening hard erasure.
- The combined real history imports cleanly and produces a reproducible
  aggregate benchmark without committing private observations. Fresh previews
  and commits accepted all 953 observations with zero invalid rows; repeat
  previews classified all 953 as exact duplicates.
- The handwritten-chart coverage gap is closed with an explicit, source-checked
  ledger and measured boundary uncertainty. Direct Fitbit boundaries remain
  authoritative in the benchmark because overlapping chart estimates are used
  only for calibration.
- The measured run used one owner-selected zone. Any unrecorded travel-zone
  changes remain a source-data limitation, not something the converter guesses.
- Broad window tightening is closed as a candidate for now. Calibration/misfit
  work remains open and quantitatively gated.
- Importing an untrusted file can disclose its contents to the local process,
  but not to a network service. Size, shape, row, identifier, transaction, and
  logging boundaries are documented in the threat model.
