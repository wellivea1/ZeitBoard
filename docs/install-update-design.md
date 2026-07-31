# Installation and update tooling

> Implemented contract for the ZeitBoard Windows lifecycle scripts. Windows
> PowerShell 5.1 is the compatibility floor. Linux server operation remains a
> manual path in `docs/self-hosting.md`; there is no Linux installer twin yet.

## 1. Scope and guarantees

The tooling consists of a shared, side-effect-free-on-load PowerShell library
and five entry scripts:

```text
scripts/installer/
  _zb.common.ps1
  pins.psd1
  install.ps1
  update.ps1
  build-android.ps1
  install-server.ps1
  uninstall.ps1
  test-installer.ps1
```

The implemented guarantees are deliberately narrower than "one command can
never fail":

1. Downloads are version-pinned and SHA-256 verified before extraction.
2. Tool archives are staged before publication. Incomplete local tool folders
   are quarantined instead of silently reused.
3. Desktop, MCP, daemon, config, and version metadata publication verifies the
   installed bytes. Replacement paths retain one prior release for rollback.
4. A publication failure attempts to restore the previous coherent release and
   reports recovery failure separately from the original error.
5. Re-running install reconciles selected behavior. Update exits early only when
   the installed hashes verify and its commit and declared component set exactly
   match fetched `HEAD`; `-ForceRebuild` explicitly bypasses that no-op.
6. `%APPDATA%\ZeitBoard` is never modified by install or update. Uninstall
   preserves it unless `-PurgeData` is selected and `DELETE` is typed.
7. `-PurgeData` creates a raw recovery ZIP first. That ZIP can contain database
   files and tokens; it is not the portable ADR-0014 export.
8. Dry-run prints the phase plan and skips every action block. It has no intended
   side effects, but it is not a substitute for dependency, build, or runtime
   validation.
9. Startup entries, shortcuts, services, and firewall rules are changed only
   when their current target is owned by this ZeitBoard installation. A
   same-named foreign target is preserved.
10. Entry points take one named lifecycle mutex. Concurrent lifecycle commands
    in the same Windows session fail before mutation; update releases the mutex
    before restarting under newly pulled installer code.

Failures inside an `Invoke-ZbStep` action print the failed step, log location
when logging is active, and an exact resume command. Entry-script catch blocks
print the underlying error. Success art is never printed on failure.

## 2. Shared library contract

`_zb.common.ps1` is dot-sourced by each entry script and does not perform work
when loaded. Its main responsibilities are:

- `Get-ZbPaths`: repo, tool, install, data, and temporary log locations.
- `Start-ZbLog`, `Write-ZbLog`, and `Invoke-ZbStep`: transcript logging, step
  boundaries, resume hints, check hooks, and dry-run behavior.
- `Enter-ZbLifecycleLock` and `Exit-ZbLifecycleLock`: process serialization.
- `Get-ZbPins`, `Test-ZbPins`, `Get-ZbExpectedHash`, and
  `Install-ZbArchivePin`: runtime pin and path-containment validation, HTTPS
  downloads, hash checks, staged extraction, wrapper-directory normalization,
  and collision-safe quarantine of incomplete installs.
- `Assert-ZbGo`, `Assert-ZbNode`, `Assert-ZbWails`, and `Assert-ZbWebView2`:
  exact tool probes and process-local PATH changes. Nothing edits machine PATH.
- `Assert-ZbRepoClean`, `Get-ZbVersionStamp`, and `Backup-ZbData`.
- `Publish-ZbVerifiedFile`, `Publish-ZbDesktopBuild`,
  `Restore-ZbPreviousBuild`, and `Test-ZbInstalledBuild`: staged publication,
  SHA-256 metadata, post-publication verification, and restoration of the prior
  destination on any failed publication.
- Service, executable, firewall, shortcut, startup-entry, and server-root
  ownership guards.
- ASCII-only banners and finale output.

The test runner uses plain PowerShell assertions instead of Pester so the same
suite runs under stock Windows PowerShell and GitHub Actions.

## 3. Dependency matrix

