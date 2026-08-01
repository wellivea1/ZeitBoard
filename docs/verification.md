# Verification record

Most recent local verification: Windows 11 on 2026-07-22.

## Passing checks

- Frontend formatting, ESLint, and repository UI standards. The UI guard passed
  with 16 screen modules and 10 component stylesheets.
- Frontend TypeScript and production builds for the desktop and trusted-view
  workspaces.
- Frontend tests: 30 desktop test files with 213 tests, plus 2 trusted-view test
  files with 6 tests.
- `scripts/dev.ps1 -Action check -Component desktop`: desktop production build.
- `scripts/dev.ps1 -Action check -Component core`: Go formatting, tests, and vet
  for `core` and `apps/desktop`, including the Windows tray package.
- `scripts/dev.ps1 -Action build -Component core`: Go builds for `core` and
  `apps/desktop`.
- `scripts/dev.ps1 -Action check -Component server`: server formatting, tests,
  and vet, including the server/MCP projection privacy allowlist.
- `scripts/dev.ps1 -Action check -Component contracts`: deterministic drift
  check for 26 v1 fixtures plus 3 v2 fixtures, tools tests/vet, and schema
  validation.
- Native Wails production build at `apps/desktop/build/bin/ZeitBoard.exe`.
- Android `testDebugUnitTest`, `lintDebug`, and `assembleDebug`; U-G changes no
  Android source.
- Browser QA of the medication clinician-report workflow at 1440 x 900 and
  390 x 844: no page-level horizontal overflow, no console warnings or errors,
  and the dense chart, tables, controls, and explicit internal chart scrolling
  remained usable. Changing report controls invalidated export until a fresh
  preview was generated, and export stayed disabled until the exact `EXPORT`
  confirmation was entered.
- Sleep erasure regression: suppression remains exportable and excluded from
  effective reads; hard deletion removes observation/correction rows and the
  unique payload marker from the compacted SQLite database and WAL.
- Medication evidence regression: exclusion remains an append-only correction
  and stored evidence remains countable/exportable; event erasure removes the
  selected event and correction chain without deleting its definition, while
  medication erasure removes the definition and all dependent evidence. Both
  paths remove unique payload markers from the compacted SQLite database and
  WAL.
- Medication schedule regression: strict as-needed/fixed/cycling validation,
  explicit IANA zones, civil cycle boundaries, DST gaps, first repeated-time
  occurrence, and real estimator-horizon collision counts pass. Reminder tests
  verify explicit opt-in, claim-before-notify, durable at-most-once behavior,
  no retry after notification failure, inactive-definition pause, private-label
  control-character normalization, immutable claims, and erasure cascade.
- Medication clinician-report regression: local civil range and DST handling,
  observed and inferred sleep layers, opt-in forecast and private fields,
  pseudonymized medication labels, explicit taken/skipped adherence only,
  rhythm-context annotations, selected-range drift, and robust descriptive
  before/after medication-start association all pass. Export requires the exact
  confirmation phrase and emits canonical, script-free standalone HTML with a
  restrictive CSP, complete chart text alternative, mandatory redactions, and
  no IANA zone identifier. A transient renderer pass also exercised the full
  standalone document outside the desktop response adapter.
- Rhythm-context regression: the four non-diagnostic marker kinds and manual,
  user-reported provenance are schema-closed; SQLite rows are immutable; local
  civil inputs reject future times and DST gaps while selecting the first
  repeated-time occurrence; and adding a marker leaves the estimator projection
  unchanged under an exact deep comparison. Contract-shaped export passes,
  trusted-view contracts reject markers and private notes, actogram joins reject
  a marker whose explicit zone does not match the row clock, and hard erasure
  removes a unique private note from both the compacted database and WAL.
- Calendar ownership regression: bounded ICS/CalDAV preview and import,
  recurrence/DST parsing, immutable imported rows, and source erasure all pass.
  Erasure removes private labels, titles, locations, notes, and saved endpoints
  from both the compacted SQLite database and WAL. Text-free scheduler
  projection, task/sleep/event stale-decision refusal, app-owned approval
  materialization, rejection, undo, and import-free ICS export also pass.

