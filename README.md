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

Every planned function is delivered; [docs/completion-plan.md](docs/completion-plan.md) records
each one with its tests, and what remains needs the owner's machine, an independent reviewer or a
release ceremony rather than code.

- **Sleep and rhythm.** Manual nights and file imports keep raw records with append-only
  corrections, and a night's last edit is undone in one click. The Go estimator forecasts
  uncertain sleep and waking windows and refuses plainly when the evidence is thin. The clinical
  chart prints, saves as PDF, or saves as PNG.
- **Planning.** Real tasks get suggested times that stay clear of busy events. Suggestions are
  reviewed and decided together, and each decision can be undone. Reaching hours can be set for
  several people, each on their own days.
- **Calendars.** ZeitBoard reads ICS files, CalDAV, a calendar's private iCal address (read again
  every hour), and Google or Microsoft accounts signed in through an app the owner registers
  (ADR-0064). Accepted times can be written to one calendar, by CalDAV or in such an account. The
  writes use preconditions that never change the owner's own events, nor ZeitBoard's once the
  owner has changed them (ADR-0053, ADR-0065).
- **Medications.** Schedules, dose logging, opt-in reminders and a clinician context report are
  available. Missed-dose sharing is off by default per link.
- **Activity, in the shadow.** Opt-in desktop activity, Windows' power log and a Google Takeout
  export suggest nights beside the recorded ones. They do not change forecasts until a candidate
  passes the backtest gate (ADR-0031).
- **Your own server.** The self-hosted backend provides encrypted sync, server-side projections,
  passcode-protected availability links, time requests and notifications. The Android companion
  syncs Health Connect sleep, shows forecasts and tasks, and records doses as taken or skipped.
- **Assistants.** The assistant and agents propose and never apply. The assistant uses the
  server's own provider key, or the owner's ChatGPT plan through a sign-in the server holds
  (ADR-0066). ZeitBoard also offers a local MCP connector, a Claude Desktop extension and a skill.
- **This computer.** Local files are restricted to the owner's account. Windows can encrypt them
  by choice (ADR-0063). Everything can be exported or erased.
- Browser-only desktop preview data is synthetic and visibly labeled `Sample data`; it is not used
  by the running Wails desktop service.

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