| Dependency | Accepted or pinned version | Implemented behavior | Elevation |
|---|---|---|---|
| Git | system Git 2.40+ | Required preflight; never bootstrapped | No |
| Go | system Go 1.26.x or pinned Go 1.26.0 | Portable ZIP under `.tools\go` when needed | No |
| Node | exactly v24.16.0 | Portable ZIP under `.tools` when needed | No |
| Wails CLI | exactly v2.12.0 | `go install` into `.tools\bin`; installed version is checked | No |
| WebView2 | installed Evergreen runtime | Detect registry; offer per-user `winget`; fail with the official manual-install URL if declined or unavailable | No |
| npm dependencies | root lockfile | `npm ci` at workspace root | No |
| JDK for Android | JDK 17 through 21 accepted; Temurin 17.0.12+7 fallback | Existing JDK/Android Studio JBR first, then pinned portable ZIP | No |
| Android command tools | 15859902 | SHA-pinned portable bootstrap | No |
| Android platform/build tools | API 36.1 / build-tools 36.1.0 | `sdkmanager` after explicit license consent | No |

The Android license flow first asks for consent. Only after consent does the
script feed automated `y` responses to `sdkmanager --licenses`. This keeps the
license decision explicit without requiring the user to answer repeated copies
of the same accepted prompt.

## 4. Desktop install

`install.ps1` performs these phases:

1. Check Windows 10 build 17763+, Git 2.40+, repository cleanliness, disk space,
   and administrator status when `-WithServer` was selected.
2. Resolve Go, Node, Wails, and WebView2.
3. Run `npm ci` and `wails build`.
4. Build the MCP connector when `-WithMcp` is selected or one is already
   installed. An existing connector is always kept in sync with the desktop.
5. Stage, publish, and hash-verify the desktop and optional MCP artifacts.
6. Reconcile Start Menu, desktop shortcut, and HKCU startup choices.
7. Delegate optional server and Android work.
8. Reverify installed hashes, optionally launch, and print the success finale.

The desktop/MCP publication is one recovery unit. It handles an existing
release, a legacy release without hash metadata, and an unusual pre-existing MCP
connector without a desktop executable. Optional server and Android delegates
are separate operations; failure in one does not pretend that earlier desktop
publication never happened.

Behavior flags are `-StartMenu`, `-DesktopShortcut`, `-Startup`, `-NoLaunch`,
`-WithServer`, `-WithMcp`, `-WithAndroid`, `-AcceptAndroidLicenses`,
`-NonInteractive`, `-AllowDirty`, and `-DryRun`. `-WithServer` requires the
parent PowerShell process to already be elevated; the script does not relaunch
itself with elevation midway through a transaction.

## 5. Update and rollback

`update.ps1` uses this order:

1. Verify the current installed hashes and repository state.
2. Fetch the tracked upstream and pull only with `--ff-only` after consent.
3. Restart under newly pulled installer code when a pull occurred.
4. Resolve the installed commit to the fetched repository, compare the exact
   declared component set, and exit when the verified release is already
   current unless `-ForceRebuild` was selected. Dirty trees allowed with
   `-AllowDirty` rebuild because commit metadata cannot describe local changes.
5. Resolve toolchains and run `npm ci`.
6. Run frontend and Go tests unless `-SkipTests` was selected.
7. Build desktop and any installed/requested MCP connector while the current app
   remains available.
8. Require the desktop and MCP processes to be stopped, then create a quiesced
   raw data backup.
9. Publish and verify the new artifacts, restoring and verifying the previous
   coherent install if publication fails.

