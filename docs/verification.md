# Verification record

Verified on Windows 11 on 2026-06-15.

## Passing checks

- `scripts/setup.ps1`: pinned local Node.js and Wails setup, Go modules,
  Java/Gradle detection, npm install, and deterministic fixture check.
- `scripts/dev.ps1 -Action check -Component all`: contract validation, Go
  formatting/tests/vet, both web production builds, and Android `check`.
- Frontend quality checks: Prettier, ESLint, TypeScript, 5 desktop tests, and 6
  trusted-view tests.
- Native Wails production build:
  `apps/desktop/build/bin/Non24Planner.exe`.
- Windows launch health check: the native executable remained running with a
  nonzero main-window handle for six seconds before test cleanup.
- Android debug build:
  `apps/android/app/build/outputs/apk/debug/app-debug.apk`.
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
