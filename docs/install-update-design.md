# Installation & update scheme design

> Design for the ZeitBoard install/update tooling: a shared base support
> library with thin scripts on top — all-in-one Windows installer with a
> behavior decision tree, updater with rollback, Android builder, server
> installer, uninstaller. Windows PowerShell 5.1 is the floor (no pwsh
> assumption); Linux server hosts keep the `self-hosting.md` path with a
> bash twin of the base library later.

## 1. Principles

1. **Pinned and vendored, never "latest".** Every toolchain lands in the
   repo-local `.tools\` directory (the pattern node and wails.exe already
   use), pinned by version and SHA-256 in one manifest. The system is
   touched only when a component genuinely requires it (WebView2, a
   Windows service). No `curl | iex`, ever.
2. **Idempotent and resumable.** Re-running any script is always safe;
   completed steps are detected and skipped; a failed run prints the exact
   command to resume.
3. **Consent-shaped.** Anything that persists outside the repo/app dirs —
   startup entries, services, firewall rules, SDK license acceptance — is
   an explicit decision-tree question (or flag), never a side effect.
   Elevation is requested per-step only when the chosen component needs
   it, and the script says why.
4. **Honest failure.** No art on failure. The failing step, its last log
   lines, the full log path, and the resume command — nothing else.
5. **Data outlives binaries.** `%APPDATA%\ZeitBoard` (store, tokens,
   config) is never written by the installer and never removed by the
   uninstaller without the separate `-PurgeData` + typed `DELETE`
   confirmation (mirroring ADR-0014 in-app semantics).

## 2. Evaluation: base support file vs. alternatives

| Option | Verdict |
|---|---|
| **Shared dot-sourced library + thin entry scripts** | **Chosen.** One tested implementation of logging, probing, downloading, verifying, extracting, shortcuts, and art; entry scripts read as plain phase lists; PS 5.1-native; unit-testable with Pester. |
| One monolithic `install.ps1` | Rejected: updater/Android/uninstall would copy-paste the same 400 lines; drift guaranteed. |
| Makefile / Taskfile | Rejected as the *user-facing* layer: adds a dependency before dependencies are installed, and Windows-hostile. Fine internally for CI, which already calls the npm/go commands directly. |
| MSI / MSIX / Inno Setup now | Deferred: packaging binaries is a distribution problem; today's audience builds from source. The library is designed so a packaging script later becomes just another consumer (`package.ps1`). |

Layering:

```
scripts/installer/
  _zb.common.ps1      <- the base support file (library, no side effects on load)
  pins.psd1           <- single version/URL/SHA-256 manifest
  install.ps1         <- all-in-one Windows app installer
  update.ps1          <- updater with backup + rollback
  build-android.ps1   <- Android APK builder
  install-server.ps1  <- zeitboardd as a Windows service (optional host role)
  uninstall.ps1       <- reverse of install; data purge is separate + typed