The check wrapper itself was also corrected: native Go, npm, Wails, contract,
and Gradle failures now propagate as a non-zero script result.

## Real-history import and estimator gate

Only aggregate results are recorded here. Raw exports, converted observations,
the validation SQLite databases, rendered charts, and detailed refusal points
remain private and ignored.

The finalized digital source conversion accounted for every source row:

| Stage | Count |
|---|---:|
| Finalized Fitbit files read | 35 |
| Source rows read | 923 |
| Exact overlapping rows | 105 |
| Rows outside 2021-2023 | 80 |
| Included observations | 738 |
| Included observations under 3h, classified as nap | 1 |
| Matching superseded files excluded (`Old` / `Incomplete` / `weekly`) | 26 |

Against a fresh ignored database, preview reported 738 ready, 0 duplicate,
and 0 invalid rows. Commit imported all 738 atomically. A second preview
reported 0 ready, 738 exact duplicates, and 0 invalid rows. The digital
history covers late October 2021 through December 2023.

The five original handwritten-chart pages were source-reviewed without OCR.
Their 241 labeled day rows use the Sleep Diary report's 18:00-to-18:00 layout.
Grid-aligned five-minute boundary estimates were visually checked on full-page
overlays. The date-complete review ledger contains 243 explicit statuses: 223
`confirmed_sleep` entries and 20 `confirmed_no_observation` entries. There are
two more statuses than chart rows because two rows contain two sleep episodes;
`confirmed_no_observation` means that a row was checked and contains no new
episode start, not that the owner did not sleep.

Eight chart episodes overlap finalized Fitbit records and were reserved for a
source-accuracy check. Across their 16 start/end boundaries, absolute chart
error versus Fitbit was 23.2 minutes mean, 10.0 minutes median, 55.5 minutes
P90, and 127 minutes maximum. The primary benchmark therefore uses 215 chart
episodes from before Fitbit begins and retains the eight overlap episodes only
for calibration. Its 232 chart rows produce a 234-status ledger: 215 confirmed
sleep entries and 19 explicitly checked rows without a new episode. Conversion
reported all 234 rows, wrote 215 observations, and silently skipped none.

The combined import used 738 Fitbit observations plus those 215 non-overlapping
chart observations. On a fresh ignored database, the source previews reported
738 and 215 ready with zero invalid rows. Their commits appended all 953
observations atomically; repeat previews reported 738 and 215 exact duplicates
with zero ready or invalid rows. The measured conversion used one owner-selected
zone; no travel-zone inference was applied.

The combined backtest used 952 principal episodes; the remaining observation
was the reported Fitbit nap. With the seven-episode minimum, all 945 eligible
holdouts were accounted for as 809 evaluations plus 136 typed refusals
(coverage 0.856). Every refusal was `ambiguous_cycle_index`.

| Candidate | Scale | Evaluations | Refusals | Coverage | Median error | Mean error | P90 error | Hit rate | Mean window |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| Baseline | 1.00 | 809 | 136 | 0.856 | 1.71 h | 2.40 h | 5.41 h | 0.78 | 14.71 h |
| Tighten-75 | 0.75 | 809 | 136 | 0.856 | 1.71 h | 2.40 h | 5.41 h | 0.72 | 13.31 h |
| Tighten-50 | 0.50 | 809 | 136 | 0.856 | 1.71 h | 2.40 h | 5.41 h | 0.66 | 11.91 h |

Baseline confidence calibration:

| Confidence | Evaluations | Hit rate | Median error |
|---|---:|---:|---:|
| High | 28 | 0.61 | 1.23 h |
| Medium | 386 | 0.81 | 1.42 h |
| Low | 395 | 0.77 | 2.19 h |

Measured decision: keep the production uncertainty scale at 1.00. Tighten-75
reduced mean width by 1.40h but lost 6 percentage points of hit rate;
Tighten-50 reduced width by 2.80h but lost 12 points. Point error did not
improve. The chart-inclusive baseline also improves on the digital-only run's
1.77h median, 2.50h mean, and 5.65h P90 errors while preserving its 0.78 hit
rate. A sensitivity run that allowed the eight lower-precision chart overlaps
to merge into Fitbit changed only median error, from 1.71h to 1.70h at displayed
precision; the decision is unchanged. The low-versus-medium error delta and
the high bucket's lower hit rate justify testing an explicit confidence-window
calibration or misfit candidate next, but no such signal ships without a
positive delta against this combined baseline.

