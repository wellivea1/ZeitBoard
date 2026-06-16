# Verification record

Most recent local verification: Windows 11 on 2026-06-16.

## Passing checks

- `scripts/dev.ps1 -Action check -Component all`: contract validation, Go
  formatting/tests/vet, both web production builds, and Android `check`.
- Frontend quality checks: Prettier check, ESLint, TypeScript, 26 desktop tests,
  and 6 trusted-view tests.
- Contract tooling: deterministic fixture drift check, schema validation, tools
  module tests, and tools module vet.
- Android Gradle check: debug unit tests, lint, and Gradle `check`.
- Native Wails production build:
  `apps/desktop/build/bin/ZeitBoard.exe`.
- Hidden Windows launch health check: `ZeitBoard.exe` remained running for six
  seconds before test cleanup. Hidden startup does not expose a main-window
  handle.
- Android debug build:
  `apps/android/app/build/outputs/apk/debug/app-debug.apk`.
- Desktop theming/reduced-stimulation: unit tests for theme and reduced-
  stimulation persistence, `useAppearance` integration tests, and manual
  verification that the Settings controls apply `data-theme` and `data-reduced`
  attributes and persist across reload.

## Previously verified environment setup and artifacts

Verified on Windows 11 on 2026-06-15:

- `scripts/setup.ps1`: pinned local Node.js and Wails setup, Go modules,
  Java/Gradle detection, npm install, and deterministic fixture check.
- In-app browser verification: desktop overview and trusted view rendered with
  no console warnings/errors; screenshots are in `docs/screenshots/`.

## Environment limitations

- No Android device was connected and `emulator -list-avds` returned no AVDs.
  The APK, unit tests, lint, and Gradle checks pass, but this environment could
  not launch the Android UI or capture an Android screenshot.
- The installed Go toolchain reports CGO disabled, so `go test -race` is not
  available. Normal Go tests and vet pass.
- The added DOCX specifications were read structurally. Visual DOCX rendering
  was unavailable because LibreOffice is not installed; the renderer also
  reported missing explicit page-size properties in the source documents.
