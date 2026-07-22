# Verification record

Most recent local verification: Windows 11 on 2026-07-22.

## Passing checks

- Frontend formatting, ESLint, and repository UI standards. The UI guard passed
  with 14 screen modules and 6 component stylesheets.
- Frontend TypeScript and production builds for the desktop and trusted-view
  workspaces.
- Frontend tests: 24 desktop test files with 187 tests, plus 2 trusted-view test
  files with 6 tests.
- `scripts/dev.ps1 -Action check -Component desktop`: desktop production build.
- `scripts/dev.ps1 -Action check -Component core`: Go formatting, tests, and vet
  for `core` and `apps/desktop`, including the Windows tray package.
- `scripts/dev.ps1 -Action build -Component core`: Go builds for `core` and
  `apps/desktop`.
- `scripts/dev.ps1 -Action check -Component server`: server formatting, tests,
  and vet, including the server/MCP projection privacy allowlist.
- `scripts/dev.ps1 -Action check -Component contracts`: deterministic drift
  check for 24 v1 fixture files, tools tests/vet, and schema validation.
- Native Wails production build at `apps/desktop/build/bin/ZeitBoard.exe`.
- Android `testDebugUnitTest`, `lintDebug`, and `assembleDebug`; this medication
  slice changes no Android source.
- Sleep erasure regression: suppression remains exportable and excluded from
  effective reads; hard deletion removes observation/correction rows and the
  unique payload marker from the compacted SQLite database and WAL.
- Medication evidence regression: exclusion remains an append-only correction
  and stored evidence remains countable/exportable; event erasure removes the
  selected event and correction chain without deleting its definition, while
  medication erasure removes the definition and all dependent evidence. Both
  paths remove unique payload markers from the compacted SQLite database and
  WAL.
- Calendar ownership regression: bounded ICS/CalDAV preview and import,
  recurrence/DST parsing, immutable imported rows, and source erasure all pass.
  Erasure removes private labels, titles, locations, notes, and saved endpoints
  from both the compacted SQLite database and WAL. Text-free scheduler
  projection, task/sleep/event stale-decision refusal, app-owned approval
  materialization, rejection, undo, and import-free ICS export also pass.

The check wrapper itself was also corrected: native Go, npm, Wails, contract,
and Gradle failures now propagate as a non-zero script result.

## Real-history import and estimator gate

Only aggregate results are recorded here. Raw exports, converted observations,
the validation SQLite databases, rendered charts, and detailed refusal points
remain private and ignored.

The finalized digital source conversion accounted for every source row:

| Stage | Count |
|---|---:|
| Finalized Fitbit files read | 35 |
| Source rows read | 923 |
| Exact overlapping rows | 105 |
| Rows outside 2021-2023 | 80 |
| Included observations | 738 |
| Included observations under 3h, classified as nap | 1 |
| Matching superseded files excluded (`Old` / `Incomplete` / `weekly`) | 26 |

Against a fresh ignored database, preview reported 738 ready, 0 duplicate,
and 0 invalid rows. Commit imported all 738 atomically. A second preview
reported 0 ready, 738 exact duplicates, and 0 invalid rows. The digital
history covers late October 2021 through December 2023.

The five original handwritten-chart pages were source-reviewed without OCR.
Their 241 labeled day rows use the Sleep Diary report's 18:00-to-18:00 layout.
Grid-aligned five-minute boundary estimates were visually checked on full-page
overlays. The date-complete review ledger contains 243 explicit statuses: 223
`confirmed_sleep` entries and 20 `confirmed_no_observation` entries. There are
two more statuses than chart rows because two rows contain two sleep episodes;
`confirmed_no_observation` means that a row was checked and contains no new
episode start, not that the owner did not sleep.

Eight chart episodes overlap finalized Fitbit records and were reserved for a
source-accuracy check. Across their 16 start/end boundaries, absolute chart
error versus Fitbit was 23.2 minutes mean, 10.0 minutes median, 55.5 minutes
P90, and 127 minutes maximum. The primary benchmark therefore uses 215 chart
episodes from before Fitbit begins and retains the eight overlap episodes only
for calibration. Its 232 chart rows produce a 234-status ledger: 215 confirmed
sleep entries and 19 explicitly checked rows without a new episode. Conversion
reported all 234 rows, wrote 215 observations, and silently skipped none.

The combined import used 738 Fitbit observations plus those 215 non-overlapping
chart observations. On a fresh ignored database, the source previews reported
738 and 215 ready with zero invalid rows. Their commits appended all 953
observations atomically; repeat previews reported 738 and 215 exact duplicates
with zero ready or invalid rows. The measured conversion used one owner-selected
zone; no travel-zone inference was applied.

The combined backtest used 952 principal episodes; the remaining observation
was the reported Fitbit nap. With the seven-episode minimum, all 945 eligible
holdouts were accounted for as 809 evaluations plus 136 typed refusals
(coverage 0.856). Every refusal was `ambiguous_cycle_index`.