## Visual verification

- Reviewed unavailable and populated Rhythm Context states at 1440x900 and
  390x844 in Paper, Dark, and High contrast. The ruled entry/ledger layout stays
  within the page, all four marker kinds remain shape-distinct, exact date/zone
  matches are duplicated across both actogram plots, the forecast remains off by
  default, and the screen-reader table reports the same context. The export
  control discloses that owner export includes private notes. Typed `DELETE`
  enables the permanent-erasure action only after the explicit copy distinguishes
  physical deletion from observation suppression. Runtime console review
  reported no warnings or errors; the temporary contract-shaped preview was
  removed and appearance and viewport overrides were reset.
- Reviewed the real Medications workspace with unavailable-service and populated
  bridge states at 1440x900 and 390x844. Definition setup, compact quick log,
  observed/predicted/unavailable timing context, correction, exclusion, typed
  hard-erasure controls, the user-authored schedule editor, reminder disclosure,
  neutral feasibility counts, and DST gap copy remained legible and contained.
  At 1440x900 the 1425px document had no off-viewport elements; at 390x844 the
  document remained within its 375px layout viewport while the 720px occurrence
  table scrolled only inside its named 315px region. Runtime console review
  reported no warnings or errors, and the viewport override was reset.
- Reviewed the real-calendar replacement at 1440x900 in the browser fixture:
  source administration remains a compact rail, forecast ranges remain
  background bands, fixed events use rectangular overlap lanes, exact event
  details open in the inspector, and the document has no horizontal overflow.
- Repeated Calendar at 390x844. The primary board precedes source
  administration, date controls collapse to two compact rows, the 620px civil
  board scrolls inside its column, and the document remains within the 375px
  layout viewport. Runtime console review reported no warnings or errors.

- Manually reviewed Overview and Rhythm at 1440x900 in Paper, Dark, Pitch black,
  Amber, and High contrast, with reduced stimulation both off and on. Appearance
  was restored to Auto with reduced stimulation off after the matrix.
- Reviewed the U-E time probes on Overview, actogram, drift, and Calendar at
  1440x900. The actogram second plot resolved to the following civil day,
  forecast positions stayed explicitly predicted, drift reported observed and
  fitted onsets, and edge chips remained inside their tracks.
- Repeated probe and containment checks with a 390x844 viewport override in
  High contrast + reduced stimulation. Overview and Calendar did not widen the
  page; the actogram retained internal horizontal scrolling while the document
  stayed at viewport width; the drift screen-reader table remained semantic
  without contributing its intrinsic column width to page overflow.
- Pointer movement, chart scrolling, and route changes left at most one probe
  visible. Runtime console review reported no warnings or errors. Paper, Amber,
  and High contrast probe chips were visually reviewed; theme contrast tests
  cover all five presets. Appearance and viewport overrides were reset.
- Overview measured approximately 693px high (77% of the viewport) with one
  primary surface and no nested generic panels or metric cards.
- Rhythm's actogram measured approximately 568px high and stayed within the
  desktop viewport.
- Overview had no page-level horizontal overflow at 900x900 or 390x844. The
  navigation rail scrolls internally at narrow widths as designed.
- The 390x844 Rhythm pass exposed chart overflow propagating to the page. The
  final CSS paint-contains that overflow at `.actogram-panel` while preserving
  horizontal scrolling in `.actogram-chart`; hidden screen-reader tables use
  fixed layout so added columns cannot widen the page. The UI standards check
  guards these rules and the readable 760px double-plot width.
- Reviewed the U-F Data Sources and Sharing remediation at the native 1280x720
  viewport and 390x844 in Paper, system Dark, Amber, and High contrast. Data
  Sources leads with a two-column provenance ledger and fits source status plus
  both input paths above the native fold. Sharing reports that no link exists,
  marks all relationship rows `Example only`, and uses no avatar or profile
  card. Both routes use one ruled workspace, contain zero generic `.panel`
  descendants, and reported zero page-level horizontal overflow. At 390 px the
  ledgers collapse to label/value rows; the eight route icons remain reachable
  while their internal scrollbar is visually suppressed. An exact 1440x900
  metric pass also found no overflowing main-content descendants. Appearance
  was restored to Auto and the viewport override was reset.
