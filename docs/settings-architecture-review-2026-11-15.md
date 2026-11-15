# Settings architecture review, 2026-11-15

This review reads how ZeitBoard keeps what the owner chooses: where each choice is stored, who
can read it, and what happens to it on export, erasure and a second device. It was made while
adding the choices for a reported status ([ADR-0070](decisions/0070-a-status-the-owner-reports.md)),
which is the newest thing to be kept and so the easiest place to see how the others are.

## Where choices are kept today

| What | Where | Kept by |
|---|---|---|
| Theme, reduced stimulation, night rule | `appearance.json`, and again in the window's `localStorage` | Computer |
| Clock and region | `regional.json`, mirrored to `localStorage` (`zeitboard.clock`) | Computer |
| Reaching hours | `reaching.json` | Computer |
| Status choices (share by default, follow the sleep buttons) | `status.json` | Computer |
| Windows file encryption on or off | `storage-encryption.json` | Computer |
| Start with Windows | Windows registry | Computer |
| Activity collection consent | SQLite, `source_collection_preferences` | Computer |
| Server connection and its credential | SQLite, `local_sync_connection` | Computer |
| Which calendar accepted times are written to | SQLite | Computer |
| Hidden notices, week range, assistant welcome seen | `localStorage` only | Computer, window |
| Sample or own data, 24-hour clock, background refresh, time-request notices, share reported status | `SharedPreferences` (`non24_settings`) | Phone |
| Server address, device token, home zone | `SharedPreferences`, separate file | Phone |
| What each share link shows | The server's portal store | Server |
| Listen address, portal, trusted proxies, limits | `config.json` and environment | Server |

The four JSON files on the first rows share one store, `localSettingsStore[T]`. It writes a
schema version and a revision, keeps a backup beside the file, writes atomically, and refuses a
save made against a stale revision. That is the best part of the design, and `status.json` uses
it unchanged.

## What works

- **A setting sits where it is used.** Reaching hours are on Reaching, status choices are on
  Sharing beside the links they govern, and Settings holds what belongs to the whole device.
  The phone follows the same rule. Nothing has to be hunted for in one long page.
- **Private by default is consistent.** Every sharing choice starts off, on every device.
- **Security choices are not treated as preferences.** Consent, the server connection and the
  credential are in the database with the data they protect, not in a preferences file.
- **Stale saves are refused.** Two windows cannot silently overwrite each other's choice.

## Findings

Ranked by what they could cost.

1. **Nothing lists the stores.** There are five kinds of store on the computer alone, and each
   new one has to be remembered in several places by hand: the storage-protection list, export,
   erasure. Adding `status.json` meant adding it to the protection list separately, and
   `storage-encryption.json` is not in that list today. Nothing fails when a file is left out.
2. **Appearance is kept twice.** The theme, reduced stimulation and the night rule are in
   `appearance.json`, so the local agent can read and change them, and in `localStorage`, which
   is what the window's theme code reads. The two are kept in step by the window. If they
   disagree, which wins depends on which code path ran. `localStorage` also belongs to the
   page's address, so a build served from another address starts without it. The clock already
   does this the right way round: the desktop decides, and `localStorage` only remembers the
   last answer so the first page is drawn with it.
3. **Some lasting choices live only in the window.** Closing a notice is meant to be for good
   (`ui-refactor-plan.md` §14). The list of closed notices is in `localStorage`, so clearing the
   webview's data brings every notice back, and no export or erasure knows the list exists. The
   week range and the assistant welcome are view state and are fine there.
4. **Choices do not travel between devices.** "Let my share links show it" is set separately on
   the computer and on the phone. That is defensible, since each device asks at the moment of the
   report, but someone who turns sharing off on one device may believe they turned it off. The
   phone's wording says where the per-link choice is made; neither says the default is this
   device's alone.
5. **An old reader is still in the code.** `app_reaching.go` reads `reaching-hours.json`, the
   single schedule kept before ADR-0056. `AGENTS.md` says not to keep migrations for development
   schemas before the first packaged release.
6. **Only appearance can be read or changed by an assistant.** `AGENTS.md` asks every feature for
   an agent-operable interface. A person using ZeitBoard by voice cannot ask what their reaching
   hours or status choices are, or propose a change to them.
7. **What a link shows was fixed when the link was made.** This is now changed for waking
   windows, skipped doses and reported status (ADR-0070). Requests, messages, expiry and the
   passcode are still fixed; changing those still means a new link.
8. **The encryption choice has its own reader and writer.** `storage-encryption.json` does not
   use the shared store, so it has no backup file and no revision.

## Recommendations

In the order I would do them.

1. **One table of stores, in code.** Each entry names the file or table, what it holds, and
   whether it is exported, erased with the data, and checked for protection. The protection
   list, export and erasure read the table, and a test fails when a settings file exists that
   the table does not name. This closes finding 1 and stops it recurring. About a day.
2. **Make `appearance.json` the one source, as the clock is.** The window keeps a copy only to
   paint the first frame in the right theme, and overwrites it from the file on load. Closes
   finding 2.
3. **Move closed notices to a settings file.** One more `localSettingsStore`. Closes finding 3.
4. **Say that sharing defaults are per device**, in one sentence under each switch. Syncing a
   small owner-preferences record is the larger answer and needs a decision first: it would be
   the first setting to leave a device.
5. **Remove the `reaching-hours.json` reader**, once the owner confirms their own profile has
   been opened since ADR-0056. It is the only step here that could lose a real choice.
6. **Give the assistant a read-only view of non-security settings**, then propose-only actions
   for reaching hours and status choices. Consent, the server connection and encryption stay
   with the person.
7. **Put the encryption choice on the shared store.**

## Changed with this review

- `status.json` is kept through `localSettingsStore` and is in the storage-protection list.
- What a link shows can be changed after it is made.

Nothing else above is changed. Recommendations 1 to 3 and 7 are contained and can be done without
a decision; 4 to 6 need the owner's.
