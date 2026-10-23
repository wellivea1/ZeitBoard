# ZeitBoard code quality review — 2026-10-16

The completion goal asks for code of excellent quality as well as every planned function. This
records what that rests on, as measured on main after PR #92, how it was established, and where
the evidence stops.

## Checks on every pull request

Continuous integration (`.github/workflows/ci.yml`) runs eight jobs, and a pull request merges only
once all of them pass:

| Job | What it holds the code to |
|---|---|
| Go (core), Go (server), Go (server-windows), Go (desktop-service) | `go vet`, every test, and the race detector for core, server and desktop |
| Web format, lint, types, tests, and builds | Prettier; ESLint, with cyclomatic complexity at most 24 and functions at most 300 lines; UI modules at most 600 lines; TypeScript; Vitest (621 tests); both builds |
| Android JVM tests, lint, and debug APK | the companion's tests, Android lint, a debug build |
| Contracts and synthetic fixtures | the v1 contracts and the fixtures generated from them |
| Installer tooling (dry-run) | the install and update scripts |

On main: `gofmt` and `go vet` are clean in every module, and ESLint reports nothing. No TODO,
FIXME, XXX or HACK marker is left in the source.

One check failed on a run that changed nothing it tested: the setup guide's "closes for good". The
test clicked the note's × as soon as it could find it, and React can show a note a moment before
it subscribes to being closed. A click in that moment closes the note an instant later, after a
check made at once has already failed. The test now waits for the note to go, and eight runs of it
in a row pass. A person cannot click within that moment, and the note reads the closing when it
subscribes, so the app was never wrong.

## Coverage

Statements covered by all of a module's tests together:

| Module | Covered |
|---|---|
| `core` | 77.3% |
| `apps/server` | 73.9% |
| `apps/desktop` | 76.6% |

By their own tests, the packages that carry the most risk:

| Package | Covered |
|---|---|
| `apps/desktop/internal/calendaraccounts` | 82.7% |
| `apps/server/internal/auth` | 88.9% |
| `core/storage/sqlite` | 73.6% |
| `core/calendar` | 70.8% |

The window's own code, measured 2026-10-18 by `npm run coverage` in `apps/desktop/frontend` with
`@vitest/coverage-v8`, which the owner allowed:

| Measure | Covered |
|---|---|
| Statements | 85.0% |
| Branches | 78.6% |
| Functions | 85.0% |
| Lines | 88.8% |

## Dependencies

`npm audit` lists nine advisories in the tooling that builds and tests the window: Vitest's
mocker, PostCSS, browserslist, nanoid, brace-expansion, source-map-js, undici and
baseline-browser-mapping. None of it ships in the app. The window's bundle holds its own code
and its runtime libraries, and the desktop, server and companion are Go and Kotlin. The coverage
package adds no advisory of its own; it shares Vitest's. Moving the tooling to patched versions is
a download, and waits on the owner like any other.

## Reviews

Every change to calendar accounts and to writing accepted times was reviewed by an automated code
reviewer at high or medium effort. Its findings were fixed or answered before merging. Each round
is recorded in the completion plan, which keeps what was declined and why:

- The calendar accounts (PR #82) and writing to accounts (PR #83) were reviewed at high effort.
  Ten findings each were fixed, in PRs #88 and #89.
- PR #89 was reviewed again, at high effort, and its nine findings were answered in PR #90. Five
  further rounds then reviewed PR #90 itself. Later rounds raised mostly what earlier ones had
  settled, and the last, at medium effort, found one gap older than the work under review. The
  completion plan records the fixes as "Writing to accounts after a second review" through "after
  a sixth review".
- That gap, the unconfirmed create of a time undone before a retry, was closed in PR #92. Three
  reviews of PR #92 led to two redesigns: what is found waits for the owner, and a lookup a busy
  time defeats gives up rather than wait for good.
- A security review of PR #89 found no vulnerabilities.
- PR #97, which made Data Sources and Settings say what is true on every computer, was reviewed
  twice, on 2026-10-18 at medium and then low effort. Usage limits cut both runs short. Only the
  simplification reviewer finished, and its eight findings were applied in PR #98:
  - Settings and Data Sources now name desktop activity's state through one helper. A system that
    cannot record had read "Off" on one screen and "Unavailable" on the other.
  - The computer's zone is read in one place, where eleven had each written their own fallback.
  - The calendar zone is no longer copied into state that the browser preview's fixture could
    overwrite with UTC.
  - The rest made repeated code shorter.

  The bug-finding reviewers were then resumed, and in the end all five reported. PR #98 fixes
  everything they found, recorded in the verification log as "What the review of Data Sources and
  Settings found". Among the findings:
  - a status row read only once;
  - a Retry the desktop always refuses;
  - promises of a tray that may not exist;
  - a clock stood in for when the desktop does not answer;
  - a host zone no formatter accepts, which stopped three screens opening;
  - calendars placed in one zone when added and another when refreshed.

Two points the reviews raised more than once are settled by the project's own rules:

- **Compatibility with development data.** The owner's clarification of 2026-09-08, at the head
  of the completion plan, says there are no deployed users or releases. So names and formats that
  changed before any release need no migration.
- **Asking rather than guessing.** Where ZeitBoard cannot tell whether the owner changed an event,
  it asks them, as ADR-0053 asks before removing a changed event.

The fixes that rest on a single mechanism were checked by breaking that mechanism and watching its
test fail:

- the trigger that writes a time accepted again once its earlier removal is done
- the name for each time of writing, which decides whether a removed Google event is put back
- the trigger that turns an undone, unconfirmed create into a lookup

## Where the evidence stops

- **Who reviewed.** The reviews were automated. The independent review that milestone C5 asks for,
  before the clinician portal may be exposed, is still for a person.
- **The running app.** On 2026-10-18 every screen of the desktop window was checked in the
  built-in browser pane, which now loads the Wails bridge. It ran against a throwaway data folder
  holding synthetic nights. The check found five defects, all now fixed and recorded in the
  completion plan's "The running app". On 2026-10-23 Narrator was run through the window by
  keyboard, and the accessibility tree of every screen was swept. TalkBack on the phone is still to
  do.
- **The providers.** Google, Microsoft and CalDAV are checked against stand-ins that behave as their
  documentation says. ADR-0065 records the two behaviours of Graph's that need a real account to
  confirm.
