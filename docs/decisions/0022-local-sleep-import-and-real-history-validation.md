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
The digital files can be converted deterministically. Handwritten times cannot
be treated as machine-readable facts without owner review. Raw history and
converted observation files are private and must not enter source control, test
fixtures, CI, or logs.

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
- generating a header-only transcription template and converting only
  owner-reviewed rows;
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
or silent time inference is claimed.

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
on 737 principal episodes from the imported digital history (738 observations,
one classified nap):

| Candidate | Evaluations | Refusals | Median error | P90 error | Hit rate | Mean window |
|---|---:|---:|---:|---:|---:|---:|
| Baseline (1.00) | 636 | 94 | 1.77 h | 5.65 h | 0.78 | 14.48 h |
| Tighten to 0.75 | 636 | 94 | 1.77 h | 5.65 h | 0.72 | 13.06 h |
| Tighten to 0.50 | 636 | 94 | 1.77 h | 5.65 h | 0.66 | 11.64 h |

All 94 refusals were `ambiguous_cycle_index`; every eligible holdout was either
evaluated or refused.

**Estimator decision:** reject blanket forecast-window tightening. A 0.75 scale
saved 1.42 hours of mean width but lost 6 percentage points of hit rate; a 0.50
scale saved 2.84 hours but lost 12 points. Neither candidate changed point error.
The production default remains 1.00.

The baseline confidence buckets also justify a measured follow-up, not an
immediate model change: low-confidence median error was 2.31 hours versus 1.37
hours at medium confidence, while the small high-confidence bucket had only a
0.44 hit rate (16 evaluations). An explicit calibration/misfit candidate may be
developed next, but it must beat this committed baseline before shipping.
Phase-dependent duration remains deferred until the harness scores duration.

## Consequences

- Local imports are append-only and idempotent without weakening hard erasure.
- The real digital history imports cleanly and produces a reproducible aggregate
  benchmark without committing private observations.
- The available digital history begins in late October 2021. Earlier
  handwritten-only charts remain owner-reviewed transcription work; this ADR
  does not claim complete coverage of those charts.
- The measured run used one owner-selected zone. Any unrecorded travel-zone
  changes remain a source-data limitation, not something the converter guesses.
- Broad window tightening is closed as a candidate for now. Calibration/misfit
  work remains open and quantitatively gated.
- Importing an untrusted file can disclose its contents to the local process,
  but not to a network service. Size, shape, row, identifier, transaction, and
  logging boundaries are documented in the threat model.
