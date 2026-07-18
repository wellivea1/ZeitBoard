# Verification record

Most recent local verification: Windows 11 on 2026-07-18.

## Passing checks

- Frontend formatting, ESLint, and repository UI standards. The UI guard passed
  with 12 screen modules and 3 route stylesheets.
- Frontend TypeScript and production builds for the desktop and trusted-view
  workspaces.
- Frontend tests: 13 desktop test files with 150 tests, plus 2 trusted-view test
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