A test or build failure happens before publication, so there is nothing to roll
back in that case. `update.ps1 -Rollback` restores `previous\` and verifies it.
Application data is never rolled back automatically.

Dry-run does not fetch or claim that a cached upstream ref is current. It prints
the conditional no-op decision and the full plan that would run if fetched
`HEAD` or the requested component set requires rebuilding.

`-AllowDirty` prints `git status --porcelain` and proceeds with those local
changes in place. It does not discard, reset, or stash them. Git may still
refuse a fast-forward pull when local files conflict.

## 6. Android builder

`build-android.ps1` resolves JDK and SDK tools, reconciles one `sdk.dir` entry in
`local.properties`, then runs `assembleDebug` by default.

`-Release` requires an existing keystore and alias. Passwords come from a secure
prompt or `ZEITBOARD_KEYSTORE_PASS` / `ZEITBOARD_KEY_PASS`; signing inputs are
passed through process-only environment variables, not command-line arguments.
The script does not create or choose a signing key. It verifies the resulting
APK with pinned `apksigner` and optionally runs `adb install -r` only when
exactly one authorized device is attached.

## 7. Windows server installer

`install-server.ps1` requires an elevated PowerShell process and defaults to the
canonical `%PROGRAMDATA%\ZeitBoard` root. It:

- stages the daemon build in a per-run temporary directory before service
  downtime;
- verifies ownership and stops an existing service before one fail-closed walk
  that rejects personal-file subtrees, reparse points, unsafe roots, or
  unrelated non-empty roots;
- marks the managed root with a protected inheritable SYSTEM/Administrators
  policy and writes a versioned completion marker only after existing
  descendants are reset to inherit it; recursive `icacls` is skipped only when
  both the exact root policy and marker are current, so a root-only match cannot
  preserve stale explicit child permissions;
- generates 32-byte data and enrollment secrets only when absent;
- preserves an existing config except for explicitly supplied listen/TLS values;
- validates the staged config with the staged daemon before publication or
  service-registration changes;
- normalizes explicitly supplied TLS paths to absolute paths and resolves paths
  already stored in config relative to the config file directory;
- atomically publishes daemon and config files;
- registers a delayed-auto Windows service with `sc.exe`;
- runs the daemon through the native Windows SCM handler, which reports Running
  only after the listener is established and performs bounded graceful shutdown;
- writes a 10 MiB rotating service log with one backup;
- defaults to loopback and requires TLS files for a non-loopback bind; and
- creates only an explicitly accepted Private-profile firewall rule tied to the
  managed daemon path.

Reruns refuse to modify a same-named service pointing to another executable.
Service failure recovery restores prior registration, description, start mode,
running/stopped state, daemon, and config. A failed first install removes newly
published daemon/config files but deliberately retains generated secrets and the
managed root so a rerun can recover safely. A failure after stopping an existing
service but before publication revalidates ownership and restarts the unchanged
service.

Firewall reconciliation is a post-service commit step. If that later step
fails, the script exits unsuccessfully but leaves the verified running service
in place and tells the operator to rerun the same command.

The tooling never enables the availability portal. `portal.enabled` defaults to
false, the installer writes no portal keys into the generated config, and a
daemon started by this tooling therefore serves no `/p/` route. Enabling it is
a deliberate operator edit, gated by the exposure checklist in
`docs/portal-design.md` section 12.

## 8. Uninstall and erasure

`uninstall.ps1` removes owned desktop binaries, MCP connector, shortcuts, and
startup state. `-RemoveServer` additionally removes the owned service and owned
firewall rules, but preserves the entire server root, including its data,
config, secrets, logs, and binaries.

`-PurgeData` applies only to `%APPDATA%\ZeitBoard`. It is rejected in
non-interactive mode, requires the exact typed word `DELETE`, creates a raw ZIP
under `%TEMP%\zeitboard-install`, warns that the backup is sensitive, and only
then recursively erases the desktop data directory. This is real filesystem
erasure and is distinct from append-only record suppression inside the app.

## 9. Verification and acceptance status

GitHub CI performs:

- contract fixture drift checks and trusted-prototype network guards;
- Go format, test, and vet on core, desktop service, Linux server, and Windows
  server builds;
- frontend and trusted-view tests/builds plus frontend lint;
- Android JVM tests, lint, and debug assembly after installing API 36.1 and
  build-tools 36.1.0 explicitly; and
- installer library tests plus dry-run coverage of all entry points and major
  optional branches.

Local verification in the implementation pass covered the server suites, all
PowerShell parsers, the installer regression suite, Android JVM tests/lint,
debug APK assembly, APK signature verification, emulator install, launch, and
screen-level accessibility/layout inspection.

The following remain manual release checks and must not be described as already
proven by CI:

| Check | Status |
|---|---|
| Fresh Windows 11 VM from clone to launched tray app | Pending manual release check |
| Update across two published commits and explicit rollback | Pending manual release check |
| Android bootstrap on a machine with no usable JDK or SDK | Pending manual release check |
| Release APK with the user's production keystore | Pending operator check |
| Elevated native service install, reboot survival, and uninstall on a clean VM | Pending manual release check |
| Interactive `-PurgeData` ceremony against disposable test data | Pending manual release check |

These checks are release acceptance gates, not implementation claims.