```

## 3. Base support file contract (`_zb.common.ps1`)

Dot-sourced first by every entry script. Provides, and nothing else runs on
load:

- `Get-ZbPaths` — the single source of truth: repo root, `.tools\`,
  install dir (`%LOCALAPPDATA%\Programs\ZeitBoard`), data dir
  (`%APPDATA%\ZeitBoard`), log dir (`%TEMP%\zeitboard-install\`).
- `Write-ZbLog -Level info|warn|fail -Message` — console + transcript file.
- `Invoke-ZbStep -Name -Action { }` — step runner: timing, skip-detection
  hook, failure capture (last 20 log lines + resume hint), `-DryRun`
  support (prints the plan instead of acting).
- `Test-ZbDependency` / `Install-ZbDependency -Name` — probe order:
  `.tools\` pin → acceptable system version → download from `pins.psd1`
  URL, **verify SHA-256**, extract into `.tools\<name>-<version>`.
  Never PATH-pollutes the machine; each script prepends `.tools` paths to
  its own process `PATH` only (exactly like `scripts/dev.ps1` today).
- `Assert-ZbRepoClean`, `Get-ZbVersionStamp` (commit hash + date),
  `Backup-ZbData` (db + `-wal`/`-shm` + config, timestamped zip),
  `New-ZbShortcut`, `Set-ZbStartupEntry` (HKCU Run key add/remove),
  `Show-ZbBanner` / `Show-ZbFinale -Kind install|update|android`.
- All prompts route through `Read-ZbChoice`, which honors
  `-NonInteractive` + per-question flags so CI and unattended installs
  answer everything from the command line.

## 4. Dependency matrix (what "all needed dependencies" means)

| Dependency | Pin | Detect | Install action | Elevation |
|---|---|---|---|---|
| Git | any ≥ 2.40 | `git --version` | instruct only (installer refuses to fetch git itself — it is how you got the repo) | – |
| Go | **1.26.0** | `.tools\go\` → `go version` | portable zip → `.tools\go` | no |
| Node | **v24.16.0** | `.tools\node-v24.16.0-win-x64` (already vendored) | zip → `.tools` if missing | no |
| Wails CLI | **v2.12.0** | `.tools\bin\wails.exe` | `go install` into `.tools\bin` (GOBIN) | no |
| WebView2 Runtime | Evergreen | registry key | winget per-user, else Evergreen bootstrapper; Win11 ships it — usually a no-op | only if machine-wide chosen |
| C compiler | **none** | – | not needed: `modernc.org/sqlite` is pure Go — the design explicitly forbids adding cgo deps without updating this doc | – |
| npm packages | lockfile | `node_modules` freshness | `npm ci` (workspace root) | no |
| JDK (Android only) | Temurin **17** | `.tools\jdk-17` | portable zip → `.tools` | no |
| Android SDK (Android only) | cmdline-tools pinned | `.tools\android-sdk` | sdkmanager fetch; **license acceptance is an explicit prompt**, never `--licenses` piped `y` silently | no |

## 5. `install.ps1` — the all-in-one installer

Phases (each an `Invoke-ZbStep`): banner → preflight (OS ≥ Win10 1809 for
WebView2, disk space, repo clean-or-warn, execution-policy *note* — the doc
tells users to run `powershell -ExecutionPolicy Bypass -File install.ps1`;
the script never rewrites policy) → dependencies (§4) → build (npm ci →
frontend build → `wails build` → server + MCP binaries via `go build`) →
install (copy `ZeitBoard.exe` + version stamp to install dir; previous
build preserved in `previous\`) → **decision tree** → smoke test (binary
launches and writes its version stamp; data dir untouched check) → finale.

### Decision tree (interactive; every branch has a flag twin)

```
Install ZeitBoard how?
├─ Desktop app (default: yes)
│   ├─ Start Menu shortcut?          [Y/n]   -StartMenu:$false
│   ├─ Desktop shortcut?             [y/N]   -DesktopShortcut
│   ├─ Launch at Windows startup?    [y/N]   -Startup
│   │    └─ (HKCU Run key; the app starts to tray — matches the
│   │        existing tray Start/Quit behavior; removable by
│   │        uninstall.ps1 or the same flag with :$false)
│   └─ Launch now when finished?     [Y/n]   -NoLaunch
├─ Self-hosted server on THIS machine (default: no)  -WithServer
│   └─ delegates to install-server.ps1 (service, config, firewall Q)
├─ MCP connector on PATH? (default: no)              -WithMcp
│   └─ copies zeitboard-mcp.exe to install dir + prints the
│       Claude Desktop config snippet from self-hosting.md
└─ Android APK build? (default: no)                  -WithAndroid
    └─ delegates to build-android.ps1
