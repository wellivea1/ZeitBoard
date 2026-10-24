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
| Go (core), Go (server), Go (server-windows), Go (desktop-service) | `go vet`, every test, and the race detector for core, server and desktop; since PR #109, through `core/codequality`, cyclomatic complexity at most 24 and functions at most 300 lines, as the window's lint |
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

| Measure | Before this review | After PR #108 | After PR #109 |
|---|---|---|---|
| Functions | 2,115 in 227 files | 2,134 | 2,187 |
| Complexity over 24 | 15 | 12 | 0 |
| Complexity over 40 | 3 | 2 | 0 |
| Longer than 150 lines | 7 | 6 | 2 |
| Longer than 300 lines | 1 | 1 | 0 |

**PR #108** split three functions, and nothing they do changed:

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

**PR #109** brought every other function within the window's limits, the same way. In each, the
checks, their order and their messages are unchanged:

| Function | Was | Now |
|---|---|---|
| `parseEventComponent`, `core/calendar` | 61, 189 lines | the UID and start, then one reader each for text, status, duration and recurrence |
| `collectEventComponents`, `core/calendar` | 29 | a walk with a method each for BEGIN, END, a property, and the end of the calendar |
| `materializeEvents`, `core/calendar` | 38 | the series grouped, then each series added. The check of an exception against its series was written twice, and adding an event at the cap three times; each is now one function |
| `sanitizeSnapshot`, server assistant | 46 | one method for each part of the snapshot the model may read, with the allowlist in each beside its part |
| `Store.Append`, server | 35 | a tombstone lookup it repeated five times is one function. Then whether a record, or what it depends on, was erased, and then keeping it |
| `Store.ListProposalPage`, server | 34 | the cursor, the query for its queue, the rows, the page and its tokens, and the pending count |
| `App.getCalendar`, desktop | 37, 176 lines | the days asked for, each calendar's description, the empty days, events and forecast bands placed on them, and a sort |
| `ValidateCorrection`, `core/sleepv1` | 31 | identity, provenance and changes, checked in that order |
| `Fold`, `core/sleepv1` | 30 | the targets, the checked corrections, the active leaves, and applying them |
| `validateProposalDecisionInput`, `core/storage/sqlite` | 29 | the proposal decided on, then the record of the decision |
| `validatePullEnvelopePage`, desktop | 28 | the page, then whether each record names itself |
| `EncodeTranscriptionTemplate`, desktop history | 27 | what the template covers, then the writing |
| `Store.initialize`, core | 340 lines | the schema is a named list the store applies |

One of them had a latent fault. `validateProposalDecisionInput` checked four identifiers by ranging
over a map, so with several invalid it named whichever came first on that run; it checks them in a
fixed order now. Every test in core, the server and the desktop passed after each split.

**The limits hold from now on.** `core/codequality` measures every function as this review did. A
test in each Go module runs it over that module's own source:
- `core/codequality`;
- `apps/server/internal/quality`;
- `apps/desktop/internal/quality`.

A function past 24 or 300 lines fails its module's job in CI, as it would fail the window's lint.
Its own tests check the counting against a sample whose complexity is worked out by hand.

The longest functions left are the server's `Store.Migrate` (168 lines, complexity 3), the
server's schema list, and the daemon's `serve` (154 lines, complexity 19).

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
- **The ChatGPT sign-in's server half (PR #105)** was then reviewed by an automated reviewer at
  medium effort. It raised ten findings, and all ten are fixed (completion plan, "The ChatGPT
  sign-in after an automated review"). Among them were five faults:
  - a context too long that the assistant never asked again with less;
  - a rotated token left live when the plan was withdrawn;
  - consent not asked again;
  - a refusal in the stream that did not end the sign-in;
  - a sign-in route that could outlast the server's response limit.
- **The computer's half (PR #106)** was reviewed by the same reviewer at medium effort. It raised
  ten findings, and all are settled (completion plan, "The ChatGPT sign-in's computer half after an
  automated review"). Among them:
  - a Cancel that could stop a sign-in midway;
  - a browser page that claimed success early;
  - raw server answers shown to the owner;
  - a store fault read as sync turned off.
- **This review** found:
  - the providers' missing tests;
  - fifteen functions past the window's complexity limit and one past its length, all now split;
  - a validator that named a different fault from run to run;
  - the advisory count, nine on record against twelve now.

## Where the evidence stops

- **Who reviewed.** The reviews were made by the agent that wrote the code, or by automated
  reviewers. The independent review milestone C5 asks for before the clinician portal may be
  exposed is still for a person.
- **The ChatGPT sign-in** was checked against a synthetic OpenAI. Signing in with a real account
  needs the owner's (`release-checks.md` section 6).
- **The providers.** Google, Microsoft and CalDAV are still checked against stand-ins that behave as
  their documentation says.
- **Complexity is a count, not a judgement.** The limits keep functions small enough to read, but
  not well named or well divided. The splits above are the agent's judgement of that, and open to
  the independent reviewer's.
