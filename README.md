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

- The Wails desktop runs on persisted manual sleep observations, immutable raw records, and
  append-only corrections. Overview, Rhythm, and proposals use the real Go estimator and refuse
  honestly when evidence is insufficient.
- Desktop activity collection is explicitly opt-in, persists minimized transitions locally,
  and offers export/erasure in Settings. Enrolled sleep/task sync runs automatically while
  ZeitBoard is running, including when its window is hidden. Activity does not change forecasts.
- Contract-shaped JSON export, permanent local erasure, opt-in self-hosted sync, cross-device
  tombstones, user-owned tasks, unified approvals, and the propose-only assistant surface are
  implemented.
- The self-hosted backend provides encrypted sync, server-side projections, BYOK assistant
  providers, and a local MCP connector with read and propose-only tools.
- The Android companion has opt-in automatic Health Connect sync, durable pull/erasure, cached Go
  forecasts and read-only task downloads. It exposes uncertainty, cache age and time-zone holds.
  Phone-authored sleep corrections now sync, including pre-enrollment edits.
  Medication sync and desktop lifecycle implementation/qualification remain
  unfinished; real-device background reliability and passive coverage need pilot measurement
  (ADR-0037/0038).
- The desktop Sharing screen manages real passcode-protected availability links on the owner's
  server. The separate trusted-web prototype remains static and synthetic.
- Task creation, editing, completion and reviewed deletion are available in Plan. Desktop service
  failures show retryable unavailable states; sample forecasts are confined to browser preview.
- Browser-only desktop preview data is synthetic and visibly labeled `Sample data`; it is not used
  by the running Wails desktop service.

See the [September product quality review](docs/product-quality-review-2026-09-08.md) for the
current usability assessment, implemented repairs and remaining work.

## Install (end users)

To build and install the desktop app in one step — dependencies, build, a
behavior decision tree (shortcuts, launch-at-startup), and optional server /
MCP / Android extras:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\installer\install.ps1
```

Add `-DryRun` to preview every step without changes, or `-NonInteractive`
with flags (`-Startup`, `-DesktopShortcut`, `-WithServer`, ...) for an
unattended install. Update in place with `scripts\installer\update.ps1`
(auto-backs-up data, `-Rollback` to revert). Full scheme:
[`docs/install-update-design.md`](docs/install-update-design.md).

## Build and test (contributors)

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
