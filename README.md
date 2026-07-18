# ZeitBoard

ZeitBoard is a local-first planner for people whose sleep-wake timing is free-running or highly irregular. It estimates an **observed sleep-wake rhythm** from user-controlled data, forecasts uncertain sleep and waking windows, and uses those forecasts to propose useful times for flexible tasks.

It is not a medical device, does not estimate DLMO or exact circadian phase, and does not recommend medication, light, melatonin, stimulant, hypnotic, meal, or exercise timing.

ZeitBoard is a separate project from the neighboring Zeitlog and Zeitdex workspaces. The shared `zeit` prefix is intentional branding, not shared code, history, or product scope.

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


## Current status

- The Wails desktop runs on persisted manual sleep observations, immutable raw records, and append-only corrections. Overview, Rhythm, and proposals use the real Go estimator and refuse honestly when evidence is insufficient.
- Contract-shaped JSON export, permanent local erasure, opt-in self-hosted sync, cross-device tombstones, user-owned tasks, unified approvals, and the propose-only assistant surface are implemented.
- The self-hosted backend provides encrypted sync, server-side projections, BYOK assistant providers, and a local MCP connector with read and propose-only tools.
- The Android companion remains a build-ready Health Connect skeleton and does not yet sync. The trusted-view prototype remains static and synthetic.
- Browser-only desktop preview data is synthetic and visibly labeled `Sample data`; it is not used by the running Wails desktop service.

## Build and test

From the repository root on Windows:

```powershell
.\scripts\setup.ps1
.\scripts\dev.ps1 -Action check -Component all
```

Direct component commands:

```powershell
go test ./core/... ./apps/desktop/...
.\scripts\dev.ps1 -Action build -Component desktop
.\scripts\dev.ps1 -Action build -Component android
```

The setup and development scripts prefer the pinned local Node.js runtime under
`.tools`. If running npm directly in a fresh shell, first put
`.tools\node-v24.16.0-win-x64` on `PATH`, or use the scripts above.

See [docs/development.md](docs/development.md) for Linux commands,
[docs/verification.md](docs/verification.md) for the verified environment,
[docs/frontend-architecture.md](docs/frontend-architecture.md) for desktop UI
boundaries, and [docs/roadmap.md](docs/roadmap.md) for remaining work. Android
emulator launch requires an installed AVD; the debug APK can be built without
one.