| Candidate | Scale | Evaluations | Refusals | Coverage | Median error | Mean error | P90 error | Hit rate | Mean window |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| Baseline | 1.00 | 809 | 136 | 0.856 | 1.71 h | 2.40 h | 5.41 h | 0.78 | 14.71 h |
| Tighten-75 | 0.75 | 809 | 136 | 0.856 | 1.71 h | 2.40 h | 5.41 h | 0.72 | 13.31 h |
| Tighten-50 | 0.50 | 809 | 136 | 0.856 | 1.71 h | 2.40 h | 5.41 h | 0.66 | 11.91 h |

Baseline confidence calibration:

| Confidence | Evaluations | Hit rate | Median error |
|---|---:|---:|---:|
| High | 28 | 0.61 | 1.23 h |
| Medium | 386 | 0.81 | 1.42 h |
| Low | 395 | 0.77 | 2.19 h |

Measured decision: keep the production uncertainty scale at 1.00. Tighten-75
reduced mean width by 1.40h but lost 6 percentage points of hit rate;
Tighten-50 reduced width by 2.80h but lost 12 points. Point error did not
improve. The chart-inclusive baseline also improves on the digital-only run's
1.77h median, 2.50h mean, and 5.65h P90 errors while preserving its 0.78 hit
rate. A sensitivity run that allowed the eight lower-precision chart overlaps
to merge into Fitbit changed only median error, from 1.71h to 1.70h at displayed
precision; the decision is unchanged. The low-versus-medium error delta and
the high bucket's lower hit rate justify testing an explicit confidence-window
calibration or misfit candidate next, but no such signal ships without a
positive delta against this combined baseline.

## Visual verification

- Reviewed the real Medications workspace with unavailable-service and populated
  bridge states at 1440x900 and 390x844. Definition setup, compact quick log,
  observed/predicted/unavailable timing context, correction, exclusion, and
  typed hard-erasure controls remained legible and contained without document
  overflow. Runtime console review reported no warnings or errors.
- Reviewed the real-calendar replacement at 1440x900 in the browser fixture:
  source administration remains a compact rail, forecast ranges remain
  background bands, fixed events use rectangular overlap lanes, exact event
  details open in the inspector, and the document has no horizontal overflow.
- Repeated Calendar at 390x844. The primary board precedes source
  administration, date controls collapse to two compact rows, the 620px civil
  board scrolls inside its column, and the document remains within the 375px
  layout viewport. Runtime console review reported no warnings or errors.

- Manually reviewed Overview and Rhythm at 1440x900 in Paper, Dark, Pitch black,
  Amber, and High contrast, with reduced stimulation both off and on. Appearance
  was restored to Auto with reduced stimulation off after the matrix.
- Reviewed the U-E time probes on Overview, actogram, drift, and Calendar at
  1440x900. The actogram second plot resolved to the following civil day,
  forecast positions stayed explicitly predicted, drift reported observed and
  fitted onsets, and edge chips remained inside their tracks.
- Repeated probe and containment checks with a 390x844 viewport override in
  High contrast + reduced stimulation. Overview and Calendar did not widen the
  page; the actogram retained internal horizontal scrolling while the document
  stayed at viewport width; the drift screen-reader table remained semantic
  without contributing its intrinsic column width to page overflow.
- Pointer movement, chart scrolling, and route changes left at most one probe
  visible. Runtime console review reported no warnings or errors. Paper, Amber,
  and High contrast probe chips were visually reviewed; theme contrast tests
  cover all five presets. Appearance and viewport overrides were reset.
- Overview measured approximately 693px high (77% of the viewport) with one
  primary surface and no nested generic panels or metric cards.
- Rhythm's actogram measured approximately 568px high and stayed within the
  desktop viewport.
- Overview had no page-level horizontal overflow at 900x900 or 390x844. The
  navigation rail scrolls internally at narrow widths as designed.
- The 390x844 Rhythm pass exposed chart overflow propagating to the page. The
  final CSS paint-contains that overflow at `.actogram-panel` while preserving
  horizontal scrolling in `.actogram-chart`; hidden screen-reader tables use
  fixed layout so added columns cannot widen the page. The UI standards check
  guards these rules and the readable 760px double-plot width.

## Previously verified artifacts

Verified on Windows 11 on 2026-06-15 and 2026-06-16:

- Pinned local Node.js and Wails setup, Go modules, Java/Gradle detection, npm
  installation, and deterministic fixture generation.
- Native Wails production build at `apps/desktop/build/bin/ZeitBoard.exe` and a
  hidden Windows launch health check.
- Android debug APK at
  `apps/android/app/build/outputs/apk/debug/app-debug.apk`.
- Desktop overview and trusted-view browser screenshots in `docs/screenshots/`.

## Environment limitations

- Android UI was previously reviewed on the Pixel 10 Pro XL API 36 emulator;
  this desktop-only slice changed no Android source. Gradle verification was
  rerun, but the emulator visual matrix was not repeated.
- The installed Go toolchain has CGO disabled, so `go test -race` is unavailable.