- Installed the rebuilt debug APK on `Pixel_10_Pro_XL_API_36_1` and reviewed
  Status, Correct, Medication, and Settings in portrait. The generic section
  `Panel` is absent; rules and headings separate content without reducing the
  44/48 dp control targets. Rotated the same AVD to landscape for the large-width
  Settings check. Accessibility bounds proved the content column was 2040
  physical pixels (680 dp at 480 dpi) after correcting the modifier order;
  before that fix it incorrectly filled the 2992 px screen. Portrait was
  restored. Logcat contained no app crash or ANR, and no text or control clipped.
- Reviewed U-G's shared proposal surfaces at 1440x900 and 390x844 in system
  Dark, Paper, and High contrast. Approvals rows measured 144/143 px at desktop
  width and 220/217 px at narrow width; the first Tasks proposal measured 299
  px instead of the prior 431 px. The narrow appearance picker measured 347 px
  instead of 821 px. Both widths retained zero page-level horizontal overflow,
  every icon-only route link retained an accessible name, and the browser
  console reported no warnings or errors. Auto appearance and the browser
  viewport were restored after review.
- Reinstalled the current debug APK after U-G and repeated the four portrait
  routes plus Settings in landscape on `Pixel_10_Pro_XL_API_36_1`. The app
  process remained live, portrait rotation was restored, and logcat contained
  only emulator/graphics compatibility warnings rather than an app exception,
  crash, or ANR.

## Previously verified artifacts

Verified on Windows 11 on 2026-06-15 and 2026-06-16:

- Pinned local Node.js and Wails setup, Go modules, Java/Gradle detection, npm
  installation, and deterministic fixture generation.
- Native Wails production build at `apps/desktop/build/bin/ZeitBoard.exe` and a
  hidden Windows launch health check.
- Android debug APK at
  `apps/android/app/build/outputs/apk/debug/app-debug.apk`.
- Desktop overview and trusted-view browser screenshots in `docs/screenshots/`.

## Desktop-local agent endpoint (ADR-0028)

Verified on Windows 11 on 2026-07-26, after the Phase 4 review:

- Full suites green: gofmt clean; `core` 10, `apps/desktop` 4, `apps/server` 7
  package groups pass `go vet` and `go test`; 29 contract fixtures verified and
  validated; 222 desktop frontend tests; TypeScript typecheck; ESLint plus the
  UI-standards check; 33 installer library tests.
- **Endpoint gates** (`internal/localagent`): non-loopback callers, absent or
  invalid bearer tokens, and requests carrying an `Origin` header - including a
  present-but-empty one - are refused. `Header.Get` cannot distinguish an empty
  header from an absent one, so the check tests for presence.
- **Descriptor ACL**: a Windows-only test asserts the published descriptor has
  a *protected* DACL with exactly one entry, for the current user.
  `os.Chmod(0600)` alone does not achieve this on Windows, which is why the
  explicit DACL exists.
