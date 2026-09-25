# ADR 0043: Login startup and recoverable desktop windows

- Status: accepted
- Date: 2026-09-08
- Continues completion-plan C1 after ADR-0042.

## Decision

Settings now offers explicit per-user Windows login startup, either visible or in
the tray. The OS registration is authoritative; there is no second saved boolean
that can re-enable a removed startup entry. Enabling startup does not change
activity, sync, assistant or notification consent. A moved executable is shown as
needing an update, and an isolated `ZEITBOARD_DATA_DIR` profile cannot change the
normal login entry.

The Windows adapter owns only the `ZeitBoard` value in the current user's Run key.
The two current commands are a quoted absolute executable path and that same path
with `--background`. Executable paths and the 260-character command bound are
validated. The installer uses the background form; removal recognizes both current
forms while preserving entries pointing to another installation. No legacy
unquoted command adapter is retained. Windows may delay or disable startup apps,
so the UI says **registered**, not that execution at logon is guaranteed. See
[Microsoft's Run-key documentation](https://learn.microsoft.com/en-us/windows/win32/setupapi/run-and-runonce-registry-keys).

Wails uses one instance per resolved data profile. Launching again opens the
existing window; a second background launch does not interrupt it. The profile
key is an opaque digest, and second-instance arguments are not interpreted as
files, imports, URLs or commands.

The window lifecycle separates close/hide from explicit quit. A normal close
hides only when the tray is available. The tray's Quit and the Settings Quit
button mark an explicit quit before calling Wails, so `OnBeforeClose` cannot
intercept them as another hide. If the tray fails, the window remains accessible
and closing quits. Startup and shutdown serialize service setup/teardown so a
quick close cannot leave a worker using a closed database.

A background first launch waits for both WebView readiness and tray initialization
before deciding whether a fallback window is needed. Either callback order works.
The Windows tray handles Explorer's `TaskbarCreated` message by republishing its
icon; a failed recreation or a stopped tray reveals the window. These callbacks
carry availability only, without health content.

## Verification and remaining work

Focused tests cover both startup callback orders, close versus explicit quit,
tray loss/recovery, second-instance dispatch, profile isolation, unchanged consent,
registry failure and command quoting. Windows registry integration tests use an
isolated non-Run key, so they do not register an actual login task. Installer
tests likewise use disposable paths and registry state. Native adapter dispatch
is tested without restarting the owner's Explorer process.

Background estimate projection refresh and desktop enrollment/restore reconciliation
remain the next C1 software work. Native Windows login/suspend/resume qualification,
installation on a clean supported machine, and the other C1–C8 operational gates
remain open. This change neither installs login startup on the owner's machine nor
publishes a release package.
