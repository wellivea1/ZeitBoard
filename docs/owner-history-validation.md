# Owner history import and validation

This workflow converts owner-controlled sleep files locally, previews every
row, imports only an entirely valid batch, and runs aggregate estimator
validation. It does not upload files or recognize handwriting.

Run commands from `apps/desktop`. Keep every input, converted observation set,
SQLite validation database, and detailed output under the ignored repository
`.cache/` directory or outside the repository. Never add those files to source
control, fixtures, issues, or CI.

## Desktop import

The Data Sources screen accepts:

- a v1 JSON `observation-set` with `schema_version`, `generated_at`, and
  `observations`; or
- canonical CSV with this exact header:

```text
observation_id,kind,start_at,end_at,zone_id,sleep_classification,acquisition_method,evidence_status,recorded_at,source_record_id
```

Every imported row must be a `sleep_episode`, use `file_import`, have a globally
stable `source_record_id`, and be classified as `principal` or `nap`. The screen
always previews first. Invalid rows block the full file; exact duplicates are
listed and do not create a second observation. Import is a separate explicit
action.

Imported entries appear in the same local sleep log as manual entries. Edit and
suppress still append corrections. Permanent deletion remains the distinct
ADR-0014/0017 erasure path and removes imported data as well.

## Fitbit conversion

The converter recursively reads finalized files named `Fitbit Sleep Data*.csv`.
By default it excludes files under directories named `Old`, `Incomplete`, or
`weekly` and reports that count. Use `--include-superseded` only for an explicit
comparison, not to silently merge stale exports.

```powershell
go run ./cmd/zeitboard-history fitbit `
  --in <private-fitbit-directory> `
  --out <private-output.json> `
  --format json `
  --zone America/New_York `
  --from 2021-01-01 `
  --through 2023-12-31
```

The summary accounts for every row as included, exact duplicate, or outside the
requested dates. Intervals under three hours are classified as naps and counted
for owner review. Civil timestamps in a DST ambiguity/nonexistence window are
rejected rather than guessed. One `--zone` applies to one conversion run; if a
history spans travel, split date ranges by the owner-confirmed zone and import
the resulting files separately. The converter never infers travel zones.

## Handwritten chart transcription

Generate a header-only owner-review template, or prepopulate one pending row per
chart date. This example covers the handwritten-only span before Fitbit begins:

```powershell
go run ./cmd/zeitboard-history template `
  --out <private-transcription.csv> `
  --from 2021-03-11 `
  --through 2021-10-27 `
  --zone America/New_York `
  --source-prefix chart `
  --force
```

The exact review header is:

```text
source_record_id,start_local,end_local,zone_id,classification,review_status
```

Every generated row starts as `needs_review`, which blocks conversion. After
checking the source chart, change it to exactly one of:

- `confirmed_sleep`: fill `start_local`, `end_local`, and `classification`
  (`principal` or `nap`);
- `confirmed_no_observation`: leave both times and classification empty. This
  records that the chart row was checked without claiming the owner did not
  sleep.

Use one stable source ID per chart entry and an IANA zone. If one chart date has
multiple sleep episodes, add rows with unique suffixes rather than overwriting
one interval. Local civil times may use `YYYY-MM-DD HH:MM`. At a DST ambiguity
use RFC 3339 with the correct explicit offset, such as
`2021-11-07T01:30:00-04:00`.

The converter rejects pending rows, repeated IDs, impossible ranges, invalid
zones, inconsistent no-observation rows, and offsets that disagree with the
named zone. It emits no partial file and reports all pending rows; nothing is
silently skipped.

```powershell
go run ./cmd/zeitboard-history transcription `
  --in <private-transcription.csv> `
  --out <private-transcription-v1.json> `
  --format json
```

The owner must review handwritten values and every dated row status. Do not use
an OCR result as an observed timestamp without that review.

## CLI preview and backtest

Use a disposable validation database first. Preview is the default; `--commit`
is required to append rows:

```powershell
go run ./cmd/zeitboard-history import `
  --in <private-observation-set.json> `
  --database <private-validation.db>

go run ./cmd/zeitboard-history import `
  --in <private-observation-set.json> `
  --database <private-validation.db> `
  --commit

go run ./cmd/zeitboard-history backtest `
  --database <private-validation.db> `
  --out <private-aggregate-report.md>
```

Only aggregate counts and error/window metrics belong in
`docs/verification.md`. Detailed refusal points contain timestamps and stay
private.