- **Medical safety**: the post-provider screen is reachable. A benign prompt
  whose model answer smuggles a dosing directive is refused, while an ordinary
  scheduling answer using "you should"/"take the 3 PM slot" is not. Decision
  questions about an unknown medication name ("how much Hetlioz should I
  take?") refuse via the phrasing rule; ordinary planning ("when should I take
  my lunch break?") does not.
- **Publish transaction**: a half-published install (pending marker present, or
  a declared component missing) fails validation closed; completing clears the
  marker only after every declared artifact validates. A test asserts the
  transaction is actually invoked by `install.ps1` and `update.ps1`.

Not verified here: the end-to-end voice path through a real MCP client, which
needs a running desktop app and a GUI session. `scripts/smoke-local-mcp.ps1`
covers it manually and was confirmed to fail closed when the bridge is absent.

## Architecture and performance hardening

Verified on Windows 11 on 2026-07-27 against the disposition ledger in
[`architecture-performance-review-2026-07-27.md`](architecture-performance-review-2026-07-27.md):

- `go test` and `go vet` pass for every core, server, and desktop package. The
  isolated tools module also passes both commands. The desktop application,
  local MCP companion, server MCP companion, and server daemon compile.
- Contract validation passes, and fixture regeneration verifies the exact set
  of 29 synthetic files from the shared manifest.
- Desktop frontend: 38 files / 243 tests pass. Trusted-view prototype: 2 files /
  6 tests pass. Both TypeScript checks, ESLint, the 19-screen UI architecture
  guard, formatting, and production Vite builds pass. Bundle output contains
  separate route, assistant, and clinician-report chunks.
- Android JVM tests were forced to rerun. `lintDebug` and `assembleDebug` pass.
  `connectedDebugAndroidTest --rerun-tasks` ran 4 tests successfully on the
  requested `Pixel_10_Pro_XL_API_36_1` API 36 AVD.
- The freshly installed APK was visually checked on Status, Correct sleep, and
  Medication event after navigation transitions settled. The screens retain
  the ruled desktop-aligned hierarchy, compact fields/actions, minimum touch
  targets, and no generic bubble-panel treatment or clipping.
- Installer policy, transaction, rollback, no-op update, ACL marker, and static
  wiring coverage reports 45 passed and 0 failed. All modified PowerShell files
  also pass the language parser.
- Focused proposal store/API tests cover active-first stable pagination,
  high-water exclusion of concurrent inserts, opaque snapshot cursor
  round-trips/tamper rejection, joined one-use nonces, and cross-device
  decisions.
- One-iteration benchmarks on the Ryzen 5 6600U verify linearized backtesting:
  100 episodes 1.6 ms, 1,000 episodes 42.6 ms, 10,000 episodes 164.3 ms, and
  20,000 episodes 291.8 ms. These are local diagnostic measurements, not
  release latency guarantees.
- `git diff --check` is clean. `git fsck --full` reports no missing or corrupt
  objects; its dangling blobs are ordinary unreachable edit objects from the
  interrupted work session.

The connected suite validates Android persistence and process-recreation
behavior but does not seed real Health Connect provider records. Paging,
source-offset, duplicate-revision, permission-loss, DST gap/overlap, and
last-good-snapshot behavior are covered by adapter/repository tests.

## Availability portal foundation (ADR-0029)

Verified 2026-07-31 for slice P5-a. The portal is not exposed: every check
below runs against a loopback test server or an in-process handler.

**Projection firewall.** The canary suite plants distinctive values in six
private fields — device label, observation ID, source record ID, task title,
correction ID, and the owner's private share label — then drives the portal
with real materialized data derived from twelve principal sleep episodes. No
canary appears in any public response body, in any response header, or in the
portal database file bytes, including the write-ahead log. The database check
reads raw file bytes rather than querying, so an unexpected column would still
fail it. The materializer's output is asserted separately: it carries no canary
and no `confidence`, `explanation`, or `estimateId` field.

**Structural boundary.** A test parses the `portal` package's own imports and
fails if it ever imports `store`, `readmodel`, `assistant`, `api`,
`portalbridge`, `provider`, `estimation`, or `domain`. The boundary is
therefore enforced by the dependency graph, not by review.

**Public DTO.** The availability response carries exactly `version`, `windows`,
`generatedAt`, `horizonEnd`, `status`, and each window exactly `startAt`,
`endAt`, `zoneId`. Both the presence of every required key and the absence of
any other are asserted.

**Honesty budget.** Every rendered state — empty, refused, insufficient,
available, and stale — carries the measured uncertainty qualifier and the
not-medical notice. A refusal's rendered text is asserted free of "sleep",
"episode", "record", "refus", "ambiguous", "cycle", and "estimator", so a typed
estimator refusal never reaches a visitor as a reason. Windows render in civil
time with the zone stated, and no RFC3339 instant appears in a visible label.
The freshness ladder is exercised at 30 minutes (fresh), 7 hours (stale, shown
with a caution), and 25 hours (withheld, no current-state claim, no windows).
The JSON path applies the same 24-hour cut, so a JSON consumer cannot present
an "awake now" the page would have withheld.

**Enumeration and revocation.** Unknown, expired, and revoked links produce
byte-identical 410 responses. Revoking a profile ends existing sessions
immediately and deletes the materialized snapshot. A session issued for one
link returns 401 on another.

**Passcode and CSRF.** A wrong passcode returns 401 and arms an exponential
per profile-and-source backoff; the immediately following attempt is throttled
with `Retry-After` even when the passcode is correct, and succeeds once the
backoff elapses. There is no global lockout. The Fetch-Metadata matrix covers
nine cases: `Sec-Fetch-Site: same-origin` succeeds with no Origin, with a
matching Origin, and with `Origin: null` — the shape a real browser sends under
`Referrer-Policy: no-referrer`; `cross-site`, `same-site`, and `none` are
refused even with a spoofed matching Origin; a matching Origin alone succeeds
as the pre-Fetch-Metadata fallback; and neither header, or a mismatched Origin
alongside same-origin metadata, is refused.

**Headers and page content.** Every response — page, availability, stylesheet,
and generic failure — carries `default-src 'none'`, `frame-ancestors 'none'`,
`style-src 'self'`, `script-src 'self'`, `connect-src 'self'`, `img-src 'self'`,
`Cache-Control: no-store, max-age=0`, `Referrer-Policy: no-referrer`,
`nosniff`, and a noindex robots tag. The rendered page is asserted to contain
no `<script`, no `<style`, no inline `style="` attribute, and no absolute URL,
so nothing in it can reach a third party or violate its own CSP. The session
cookie is asserted `Secure`, `HttpOnly`, `SameSite=Strict`, `Path=/`, with no
Domain attribute, and its expiry never outlives link expiry.

**Maintenance.** The hourly sweep the daemon runs is exercised directly: a live
session survives it, an expired one does not, and access rows older than the
30-day retention window are deleted while recent rows remain. The synchronizer
CSRF token minted with every session round-trips and rejects wrong values; P5-a
has no session-authenticated mutation to spend it on, and the test exists so it
does not rot before P5-b needs it.

**Limits and audit.** The read limit admits 120 requests per source per hour,
refuses the 121st with `Retry-After`, and restores the budget when the window
rolls over. `RecordAccess` rejects any event outside the closed enum. Audit-key
rotation changes the identifier for the same address and deletes rows past the
retention window. IPv6 addresses inside one /64 collapse to a single identifier
while a different /64 does not, so per-source limits cannot be evaded by
address rotation within a prefix.

**Off by default.** With the portal disabled, `mountPortal` leaves `/p/`
entirely to the private handler — there is no portal route to probe — and the
owner's four sharing routes return 404. A config that never mentions the portal
loads with it disabled; an enabled portal without `publicOrigin` fails to load;
non-loopback `http` origins, origins carrying a path, query, fragment, or
credentials, and non-http schemes are all rejected.

**Owner surface.** Create, list, revoke, and erase round-trip. The issued link
resolves once and stops resolving after revocation. The private label is
readable through the owner API, is absent from the portal store's own profile
listing, and is gone after erase. Weak passcodes and lifetimes past the 90-day
cap are refused. All four routes require device authentication.

**Browser verification.** The rendered page was driven in a real browser across
five states (awake now, not awake, stale, out of date, refused). Two defects
were found and fixed that the Go suite could not see: an `Origin`-only CSRF
gate refused every genuine login, because `Referrer-Policy: no-referrer` makes
browsers send `Origin: null` on same-origin form posts; and the state card's
`background` shorthand reset the card's own background colour, letting the page
canvas show through. A CSS-generated "now" badge was also replaced with real
markup so assistive technology announces it.

Measured suite results: `internal/portal` and `internal/portalbridge` pass, as
do `api`, `config`, `daemon`, `store`, `readmodel`, `sync`, `assistant`, and
`mcp`. `gofmt` and `go vet` are clean across the server module.

Not yet verified, and gating exposure: an independent security review
(exposure-gate item 6), acceptance suites run on a Linux server build, and the
operator TLS/reverse-proxy runbook with log-token redaction (item 3). Requests,
messaging, and the live SSE layer are P5-b through P5-d and do not exist.

## Visitor time requests (ADR-0030)

Verified 2026-07-31 for slice P5-b, all against loopback servers and
in-process handlers. The portal remains unexposed.

**Round trip.** A request submitted through the public form is stored, reaches
the owner's queue as a proposal with origin `visitor`, is decided with a
one-use token, and the decision appears on the visitor's status page with the
exact block the owner chose. The proposal payload carries the visitor's handle
and message, because judging a request requires them.

**Honest queued state.** With the bridge unable to file the request — no
enrolled device — the pump reports failure, the request stays `queued`, and
the visitor is shown "saved and on its way", never "sent". Once a device
exists the same request goes through without resubmission.

**Idempotency.** Four consecutive pumps over one request produce exactly one
proposal, which is the failure that actually occurs: the private commit
succeeds and the acknowledgement is lost. `ApplyDecision` is idempotent in the
other direction too — three replays of the same approval are a no-op, and a
later contradicting decision does not overwrite a settled one.

**Exact-slot rule.** Approval is refused with a typed error for a block before
the window, after it, straddling its end, of the wrong duration when one was
requested, empty, or inverted; a block inside the window with the exact
requested length is accepted. Over the API the same cases are 400, and the
approved block reaches the visitor unchanged.

**One-use tokens and route guards.** A replayed decision token is refused on
the visitor route. The generic `/v1/proposals/{id}/decision` returns 409 for a
visitor proposal, so the slot and delivery obligations cannot be skipped by
choosing the other endpoint. `DecideVisitorProposal` refuses a non-visitor
proposal. `place_visitor_request` is absent from the assistant action
registry, so an agent cannot mint a proposal that appears to come from an
outside person.

**Requester isolation.** A visitor holding the shared link and passcode but no
requester cookie sees the recovery-code form, not another visitor's request,
and none of that request's text appears in the response. A wrong secret and an
unknown request id produce identical status and body. Exchanging the correct
secret yields a request-scoped cookie that renders the author's own request.

**Validation.** Rejected with typed errors: end before start, a window already
started, a window over eight hours, durations below 15 or above 480 minutes,
a duration longer than its window, an unknown zone, and handle or message past
their rune caps. Control characters are stripped and newlines and tabs folded
without mangling the text. A window past the forecast horizon is accepted,
flagged, and rendered with the explicit infeasibility warning — owner decision
3, no product cap on how far ahead someone may ask.

**Caps and revocation.** The per-session daily cap admits five requests and
explains the sixth refusal. Revoking a link closes its open requests rather
than deleting them, so a visitor is not left watching a status that never
moves. A declined request's rendered text is asserted free of "asleep",
"sleep", "busy", "calendar", "conflict", "medication", and "because".

**Browser verification.** The whole visitor path was driven in a real browser:
passcode gate, dashboard, request form, submission, the one-time recovery
code, and the status page. The requester secret appears only after the `#` in
the continue link, never in the path or query. The "now" badge, moved from a
CSS pseudo-element into markup during P5-a, is confirmed present in the
accessibility tree.

**A P5-a defect this slice exposed.** ADR-0029 stored only a hash of the
synchronizer token, which a server-rendered form can never embed — the
mechanism could not have worked. CSRF tokens are now derived from the session
value under a server-held key, so the plaintext is recoverable on every render
while staying unguessable. The test asserts derivability, verification, and
rejection of the session value itself.

Measured suite results: `portal`, `portalbridge`, `api`, `config`, `daemon`,
`store`, `readmodel`, `sync`, `assistant`, and `mcp` pass; `gofmt` and
`go vet` are clean; Linux and Windows server builds succeed.

Not implemented, and therefore not verified: messaging threads (P5-c), the SSE
live layer and audit UI (P5-d), and the desktop dialog for choosing a block on
the calendar — an owner decides through the API today. The exposure gate is
unchanged and unmet.

## Environment limitations

- Emulator semantics and screenshots were reviewed, but a full manual TalkBack
  traversal with spoken-output verification was not performed. Periodic real
  TalkBack and desktop screen-reader participant walkthroughs remain roadmap
  work; automated roles and minimum target sizes do not substitute for them.
- The installed Go toolchain has CGO disabled, so `go test -race` is unavailable.
