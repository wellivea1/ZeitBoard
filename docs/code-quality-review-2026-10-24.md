# ZeitBoard code quality review — 2026-10-24

The completion goal asks for code of excellent quality as well as every planned function. This
review follows [`code-quality-review-2026-10-16.md`](code-quality-review-2026-10-16.md), which
measured main after PR #92. It measures the whole application again on the branch of PR #108:
main after PR #106, the ChatGPT sign-in (ADR-0066), with PR #107's changes. Beside each measure is
the earlier one, so a change shows. It also records what was done about what was found, and where
the evidence stops.

## Checks on every pull request

Continuous integration runs the same eight jobs, and a pull request merges only once all of them
pass:

| Job | What it holds the code to |
|---|---|
| Go (core), Go (server), Go (server-windows), Go (desktop-service) | `go vet`, every test, and the race detector for core, server and desktop |
| Web format, lint, types, tests, and builds | Prettier; ESLint, with cyclomatic complexity at most 24 and functions at most 300 lines; UI modules at most 600 lines; TypeScript; Vitest (669 tests, from 621); both builds |
| Android JVM tests, lint, and debug APK | the companion's tests, Android lint, a debug build |
| Contracts and synthetic fixtures | the v1 contracts and the fixtures generated from them |
| Installer tooling (dry-run) | the install and update scripts |

On this branch:
- `gofmt` and `go vet` are clean in every module, every Go test passes, and ESLint reports nothing.
- Android lint has two warnings, both asking for SDK 37, which is a download. The companion has not
  changed since they were counted.
- No TODO, FIXME, XXX or HACK marker is in the source.

## Coverage

Statements covered by all of a module's tests together:

| Module | 2026-10-16 | 2026-10-24 |
|---|---|---|
| `core` | 77.3% | 77.4% |
| `apps/server` | 73.9% | 75.4% |
| `apps/desktop` | 76.6% | 77.0% |

By their own tests, the packages that carry the most risk:

| Package | 2026-10-16 | 2026-10-24 |
|---|---|---|
| `apps/desktop/internal/calendaraccounts` | 82.7% | 82.1% |
| `apps/desktop/internal/loopback`, new with ADR-0066 | — | 91.4% |
| `apps/server/internal/auth` | 88.9% | 88.9% |
| `apps/server/internal/chatgpt`, new with ADR-0066 | — | 82.7% |
| `apps/server/internal/provider` | — | 80.6%, from 56.1% before this review |
| `core/storage/sqlite` | 73.6% | 73.7% |
| `core/calendar` | 70.8% | 70.8% |

The window's own code, by `npm run coverage` in `apps/desktop/frontend`:

| Measure | 2026-10-16 | 2026-10-24 |
|---|---|---|
| Statements | 85.0% | 85.2% |
| Branches | 78.6% | 79.3% |
| Functions | 85.0% | 85.3% |
| Lines | 88.8% | 88.9% |

**What coverage found.** The server's `provider` package stood at 56.1%. The OpenAI and Anthropic
clients, the two that an API key reaches, had no test at all. Nothing checked:
- what they ask;
- how they read an answer;
- how a refusal reads.

`hosted_test.go` now checks each against a local stand-in:
- OpenAI is asked through the Responses API to keep nothing, with the key as a bearer token, and
  its text is read from either place it may put it.
- Anthropic is asked with its key and version headers.
- An answer without text is not read as one.
- A usage limit, a context too large and any other failure each read as their kind, without
  repeating the provider's text.

## Complexity

The window's lint holds each function to a cyclomatic complexity of 24 and 300 lines. Nothing held
the Go code to any measure, so this review measured it. The scan reads every function outside the
tests, in core, the server and the desktop, with Go's own parser. It counts each `if`, `for`,
`case`, `&&` and `||`, closures included, as gocyclo does.

| Measure | Before this review | After |
|---|---|---|
| Functions | 2,115 in 227 files | 2,134 in 227 files |
| Complexity over 24 | 15 | 12 |
| Complexity over 40 | 3 | 2 |
| Longer than 150 lines | 7 | 6 |

Three were split, and nothing they do changed:

- **`Store.applySyncPullPage`** (complexity 67, 238 lines) applies a pulled page in one
  transaction, in seven phases:
  1. the records themselves;
  2. medication evidence;
  3. sleep corrections;
  4. corrections that waited for their night;
  5. medication records that waited for their definition;
  6. tombstones;
  7. the cursor, the commit and the compaction.

  Each of the first six is now a method of `syncPullApply`, which counts what each record did, and
  they run in the same order. A record's seen ID is a method of the record.

  Core's sync suites and the desktop's sync tests pass, and so does the store under the race
  detector.
