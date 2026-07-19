# Verification record

Most recent local verification: Windows 11 on 2026-07-19.

## Passing checks

- Frontend formatting, ESLint, and repository UI standards. The UI guard passed
  with 12 screen modules and 4 component stylesheets.
- Frontend TypeScript and production builds for the desktop and trusted-view
  workspaces.
- Frontend tests: 16 desktop test files with 161 tests, plus 2 trusted-view test
  files with 6 tests.
- `scripts/dev.ps1 -Action check -Component desktop`: desktop production build.
- `scripts/dev.ps1 -Action check -Component core`: Go formatting, tests, and vet
  for `core` and `apps/desktop`, including the Windows tray package.
- `scripts/dev.ps1 -Action build -Component core`: Go builds for `core` and
  `apps/desktop`.
- `scripts/dev.ps1 -Action check -Component server`: server formatting, tests,
  and vet.
- `scripts/dev.ps1 -Action check -Component contracts`: deterministic drift
  check for 20 v1 fixture files, tools tests/vet, and schema validation.
- `scripts/dev.ps1 -Action check -Component android`: Gradle `check`.
- Sleep erasure regression: suppression remains exportable and excluded from
  effective reads; hard deletion removes observation/correction rows and the
  unique payload marker from the compacted SQLite database and WAL.

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
reported 0 ready, 738 exact duplicates, and 0 invalid rows. The available
digital history covers late October 2021 through December 2023. Earlier
handwritten-only charts were not machine-read; the owner-review template and
strict converter are implemented, but those rows remain a stated coverage gap.
The measured conversion used one owner-selected zone; no travel-zone inference
was applied.

The backtest used 737 principal episodes. With the seven-episode minimum, all
730 eligible holdouts were accounted for as 636 evaluations plus 94 typed
refusals (coverage 0.871). Every refusal was `ambiguous_cycle_index`.

| Candidate | Scale | Evaluations | Refusals | Median error | Mean error | P90 error | Hit rate | Mean window |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| Baseline | 1.00 | 636 | 94 | 1.77 h | 2.50 h | 5.65 h | 0.78 | 14.48 h |
| Tighten-75 | 0.75 | 636 | 94 | 1.77 h | 2.50 h | 5.65 h | 0.72 | 13.06 h |
| Tighten-50 | 0.50 | 636 | 94 | 1.77 h | 2.50 h | 5.65 h | 0.66 | 11.64 h |

Baseline confidence calibration:

| Confidence | Evaluations | Hit rate | Median error |
|---|---:|---:|---:|
| High | 16 | 0.44 | 1.64 h |
| Medium | 302 | 0.82 | 1.37 h |
| Low | 318 | 0.76 | 2.31 h |

Measured decision: keep the production uncertainty scale at 1.00. Tighten-75
reduced mean width by 1.42h but lost 6 percentage points of hit rate;
Tighten-50 reduced width by 2.84h but lost 12 points. Point error did not
improve. The low-versus-medium error delta and the small, poorly calibrated
high bucket justify testing an explicit calibration/misfit candidate next, but
no such signal ships without a positive delta against this baseline.

## Visual verification

- Manually reviewed Overview and Rhythm at 1440x900 in Paper, Dark, Pitch black,
  Amber, and High contrast, with reduced stimulation both off and on. Appearance
  was restored to Auto with reduced stimulation off after the matrix.
- Overview measured approximately 693px high (77% of the viewport) with one
  primary surface and no nested generic panels or metric cards.
- Rhythm's actogram measured approximately 568px high and stayed within the
  desktop viewport.
- Overview had no page-level horizontal overflow at 900x900 or 390x844. The
  navigation rail scrolls internally at narrow widths as designed.
- The 390x844 Rhythm pass exposed chart overflow propagating to the page. The
  final CSS contains that overflow at `.actogram-panel` while preserving
  horizontal scrolling in `.actogram-chart`; the UI standards check now guards
  both rules and the readable 760px double-plot width.

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

- The in-app browser refused the final localhost reload after the temporary
  Vite server stopped, so the post-fix 390x844 Rhythm containment was verified
  by source guard, lint, and production build rather than a second screenshot.
- No Android device or emulator was available for an Android UI launch. The APK,
  unit tests, lint, and Gradle checks pass.
- The installed Go toolchain has CGO disabled, so `go test -race` is unavailable.