```

Defaults are deliberately conservative: nothing auto-starts, nothing
listens on a port, unless chosen.

## 6. `update.ps1` — updater with rollback

banner → `Get-ZbVersionStamp` current vs `git fetch` target → show commit
summary + confirm → **`Backup-ZbData`** (always, before anything) → move
current install to `previous\` → rebuild exactly as install does →
optional `-SkipTests` (default runs the Go + frontend suites; an update
that fails tests aborts and auto-restores `previous\`) → swap in → smoke
test → finale.

- `update.ps1 -Rollback` restores `previous\` in one move. Data is never
  rolled back automatically — the backup zip path is printed instead,
  because the schema is additive-only (`CREATE TABLE IF NOT EXISTS`
  migrations; a destructive migration requires its own ADR and explicit
  updater support before it may ship).
- The updater refuses a dirty working tree (`Assert-ZbRepoClean`) unless
  `-AllowDirty`, and prints exactly what it would discard.

## 7. `build-android.ps1`

JDK + SDK bootstrap per §4 (license prompt is a hard consent gate) →
`gradlew assembleDebug` by default → `-Release` requires
`-Keystore <path>` (+ passwords via prompt or env, never argv) and refuses
to fabricate a keystore silently — first-time keystore creation is its own
guided, clearly-labeled branch that stores the file **outside** the repo →
prints APK path → optional `-AdbInstall` when exactly one device is
attached → finale (`APK READY` variant).

## 8. `install-server.ps1` and `uninstall.ps1`

- Server: builds `zeitboardd`, materializes a config from the
  `self-hosting.md` template (secrets generated per its Generate Secrets
  section, never defaults), registers a Windows service (`sc.exe create`,
  delayed-auto), asks separately about a firewall rule (default: LAN only;
  the portal remains `portal.enabled=off` regardless — exposure stays a
  deliberate config edit per `portal-design.md`). Linux hosts keep the
  existing runbook; the bash twin of `_zb.common` is a later, separate
  deliverable so parity is tested, not assumed.
- Uninstall: removes binaries, shortcuts, Run key, service (if present) —
  then stops. `-PurgeData` additionally requires typing `DELETE` and
  removes `%APPDATA%\ZeitBoard` after offering a final export, mirroring
  the in-app erasure ceremony.

## 9. Verification of the tooling itself

- Every script supports `-DryRun` (full plan, zero side effects); the CI
  `installer` job runs `install.ps1 -DryRun -NonInteractive` and
  `update.ps1 -DryRun` on windows-latest so the phase lists can't rot.
- `scripts\installer\test-installer.ps1` unit-tests `_zb.common.ps1`:
  path resolution, pin validation, `Read-ZbChoice` precedence, the
  placeholder-checksum refusal, `Invoke-ZbStep` skip/dry-run behavior,
  Run-key add/remove round-trip against a sandbox key, and ASCII-only
  banners. It uses plain assertions with an exit code rather than Pester —
  stock Windows ships Pester 3.4 while CI has 5.x and their assertion
  syntaxes are incompatible, so a dependency-free runner is the portable
  choice and runs identically in both places.
- Pin integrity is enforced by `Test-ZbPins` (called from the test runner):
  every `Url`/`Sha256Url` is https and every downloadable entry carries
  exactly one integrity source (literal `Sha256` or vendor `Sha256Url`).

## 10. Banners

ASCII-only (Windows PowerShell 5.1 consoles on legacy codepages garble
box-drawing and block glyphs). Start banner, all scripts:

```
=================================================================
  ZZZZZ EEEEE IIIII TTTTT BBBB   OOO   AAA  RRRR  DDDD
     Z  E       I     T   B   B O   O A   A R   R D   D
    Z   EEEE    I     T   BBBB  O   O AAAAA RRRR  D   D
   Z    E       I     T   B   B O   O A   A R  R  D   D
  ZZZZZ EEEEE IIIII   T   BBBB   OOO  A   A R   R DDDD
              a planner for free-running rhythms
=================================================================
```

Install finale (congratulatory — success only, per §1.4):

```
        .   *        .       *          .        *
   *        .   ________________   .          .
       .       /                \        *
  ------------|  Z E I T B O A R D  |------------------
               \___ installed! ___/
        (  -_-) zzZ   ~ your rhythm, your clock ~  (^-^ )
   .        *        .        *       .        *      .
   Every day is a little longer. Now your planner knows it.
=================================================================
   Launch:  ZeitBoard from the Start Menu (or it's already up)
   Update:  scripts\installer\update.ps1
=================================================================
```

Update finale swaps the marquee line for `up to date!` and prints
old → new commit; Android finale prints `APK READY` with the artifact
path in the box. Failure output uses no art (§1.4).

## 11. Rollout slices

| Slice | Scope | Acceptance |
|---|---|---|
| **I-A** | `_zb.common.ps1` + `pins.psd1` + `install.ps1` (desktop path only) | fresh Win11 VM: repo clone → one command → running tray app; re-run is a no-op; `-DryRun` in CI |
| **I-B** | `update.ps1` + rollback + data backup | update across a real commit pair; forced test-failure auto-restores `previous\` |
| **I-C** | `build-android.ps1` | debug APK from a machine with no prior JDK/SDK; release path refuses without keystore |
| **I-D** | `install-server.ps1` + `uninstall.ps1` + Pester suite | service survives reboot; uninstall leaves data; `-PurgeData` ceremony verified |
```