- **`Account.FinishSignIn`** (31) is now its steps:
  - taking the waiting sign-in;
  - settling the client;
  - the exchange;
  - deciding whether the tokens may be kept;
  - the model a new connection starts with.
- **`App.assistantPlanningContext`** (32, 150 lines) now has a function for each of its sections:
  the windows and busy events, the tasks, the medication facts and the marker facts. The tests of
  the context's redaction pass unchanged.

The twelve still over 24:

| Function | Complexity | Lines | Kind |
|---|---|---|---|
| `parseEventComponent`, `core/calendar` | 61 | 189 | iCalendar parser, a case for each property |
| `sanitizeSnapshot`, server assistant | 46 | 129 | the chat snapshot's allowlist, field by field (ADR-0049) |
| `materializeEvents`, `core/calendar` | 38 | 120 | recurrence expansion within the work and horizon caps |
| `App.getCalendar`, desktop | 37 | 176 | reading a calendar of each kind |
| `Store.Append`, server | 35 | 153 | a sync batch, checked and written in one transaction |
| `Store.ListProposalPage`, server | 34 | 148 | a page of proposals by cursor and queue |
| `ValidateCorrection`, `core/sleepv1` | 31 | 61 | the v1 contract's checks of a correction |
| `Fold`, `core/sleepv1` | 30 | 106 | folding corrections into a night |
| `validateProposalDecisionInput`, `core/storage/sqlite` | 29 | 50 | checks of a decision |
| `collectEventComponents`, `core/calendar` | 29 | 70 | iCalendar parser |
| `validatePullEnvelopePage`, desktop | 28 | 59 | checks of a pulled page |
| `EncodeTranscriptionTemplate`, desktop history | 27 | 72 | the history template's encoder |

None was changed by this review:
- Most are parsers and validators, where each branch is one rule; they read best as the list they
  are.
- The rest are store transactions and the desktop's calendar read.
- Each is covered by its package's tests.
- The desktop's calendar read is a candidate for extraction along with the desktop's `App` facade,
  which the completion plan's limitation 7 accepts until a feature boundary calls for it.

The two longest functions are `Store.initialize` in core (340 lines, complexity 16) and the
server's `Store.Migrate` (168 lines, complexity 3). They are long because each lists every table and
index, which reads best as the one list.

## Dependencies

ADR-0066 added no dependency.
- The sign-in, its ID token checks and the stream use Go's standard library.
- The window's dialog uses React's own `createPortal`.

`npm audit` now lists twelve advisories, four moderate and eight high, up from nine. The three new
ones are:
- Vite's `launch-editor`, which the development server uses to open an editor on Windows;
- zod, which comes in only through the React hooks lint plugin;
- the coverage package, which shares Vitest's.

All twelve are in the tooling that builds, lints and tests the window, and none ships:
- the built window holds none of them;
- nothing in the app imports zod;
- the desktop, server and companion are Go and Kotlin.

Moving the tooling to patched versions is a download. It waits on the owner, as `release-checks.md`
section 7 says.

## Reviews since 2026-10-16

- **Data Sources and Settings.** The review of PR #97 and its findings are in the earlier record.
- **The ChatGPT sign-in (ADR-0066)** was read end to end: the server, the desktop and the window.
  Its findings and their dispositions are in the completion plan's "A review of the ChatGPT
  sign-in". Two of them, a dialog heading in the wrong style and focus lost after it, were found by
  looking at the window in the browser pane.
- **This review** found:
  - the providers' missing tests;
  - the three functions split above;
  - the advisory count, nine on record against twelve now.

## Where the evidence stops

- **Who reviewed.** The reviews were made by the agent that wrote the code, or by automated
  reviewers. The independent review milestone C5 asks for before the clinician portal may be
  exposed is still for a person.
- **The ChatGPT sign-in** was checked against a synthetic OpenAI. Signing in with a real account
  needs the owner's (`release-checks.md` section 6).
- **The providers.** Google, Microsoft and CalDAV are still checked against stand-ins that behave as
  their documentation says.
- **Complexity is a count, not a judgement.** The scan finds where to look; the dispositions above
  are the judgement, and they are open to the independent reviewer's.
