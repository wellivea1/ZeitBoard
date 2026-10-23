# What is left, and what each step needs

Every planned function is built, and the software of every milestone is done. What remains is
evidence and decisions, and they need things only the owner has: their own computer and phone, a
clean machine, the release signing key, a reviewer who did not write the portal, people for the
pilot, and the owner's own decisions. Each step below says what it needs, what to do, what passing
looks like, and where the result is recorded. None of it is started without the owner.

The completion plan's "Where the completion goal stands" keeps the overall picture; this is the
how.

## 1. Sign-in, sleep and wake on the owner's computer (C1)

**Needs:** the owner's Windows computer, and their agreement that ZeitBoard may register itself to
start at sign-in (the `ZeitBoard` value in the current user's Run key, ADR-0043) and that the
computer may be put to sleep.

1. In Settings › This computer › When you sign in, tick **Start ZeitBoard when I sign in** and
   **Start in the tray**, then **Save startup settings**. It should say *Registered with Windows*.
2. Turn desktop activity on in the same tab, if it is not on already.
3. Sign out and sign in again. **Passes** if ZeitBoard starts in the tray with no window. Windows
   may delay startup apps; note how long it took. If it does not start, Windows Settings › Apps ›
   Startup says whether Windows disabled it.
4. Put the computer to sleep for at least fifteen minutes, then wake it. **Passes** if Data Sources
   says desktop activity is *Running* again, Home's forecast is current rather than stale, and
   **Export activity records** (Settings › This computer) shows the computer suspended and resumed
   at those times.
5. Close the window, then choose Quit from the tray. **Passes** if the window hid at the close, and
   a later export of the activity records shows a shutdown at the moment of quitting.
6. To undo, untick **Start ZeitBoard when I sign in** and save.

**Record:** a dated section in `verification.md`; C1's native evidence is then done.

## 2. A clean Windows 11 machine (C8)

**Needs:** a fresh Windows 11 virtual machine with Git and nothing else for development. The
scripts are in `scripts/installer`; what each does is in `install-update-design.md`. A fresh
Windows does not run scripts, so each is run from the repository as
`powershell -ExecutionPolicy Bypass -File scripts\installer\<script>.ps1`.

1. **Install.** Clone the repository and run `install.ps1`. **Passes** if the tray app launches and
   the installer's closing hash check passes.
2. **Update and roll back.** Install at one published commit; once the next is published, run
   `update.ps1`, then `update.ps1 -Rollback`. **Passes** if each step verifies its hashes, the
   rollback brings back the earlier build, and the data is untouched.
3. **Android bootstrap.** On the same machine, with no JDK or Android SDK, run `build-android.ps1`;
   it asks before accepting the Android SDK licences (`-AcceptAndroidLicenses` gives that consent
   in advance). **Passes** if it installs both and builds the debug APK.
4. **Server.** In an elevated PowerShell, run `install-server.ps1`, restart the machine, then run
   `uninstall.ps1 -RemoveServer`. **Passes** if the service is running after the restart, and the
   uninstall removes it while keeping the server's data root.
5. **Erasure.** With disposable test data in the desktop app, run `uninstall.ps1 -PurgeData` and
   type `DELETE`. **Passes** if a backup ZIP is written first and `%APPDATA%\ZeitBoard` is then
   gone.

**Record:** the date against each row of `install-update-design.md` section 9, and a dated section
in `verification.md`.

## 3. The release signing key (C8)

**Needs:** the owner's production keystore and alias. ZeitBoard never creates or chooses one.

Run `build-android.ps1 -Release -Keystore <path> -KeystoreAlias <alias>`, giving the passwords at
its prompt or in `ZEITBOARD_KEYSTORE_PASS` and `ZEITBOARD_KEY_PASS`. **Passes** if `apksigner`
verifies the APK.

**Record:** the "Release APK" row of `install-update-design.md` section 9.

## 4. An independent review of the portal (C5)

**Needs:** a reviewer who did not write the portal.

Give them `portal-design.md`, `threat-model.md`, `privacy.md`, the TLS and reverse-proxy runbook in
`self-hosting.md`, and the code with its tests: `apps/server/internal/portal`, the public side, and
`apps/server/internal/portalbridge`, which decides what the public side may see (its leak tests are
in `leak_test.go`). Their task is item 6 of the exposure gate in `portal-design.md` section 12: find
every high- and medium-severity issue, and see each resolved.

The portal stays off (`portal.enabled` is false) until the whole gate passes. Turning it on is then
the owner's decision, not part of the review.

**Record:** the findings and their resolutions in the completion plan, under C5.

## 5. The private pilot (C1, C8)

**Needs:** the owner's own phone and computer in daily use for 30 to 60 days, and anyone else the
owner chooses to include.

Measure what the completion plan's "What counts as finished" lists: passive coverage and manual
burden, sync latency, stale-output incidents, forecast error, coverage and width by horizon, task
acceptance and completion, and CPU, memory, battery and storage cost. A synthetic run cannot stand
in for any of it.

**Record:** a pilot report, from which the release decision follows.

## 6. Two decisions

- **The development-tool advisories.** `npm audit` lists nine, all in the tooling that builds and
  tests the window; none ships in the app (`code-quality-review-2026-10-16.md`, Dependencies).
  Fixing them means downloading newer tooling. The owner either allows that, after which the web
  checks are run again, or accepts them as development-only.
- **The release disposition.** Once the checks above are in, the owner records in the completion
  plan whether ZeitBoard is released, how, and to whom.
