# ZeitBoard

ZeitBoard is an early local-first scheduling prototype for people whose sleep-wake timing is free-running or highly irregular. It estimates an **observed sleep-wake rhythm** from user-controlled data, forecasts uncertain sleep and waking windows, and uses those forecasts to propose useful times for flexible tasks.

This repository is a phase-one scaffold. It is not a medical device, does not estimate DLMO or exact circadian phase, and does not recommend medication, light, melatonin, stimulant, hypnotic, meal, or exercise timing.

## Repository layout

```text
apps/desktop                Wails v2 desktop application and React UI
apps/android                Native Android Compose companion
apps/trusted-web-prototype  Static synthetic trusted-view prototype
core                        Go domain, ingest, storage, estimation, and scheduling
contracts                   Versioned JSON Schemas
docs                        Product specifications, architecture, privacy, roadmap, and ADRs
scripts                     Reproducible setup, development, and fixture generation
testdata                    Synthetic observations and projected sharing fixtures
```

The application is deliberately isolated from the neighboring Zeitlog and Zeitdex workspaces. No code, branding, or repository history is shared.

## Phase-one status

The phase-one scaffold is implemented with synthetic fixture flows:

- Go core with provenance-preserving corrections, robust sleep-start drift estimation, uncertainty forecasts, scheduling proposals, medication-relative timing, SQLite storage, and default-deny sharing.
- Wails v2 Windows application with eight accessible React screens, an in-process collector boundary, and a Windows tray reopen/quit adapter.
- Native Android Compose application with visible fixture mode and Health Connect `READ_SLEEP` availability/permission flow.
- Static trusted-view prototype containing only pre-projected synthetic fields and no network requests.

## Build and test

From the repository root on Windows:

```powershell
.\scripts\setup.ps1
.\scripts\dev.ps1 -Action check -Component all
```

Direct component commands:

```powershell
go test ./core/... ./apps/desktop/...
npm ci
npm run lint
npm run typecheck
npm test
npm run build
.\scripts\dev.ps1 -Action build -Component desktop
.\scripts\dev.ps1 -Action build -Component android
```

See [docs/development.md](docs/development.md) for Linux commands,
[docs/verification.md](docs/verification.md) for the verified environment, and
[docs/roadmap.md](docs/roadmap.md) for real-source integration work. Android
emulator launch requires an installed AVD; the debug APK can be built without
one.
