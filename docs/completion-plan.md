# ZeitBoard completion plan

Status: active completion target, established 2026-09-08.

Owner clarification, 2026-09-08: there are no deployed users or packaged releases.
Backward compatibility with prototype contracts and development database versions
is not an acceptance requirement. Maintain one current contract per capability,
qualify the first packaged baseline and simplify duplicated paths (ADR-0039).

## Outcome

Finish ZeitBoard as the owner's usable, connected daily planner: a Windows desktop app and Android
companion, a self-hosted backend, BYOK assistant, sleep/rhythm and medication records, an
approval-controlled calendar, and a live, passcode-protected availability portal with requests,
messaging and notifications. An ordinary day must not depend on manually exporting wearable data or
leaving a particular screen open.

This is the current completion sequence. `roadmap.md`, the feature specs and ADRs supply detailed
requirements; older phase prompts are historical where their descriptions contradict delivered code
or later owner decisions. The [September review](product-quality-review-2026-09-08.md) records the
current UX repairs and recovered owner direction. Preserve that uncommitted work. The
[code quality review](code-quality-review-2026-10-16.md) records what the code's quality rests on,
and where that evidence stops.

## Review conclusions that change the plan

- Android initially had enrollment and durable sleep push beneath an incomplete UI. The C1 increment
  below adds reachable enrollment and automatic execution; a subsequent increment adds pull,
  erasure, real cached Go estimates, tasks and phone-authored sleep corrections. Medication sync
  and complete recovery acceptance remain. Calling the companion either a non-syncing skeleton or
  complete is wrong.
- P7 delivered its foundations; a negative inference validation result is an accepted result, not
  permission to promote inferred sleep. Its operational acceptance and private pilot remain
  unfinished.
- Sharing link management, the portal foundation and visitor requests exist. Live updates, request
  threads, notifications, owner audit/preview polish and the exposure review remain. The portal is
  no longer just the static demo.
- Reaching hours, U-H navigation, direct agent appearance actions and local file protection have
  shipped. Do not rebuild these from stale descriptions. ADR-0035 explicitly chooses OS file
  protection over local database encryption.
- Local medication M-A through M-C exists and now syncs between the owner's computers (C3). The
  companion's medication sync and complete propose-only agent coverage do not exist yet; generic
  statements that the assistant or disease-management phase is complete conceal those gaps.
- The original installer plan still has clean-machine, update/rollback, service lifecycle and
  signing checks that automated builds do not prove.

## Completion milestones

Each milestone needs implemented behavior and recorded evidence. A passing unit suite alone does not
satisfy its end-to-end acceptance. Begin with C1; collect pilot observations as soon as it works,
while completing independent software milestones. Do not wait for a multiweek pilot to write the
remaining software, and do not report the pilot as finished before its data exists.

| ID  | Deliverable                                       | Acceptance                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| --- | ------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| C0  | Preserve and reconcile the working baseline       | The September changes remain intact; current requirements map to code/tests and have an explicit implemented, partial, remaining or intentionally deferred status. Resolve old contradictory prompts instead of applying them literally.                                                                                                                                                                                                                                                                                                                                                                              |
| C1  | Reliable automatic evidence and companion loop    | Android imports permitted Health Connect records and syncs in background using durable, bounded retry; pull applies corrections, revisions and erasure without resurrection. The companion reads real cached/server estimates and task state, clearly separating sample/offline/error modes. Reboot, permission loss, process death, network loss and re-enrollment have working recovery. Travel offsets are represented truthfully or visibly held, never guessed. Desktop collection/recomputation survives hiding, sleep/resume and configured startup; quitting is explicitly represented as collection stopped. |
| C2  | Complete planning and approval flow               | One coherent queue/count across local, backend and visitor work; origins, expiry, conflicts, failed decisions and history remain visible. Batch review preserves each proposal's evidence and authority. Documented task constraints are usable and persist correctly. Calendar shows pending visitor requests and allows the same exact-block approve/decline flow as Approvals. Supported calendar refresh and an explicitly enabled CalDAV write-back path handle only approved app-owned blocks, with revision/conflict checks, idempotent retry and safe undo; imported fixed events cannot be silently moved.   |
| C3  | Complete connected medication and context records | Versioned medication definitions, immutable events, corrections and relevant context markers sync across supported clients with revocation, export and tombstone erasure. User-authored schedules retain civil-zone semantics. Reminder ownership/deduplication and missed delivery are explicit; no dose or medication timing is inferred. Existing clinical reports remain correct after remote edits.                                                                                                                                                                                                              |
| C4  | Complete assistant and agent capability coverage  | One versioned action registry backs validation, MCP definitions, dispatch and presentation. Every supported user workflow has reviewed structured readable state and appropriate allowlisted proposals, including dose logging and task/calendar operations. Private labels/notes do not enter provider or MCP output. Appearance retains its direct reversible exception; agents cannot approve their own proposals. Real local and configured-provider smoke tests cover facts, refusal, reconnect, pending decisions and the documented external voice-client path.                                                |
| C5  | Complete the live coordination portal             | Required-passcode links, exact recipient preview, freshness, SSE with polling/no-script fallback, coarse audit, request-scoped encrypted threads, retention/erasure and bounded abuse controls work together. A visitor can request any representable future date with a beyond-forecast warning, exchange permitted messages, receive the owner's decision, and lose access immediately on revocation. Public handlers never receive private-store access or health data.                                                                                                                                            |
| C6  | Deliver coordination notifications                | Request, decision and message events reach explicitly enrolled receivers while the desktop window is closed. Self-hosted configuration, permissions, retry, deduplication, expiry and delivery limitations are documented. Notification text carries no private health/calendar/message content. Disclose any browser/OS transport intermediary rather than claiming that self-hosting removes it. The optional M-F signal requires its own default-off, revocable grant and safe projection; absence of a dose log must not be represented as proof a dose was missed.                                               |
| C7  | Finish daily-use UX and clinical outputs          | First-run setup leads to a real source and usable estimate or honest refusal. Logging, correction, task edit/placement, request review, report export and recovery are navigable without developer knowledge. Complete the documented clinical chart/export/template requirements, keyboard and reasonable nonvisual equivalents, compact Android layouts, supported themes and fatigue-state affordances. Keep the five primary destinations and visual-first chart design. No enabled dead controls or synthetic fallback in personal-data mode.                                                                    |
| C8  | Qualify and package the release                   | Reproducible desktop/server/Android builds, migrations, installer/update/rollback, backup/restore and revocation/erasure checks pass on supported environments. Resolve material architecture/performance findings in the affected paths, including bounded history and registry duplication; avoid cosmetic rewrites. Record independent portal review, private pilot results, remaining limitations, release artifacts and accurate user/operator runbooks.                                                                                                                                                         |

Dependency order: C0 → C1 → C2; C3/C4 build on the stable sync and approval boundaries; C5 resumes
after C1's freshness/recovery behavior is demonstrated; C6 follows the event and permission
contracts from C3/C5. C7 applies throughout. C8 gathers evidence throughout and closes last. The
previous portal pause is a sequencing rule, not cancellation of the owner's requested live portal.

## Scope boundaries

Completion includes the accepted capabilities above, not every research idea in the original
specifications. The following remain explicitly outside this release unless the owner changes the
scope:

- A new estimator family, physiological/DLMO estimation, automatic medical interpretation, treatment
  advice, interaction checking, or driving/fitness judgments. Keep the existing measured baseline
  and honest uncertainty.
- Promoting activity-inferred sleep merely to increase passive coverage. The measured-delta gate
  remains; a negative result leaves inference shadow-only.
- General health metrics, a medical content library, multi-patient management, a project-operated
  hosted service, or proprietary speech infrastructure.
- Every possible calendar/wearable/cloud-history provider. Finish the existing Health Connect,
  file-import and CalDAV paths first; additional providers are extensions, not substitutes for
  completing those paths.
- Unreviewed health disclosure or unconditional automatic schedule changes. Optional features are
  implemented off by default with their own grants; “optional” does not mean secretly enabled or
  omitted without disposition.

The ordinary trusted-availability projection remains medication-free. M-F, if enabled by an owner,
must be a separately designed minimal signal permission, not an expansion of the default
availability DTO. (Delivered 2026-10-11 as exactly that, ADR-0058: a per-link grant, off by default,
whose snapshot field is absent from every other link.) Any new private-data boundary needs its versioned contract, ADR, redaction tests
and corresponding UI consent.

## What counts as finished

**Software complete:** C0–C7 are implemented and end-to-end tests pass using synthetic/disposable
data; builds, contracts, migrations, recovery and installer regressions pass; there are no
unresolved critical/high defects or medium-severity defects that break a core workflow, lose
records, expose data or apply an unapproved change. Remaining lower-severity limitations have a
specific disposition. A rejected research candidate is not a missing feature.

**Operationally qualified:** the required supported-device and clean-machine checks are recorded,
the existing private pilot framework is completed, and the portal exposure gate in
`portal-design.md` §12 is satisfied. Report passive coverage and manual burden, sync latency,
stale-output incidents, forecast error/coverage/width by horizon, task acceptance/completion, and
CPU/memory/ battery/storage cost. Measure against the recorded baseline; do not manufacture
historical resource measurements. Explain any unmet target and fix or qualify the associated
capability rather than suppressing refusal or uncertainty.

The overall goal is complete only when both milestones are satisfied and a release disposition is
recorded. Do not relabel “ready for a pilot” as project completion. Public availability and
notification delivery must not be claimed operational merely because mocks or a local daemon worked.

## External dependencies and continuation

Some acceptance evidence needs owner hardware, configured services/credentials, a signing identity,
elapsed real-world use or an independent reviewer. Prepare the implementation, disposable test
environment, exact test procedure and review materials first. Record the specific missing dependency
and continue other work; do not use one blocked hardware test to halt software development. Request
only the actual missing input when its dependent step is ready.

This goal authorizes development and validation, not publishing private data, sending messages to
real recipients, enabling public exposure, or replacing the owner's installation without the
applicable authorization. Release artifacts and deployment changes should be concrete and reviewable
before that final step.

## Execution record

Keep milestone status, test evidence and remaining dependencies here and in `verification.md`,
updating the existing roadmap/specification map instead of creating a new plan for every slice.
Record completed increments and preserve all prior work. Prefer fixes that close a usable workflow;
avoid expanding the stack, rewriting proven modules or substituting documentation for
implementation.

Initial state: C0 documentation reconciliation prepared; the September code baseline has 403 passing
web tests and verified Go/Android/contracts/native build checks. C1–C8 remain open to the extent
described above. No new device pilot, external deployment, independent review or signing run is
claimed.

### C1 increment — 2026-09-08

Implemented the reachable Android enrollment/upload/disconnect flow, separate default-off background
consent, Health Connect background feature/permission checks, a shared application container and
bounded durable WorkManager jobs. Fixed failed-server-switch queue loss, unstable source revision
tracking, nanosecond truncation, missing/invalid upload acknowledgments, redirect handling,
UI-thread network access and fabricated successful upload times. Schema 4 preserves earlier SQLite
records while binding queues to enrollment generations.

Provider revisions now retain imported provenance through the Go correction contract. New
source/user conflicts refuse rather than silently overriding a user correction. New corrections
cannot restore timestamps for an erased server observation. Kotlin and Go check the same generated
wire fixture. Settings and privacy text explicitly describe upload-only scope and remaining
limitations. See [ADR-0037](decisions/0037-android-automatic-upload-and-source-provenance.md).

Verified Android unit, lint, APK, SQLite migration/reopen and disposable Android 16 emulator paths,
including actual HTTP enrollment/upload/revision/restart against a disposable Go API. The Go
core/desktop/server suite, affected erasure regressions and contract validation pass. Evidence is
recorded in `verification.md`; existing September desktop improvements remain intact.

**C1 remains open.** The next increment below implements pull, task state and cached estimates.
Complete the source-correction review handoff and desktop hidden/resume/startup checks. Continue
C2–C8 thereafter; operational wearable/background/battery/TLS/pilot acceptance has not been supplied
by synthetic emulator tests. No goal-completion, deployment, signing or real-recipient action is
claimed.

### C1 companion increment — 2026-09-08

Implemented authenticated v2 companion forecasts from an atomic Go server snapshot, explicit
expiry/refusal/synthetic provenance, paginated Android pull, atomic cursor/cache invalidation,
latest task revisions and effective sleep. My data mode can now serve a phone without Health Connect
permission. Tasks is a reachable read-only destination; Status labels cached output and expires its
current assessment while open. Enrollment/disconnection text describes downloads.

Schema 5 preserves previous local evidence and adds replica/erasure metadata. Server erasure now
removes dependent correction payloads, including pre-migration records. Android erasure removes
imported versions, their local corrections, outbox and derived cache and suppresses later Health
Connect re-imports even after re-enrollment. Source reconciliation handles a newer phone observation
against an existing server original and refuses conflicting equal revisions. New provider payloads
are independent of intervening revisions seen by a device. Manual overlays retain unedited provider
endpoints and their freshness times.

Server rollback produces a re-enrollment instruction. After a complete new pull, missing locally
accepted uploads are reconciled without reviving erased sources. Bounded WorkManager/foreground
execution now drives the full exchange. The Go suite, 32 contract fixtures, Android unit/lint/build
checks and 16 disposable Android 16 instrumentation tests pass, including real HTTP
forecast/task/revision downloads and remote erasure. See `verification.md` and
[ADR-0038](decisions/0038-android-companion-cache-and-erasure.md).

**Next software work:** complete Android manual-correction sync/review and consolidate its
upload-only test paths into the production full exchange, then exercise desktop
hidden/resume/startup/recovery. Prototype compatibility is explicitly unnecessary under
the owner's clarification and ADR-0039. Task writes and
medication remain in the following milestones. C1–C8 and the original completion goal stay open; no
real wearable/OEM battery/TLS/private pilot or independent portal qualification is inferred from
these synthetic checks.

### Pre-release simplification — 2026-09-08

The owner confirmed that there are no deployed users or packaged releases.
Removed the newly introduced dual sync routes/versions and conditional exports;
correction provenance remains in the single current contract. Removed unused
medication v1 schemas and fixtures, Android development-version migrations and
server prototype backfill helpers. Current-schema creation and explicit refusal
of obsolete development profiles replace historical migration support.

Desktop task and sleep uploads now share one typed batching, acknowledgment and
durable commit loop. Removed legacy provider-author inference and parent-link
reconciliation; the current producer identifies provider changes explicitly.
Current-record idempotency, erasure, resume/restore, permission and refusal checks
remain required. See [ADR-0039](decisions/0039-pre-release-contracts-and-streamlining.md).

Verified the current Go suite, 29 maintained contract fixtures and 15 native
Android tests. The reduced fixture/test counts remove obsolete compatibility
cases; current-schema creation/reopen, record integrity and actual HTTP sync are
still exercised. Existing desktop review improvements remain intact. Continue
manual-correction sync and lifecycle qualification under the full completion goal.

### C1 connected correction review — 2026-09-08

Android now has one full sync execution path with mandatory replica storage.
Removed upload-only methods, nullable-replica branches, prototype enrollment-scope
fallbacks and redundant single-parent correction storage. Current corrections
identify the source revision and all manual heads reviewed; concurrent edits or
unseen provider changes withhold forecasts until explicitly resolved.

Connected Android Correct selects a downloaded source, shows competing changes,
preserves exact timestamps, and queues a complete resolution with classification
and exclusion. The desktop log remains editable during conflicts and rejects stale
forms. Cached Android review supports offline saves; restart retains the queue;
erasure clears cached and open review context. See ADR-0040.

Verified all Go tests and vet, 403 web tests plus lint/typecheck/build, 30 current
contract fixtures, 92 Android unit tests plus lint/APK builds, and 16 disposable
Android 16 instrumentation tests. Actual HTTP checks cover manual save/reopen,
concurrent-edit resolution and erasure. The native form was exercised through
server enrollment, source selection and correction save. No production service,
owner installation, real-device pilot or release artifact was published.

**Next:** consolidate local-only Android correction production and its later
enrollment handoff with the current reviewed-record contract, then complete desktop
hidden/resume/startup checks. Connected correction editing is delivered; complete
phone-authored record synchronization and C1–C8 remain open. Do not add compatibility
paths for prototype schemas or treat this increment as completion of the goal.

### C1 pre-enrollment correction handoff — 2026-09-08

Saved Android sleep corrections now join enrollment through the same current
manual-record encoder and durable exchange. Their immutable source versions and
reviewed parent IDs preserve what was actually seen, including sources outside the
recent provider snapshot. A newer source or unseen remote edit produces a review
conflict instead of being overwritten. Enrollment explicitly includes this saved
evidence; fixtures and medication remain excluded.

Bounded pages progress past held sources, with holds reconsidered after a home-zone
change. Missing source dependencies can be requeued after restore. Erasure removes
local correction payloads as well as queued/downloaded copies. Local forms retain
their reviewed snapshot and exact instants; a durable append sequence preserves the
latest edit across clock regression and SQLite compaction. See ADR-0041.

Verified 92 Android unit tests, lint/APK builds and 21 disposable Android 16 tests,
including an actual pre-enrollment save/reopen/HTTP handoff and explicit resolution
of unseen remote changes. Native fixtures now erase remote records between cases;
the estimator's refusal across ambiguous historical gaps is retained. PR #28's
published baseline passed all GitHub CI jobs, including installer dry-runs.

This audit finding is addressed by the next increment below. The full C1–C8
completion goal remains active.

### C1 desktop consent and background sync — 2026-09-08

Desktop activity now has persisted default-off consent, an explicit zone, live
status, durable atomic transition batches and separate export/erasure controls.
Restart respects consent and revocation; clean stop records a bounded final
shutdown. Steady unlocked use produces no repeated transitions. Poll-gap records
carry inferred provenance and unknown confidence. Activity remains local and does
not enter estimation or planning. See ADR-0042.

The enrolled desktop runs the existing bounded sleep/task sync pipeline from its
app context, at startup/enrollment and about every minute, independent of views.
Foreground and background exchanges serialize; disable cancels an active exchange
and stale errors cannot revive old settings. Credentials/settings publish
atomically and HTTP redirects cannot forward enrollment secrets. Quit joins work
before closing storage. A dedicated absolute data-directory override isolates
qualification profiles.

Removed remaining desktop development migration/backfill code and the
legacy-duplicate import trigger; current schema creation, source uniqueness,
reopen, revisions, explicit erasure and external import behavior remain tested.

**Next:** implement configured login/start-hidden behavior and background estimate
projection refresh; finish desktop enrollment/restore reconciliation and perform
native hide/resume/restart/quit qualification. In particular, review tray-failure
close behavior before claiming the desktop lifecycle is complete. Comprehensive
agent coverage for these controls belongs to C4. Real-device/pilot and the other
C1–C8 acceptance gates remain open; this software increment does not close the goal.


### C1 desktop login and window lifecycle — 2026-09-08

Settings now has explicit Windows login registration, an optional start-in-tray
mode, manual hide and explicit quit. Registration never grants activity or sync
consent. Wails uses one process per data profile; another manual launch reopens
its existing window. Close hides only with a working tray, while explicit quit
stops workers. A failed tray on startup or during Explorer recovery reveals the
window. Startup/shutdown serialize service setup and teardown. See ADR-0043.

The installer and app share the current quoted executable / `--background`
command forms. Installer startup previously promised tray launch without passing
any flag; that mismatch is fixed. Current startup registration is the authority,
with no saved preference that can silently recreate an OS-disabled entry.

Verified lifecycle and registry adapter tests, focused race checks, component
consent/recovery/quit tests, and all 45 installer tests. Registry tests use an
isolated non-Run key, with no owner login registration or Explorer restart.
The previous published increment passed all eight GitHub CI jobs.

**Next C1 implementation:** `localEstimate` still reads effective sleep sessions
and runs `RobustEstimator` on foreground requests. Connect background projection
refresh to actual readers, with source fingerprint/erasure invalidation, refusal
states and `freshness.NextChange` expiry. Preserve the existing planning snapshot
checks; an unconsumed periodic computation is not completion. Then finish desktop
sync enrollment/restore reconciliation and native login/hide/suspend/resume/quit
qualification. All later C1–C8 software and operational acceptance remains active.

### C1 desktop background analysis — 2026-09-08

Desktop readers now consume the background worker's SQLite estimate snapshot.
The app owns startup, expiry, evidence-change and cold-read refresh, using the
shared `core/recompute` worker formerly owned by the server. There is one current
pipeline for both consumers. Empty/refused states, source and content fingerprints,
minute validity and earlier freshness boundaries prevent obsolete cache use.
Content-change timestamps remain stable during unchanged housekeeping.

Source changes invalidate derived state transactionally. Erasure removes the
snapshot and journal, including a computation racing deletion; publication
rechecks evidence before persisting. Startup recovery serializes with foreground
work, and quit cancels/joins both kinds of calculation before storage closes.
Native update events refresh existing evidence-dependent views without carrying
private payloads. Existing correction-review refusals and planning snapshot guards
remain in place. See [ADR-0044](decisions/0044-desktop-background-analysis.md).

Verified all Go core/desktop/server tests and vet; affected worker, SQLite,
desktop and server race suites; 405 desktop plus six trusted-web tests and the
canonical web checks; and a Windows Wails production build with an isolated
profile. Tests demonstrate background freshness expiry without any open view,
foreground consumption without rerunning the estimator, reopen/recovery,
clock regression, erased-source rejection and timer/backoff behavior. The prior
published startup increment passed all eight GitHub CI jobs.

**Next C1 software:** finish desktop enrollment/restore reconciliation. Native
login/hide/suspend/resume/quit, supported-device and pilot evidence remain open.
The C8 history/performance audit still applies to source folding/fingerprinting;
estimator caching does not prove bounded resource use over years of records.
The full C1–C8 completion goal remains active.

### C1 desktop enrollment and restore reconciliation — 2026-09-21

Enrollment now commits credentials, settings and sync progress in the same local
SQLite transaction. Failed server switches retain the prior connection; stale
status writes cannot revive disabled enrollment. New enrollment downloads from
zero before reconciling acknowledgments and uploading missing retained records.
Desktop pull now includes its own device's records and drains bounded pages.
The API detects a cursor ahead of restored history; both clients direct the owner
to re-enroll. See [ADR-0045](decisions/0045-desktop-enrollment-and-restore-reconciliation.md).

Durable suppression prevents erased IDs and task revisions from returning across
replay/re-enrollment. Deletes account for uploads whose acknowledgments were lost.
Corrections arriving before their sources are saved and applied on a later page;
Settings shows waiting corrections and pending deletions. Immutable mismatches and
unsent task conflicts retain the local record instead of silently acknowledging
or overwriting it. The previous separate development config/token path is removed.

Stateful TLS and actual disposable-daemon tests demonstrate same-device restore,
failed/successful server switches, correction exchange, erasure and re-enrollment.
The full Go suite, vet, affected race suites, web checks and Android checks are
recorded in `verification.md`. PR #28's prior background-analysis increment passed
all eight GitHub CI jobs.

**Next:** continue C1's native lifecycle qualification where the environment can
provide actual evidence, then complete C2's coherent approval queue, task conflict
review and supported calendar write-back. Do not stall independent C2–C8 software
on hardware/pilot evidence. Native login/suspend/resume, supported-device resource
measurements, private pilot, independent portal review and release acceptance
remain open. The goal remains active.


### C2 unified review queue increment — 2026-09-21

Home, Plan, navigation and the assistant Approvals link now share one pending count
across local proposals, assistant/agent proposals and visitor requests. Server
counts cover unloaded pages; scoped pagination prevents mixed work from hiding
requests. Real visitor response decoding, terminal pagination, expired-token
visibility and persistent decision errors are corrected. Approvals has functional
source filters and history. Calendar and Approvals share request drafts and the
same exact-block decision flow, including explicit repeated-hour choices. A
committed decision survives a failed follow-up list read. See
[ADR-0046](decisions/0046-unified-review-queue-and-exact-request-times.md).

The review also found and fixed a C1 regression: contextual planning reads no
longer overwrite or reschedule live background analysis. Pre-release cleanup removes
old route aliases and permissive list adapters instead of maintaining two contracts.

Verification: full Go tests/vet, focused race suites, 410 web tests (404 desktop,
6 trusted prototype), canonical web checks/builds and Windows Wails build. Targeted
checks cover pagination/counts with 105 intervening assistant proposals, scoped
cursor rejection, used/expired tokens, exact expiry boundaries, current TLS response
shapes, acknowledgment followed by refresh failure, shared drafts, duplicate
submissions, loading remounts and DST gaps/folds. Browser inspection used synthetic
fixtures and verified shared count/history, the request form, a repeated-hour choice
preserved on Calendar, and a decision reducing the count.

**Next C2 work:** reviewed batch semantics, task conflict resolution, remaining
constraint affordances and supported calendar write-back. Native lifecycle, actual
wearable/device conditions, production TLS, independent portal review, clean-machine
installation/signing and the private pilot remain unqualified. The full C0–C8 goal
remains active; this is a partial C2 implementation, not project completion.

### C2 task conflicts and a plan that does not overlap itself — 2026-09-24

Continues the C2 increment above, which stopped mid-way when the Codex session reached its usage
limit. The conflict retention and review code it left uncommitted is finished and verified; using
the app against a synthetic data directory then found two planner defects that made the "reviewed
batch semantics" in the C2 acceptance unsafe to build.

**Task conflicts.** A downloaded task revision that conflicts with an unsent local edit is retained
instead of aborting the sync page; only that task is held from upload and placement while it awaits
review. Approvals shows a comparison card, and choosing a version creates a new revision and records
the resolution in history. Two defects were fixed before committing: none of the new TypeScript met
the repository's formatting check, so CI's web job would have failed; and each radio was wrapped in
a label containing the whole detail list, which left Chromium with no usable accessible name
("local", and nothing) — a failure jsdom did not reproduce. The card now shows only the fields that
differ, with the rest stated once, because eight rows of mostly identical values buried the one or
two a person has to choose between.

**The planner suggested overlapping and already-started blocks.** With three open tasks it suggested
all three for the same minute, because each task was placed as if it were the only one; accepting
them all would have triple-booked that time. It also suggested, and accepted, a block that began
eleven minutes earlier: the planning snapshot is pinned to the start of a 30-minute bucket so a
proposal keeps its identity while it is read, and suggestions were allowed to start at the beginning
of that bucket rather than its end. Suggestions now start when the bucket ends, each reserves its
time for the next (`scheduling.Request. Reserved`, busy but never reported as "avoids a fixed
event"), tasks are placed earliest-deadline-first so an open-ended task cannot take the only time
before another task's deadline, and approving a block that has begun is refused. Home's outlook
suggestions use the same reservation. Each regression test was checked to fail without its fix.

**Development environment.** `wails dev` could not start: the watcher ran `npm --prefix frontend`
from inside the frontend directory, and Vite was pinned to 34115, the port `wails dev` needs for
browser access to the Go bindings. Both are fixed, so the real app can be driven from a browser
against a disposable data directory, which is how the defects above were found.

**Next C2 work:** reviewed batch semantics can now be built on a plan that does not overlap itself;
supported calendar write-back and the remaining constraint affordances remain. Operational
qualification is unchanged from the record above.

### C7 streamlining increment — 2026-09-24

A pass over every screen for the C7 requirement that logging, correction, task
placement, request review, report export and recovery be navigable without
developer knowledge. It is recorded in `ui-refactor-plan.md` §13 and
`verification.md`.

The routine acts now lead each page: two-tap sleep logging, one-tap doses, and
a task's suggested time decided directly under the task. Approvals is part of
Plan › Tasks. Calendars moved to Data Sources and Settings split into
addressable tabs. Relative times ("Tonight 5:30 – 5:50 PM") replace full stamps
where a glance is the use. Per-row confidence buckets are gone, since ADR-0022
measured them inverted. Five primary destinations and the visual-first charts are
unchanged.

Using the app found and fixed four defects outside the UI layer:

- outlook events without titles;
- a first night without an onset band;
- a calendar blank for the waking stretch under way;
- a placements calendar dated 1969 to 9999.

**Next C7 work:** first-run setup to a real source, the clinical chart, export
and template requirements, keyboard and nonvisual equivalents for the new
layouts, and compact Android layouts.

### C7 almanac redesign — 2026-09-25

The visual layer was replaced rather than adjusted (`ui-refactor-plan.md` §14).
There is one paper-and-ink design system with a serif for reading and no cards,
pills or side stripes, laid out three ways:

- the desktop Home as an almanac page led by a sentence;
- Plan › Week as a calendar-native board for arranging a plan;
- the Android home screen as a 24-hour dial.

The UI lint now enforces the no-stripe, no-pill rules. Home's diary reads the
calendar, so events stay listed while the forecast is withheld. The Week board
says when it draws a stale estimate.

**Next C7 work:** unchanged from the streamlining increment. The Android
companion also needs accepted times in its contract before the dial can show
plans.

### C7 first-run setup — 2026-09-25

Home before the first forecast used to say only that there was not enough
recorded sleep. It now counts the way there and goes straight to each way in:

- **Counting.** The estimator reports the usable nights it found and how many it
  needs (main sleeps of three hours or more; seven). The lead sentence and the
  page's figure count them.
- **Ways in.** Four entries, each linking to where it is done: record from
  tonight with the buttons beside the sentence, add past nights
  (`#/log/sleep/add` opens the form), import a file, or sync from the phone.
- **Honest refusal.** When the records exist but give no forecast for another
  reason (a gap too long to count cycles across, a pattern outside the range
  the estimator was checked on, records that disagree), the page says so in
  plain words, quotes the estimator, and offers the one step likely to help.

In the running app, an empty profile went from "0 of 7" through three nights
added with the past-night form ("3 of 7") to a forecast at seven.

**Next C7 work:** the clinical chart, export and template requirements,
keyboard and nonvisual equivalents for the new layouts, and compact Android
layouts.

### C7 clinical double plot — 2026-09-25

The clinician report's chart can now be drawn as the 48-hour double plot that
ADR-0027 reserved (see its addendum): each row is a day and then the next, so
drift reads as a continuous slope. The report form offers it as "A day and the
next". Repeated halves are marked, so the text alternatives and summary counts
describe each night once.

Two defects surfaced on the way and are fixed in both orientations:

- the axis labels were a fifth of a day out of place, sitting in equal grid
  columns instead of over the hours they name;
- a segment a few seconds long after a row boundary rounded to zero width, and
  the preview rejected the whole report over it.

A blank sleep log now prints from Data Sources, beside the CSV template: one
row a day from 6 PM to 6 PM, matching the chart, for nights kept on paper.

**Next C7 work:** keyboard and nonvisual equivalents for the new layouts, and
compact Android layouts. Direct PDF/PNG output remains a refinement; printing
the exported HTML is the PDF path.

### C7 the Week board without sight — 2026-09-25

The Week board drew sleep only in paint. A screen reader heard the blocks
without their day and the sleep bands without their times, and a short band
said nothing at all. Each day is now a group named by its date ("Friday,
September 25, today"). Every recorded or forecast band says what it is and
when ("Likely asleep, 7:45 AM to 2:00 PM"), as do doses and the point past
which nothing is forecast. The drawing is unchanged. Keyboard use already
worked, since every block is a button under the app's focus ring; the Android
dial already carries a spoken description beside its sentence.

**Next C7 work:** compact Android layouts, and a screen-reader pass of the
new layouts on real assistive technology.

### C7 Android at night — 2026-09-25

Android now follows the system into Ink, the desktop's dark theme: the page,
the dial's rings and hand, the fields and the tabs change together, and the
window and system bars switch with them, so a dark launch has no light flash.
The colour names the screens use (Ink, Paper, Muted and the rest) now read the
theme's palette, so no screen had to change. A test holds both palettes to
WCAG contrast: 4.5:1 for text and 3:1 for the dial's marks.

At the narrowest common width (320dp) every tab fitted except the tab bar's
own "Settings", which broke across two lines in its fifth of the width; each
tab now has room in proportion to its word and never wraps.

### C7 print the clinician report — 2026-09-26

The clinician report prints straight from the app: after the same typed
EXPORT confirmation, "Print or save as PDF" opens the system print dialog,
where Save as PDF is a destination, and "Save HTML file" keeps the standalone
file. The page is laid out in a frame that may not run scripts and leaves
once printing is done.

Checking it in the running app found a defect behind it: the desktop
announced "analysis updated" every minute, since each result is valid for a
minute, even when nothing had changed. Every view reloaded, and the report
marked itself stale and disabled its export about a minute after it was
built. Only a changed result is announced now.

**Next C7 work:** a screen-reader pass on real assistive technology and the
Android dial's plans, which first need accepted times to reach the phone.

### C1/C7 plans on the Android dial — 2026-09-26

Accepted times reach the phone (ADR-0047, the owner's choice of three). An
accepted suggestion is an app-owned block on the desktop; it now also travels
as an immutable `placement` record naming its task and interval. A trigger on
the block's deletion queues the record's erasure, so undo, deleting the task
and erasing all data each remove it from the server. The server validates and
relays it; the companion keeps it with its downloaded tasks and draws the
accepted times of open tasks as a thin ink ring inside the forecast, for the
next 24 hours, with a "Plans" key and the plans in the dial's spoken
description. Imported calendar events still stay on the desktop.

**Next:** a screen-reader pass on real assistive technology (NVDA or Narrator
on the desktop, TalkBack on the phone), and the remaining C2 to C8 milestones.

### C5 review and the portal in the Almanac — 2026-09-26

**Where the portal stands.** P5-a (share links with required passcodes, the
security middleware, the allowlisted materializer, stale and unavailable
rules) and P5-b (visitor time requests, request codes, the outbox bridge and
exact-slot approval) are delivered and green. P5-c (request threads) and P5-d
(the live layer, persisted rate limits, the audit view, the reverse-proxy
profile) have been paused since 2026-08-04, waiting on a freshness policy for
the owner's own current-state claim. That policy now exists, so the pause
condition is met. Of the resumed sequence in `portal-design.md` section 11,
two steps are done: the desktop Sharing screen creates, revokes and erases
real links, and the materializer withholds availability by the shared
freshness policy (P7-1). The exact recipient preview section 10 asks the
Sharing screen for is still missing, and the independent review has not
happened. The exposure gate in section 12 is not passed and the portal stays
disabled by default.

**The page.** It was still in the style the owner rejected. It is now set in
the Almanac design (see `ui-refactor-plan.md`): a sentence lead, a figure of
the next three days, the windows in words, a solid button and the qualifier on
a sheet, in Paper by day and Ink at night, with the bundled faces served from
the portal's own origin. The static `trusted-web-prototype` still shows the
old demo and no longer describes the portal.

### C7 consistency pass — 2026-09-26

The owner's completion goal adds that the UX must be consistent and simple to
navigate. Walking every screen of the real app with synthetic data found
inconsistent decision and deletion vocabulary, six different permanent-delete
confirmations, loading states that claimed data was missing or unreadable, a
drift chart whose labels stacked and misaligned with real data, and sample-only
prototype panels in Rhythm › Sources. Each is fixed, with one shared hook for
loading and one component for deletion replacing the per-screen copies (see
`ui-refactor-plan.md`). The prototype `phaseTwo` module is retired: its types
live with the data they describe and its sample data with the other fixtures.

### C5 exact recipient preview, and a page that said "not awake" all day — 2026-09-26

The Sharing screen now previews each link as its recipient sees it (portal-design
section 10): the instance renders the link's snapshot with the portal's template,
records no visit, and the desktop shows the allowlist-rebuilt markup in a shadow
root. Driving it end to end against a local instance with synthetic nights found
that the portal said "Likely not awake right now" for the whole of every waking
day: the estimator forecasts waking windows only after each predicted sleep, so
the day in progress was never a window. The materializer now publishes it, from
the observed wake to the earliest likely next sleep, and a regression test pins
that a visitor during the owner's waking day reads "Likely awake right now".
Next in C5: the live layer (SSE with polling and no-script fallback), the coarse
audit view, persisted rate limits and request-scoped threads.

### C5 live layer — 2026-09-26

The availability page now stays current while it is open (P5-d's live
layer): an authenticated event stream says only which version is current and
how fresh it is, the page's own script re-reads itself through the normal
route and replaces its content in place, and it also refreshes at the exact
instant its claim next changes. Refused or dropped streams fall back to a
one-minute poll; the no-script page keeps its five-minute reload. Streams are
bounded per session, per link and globally, end on revocation and on
shutdown, and the reverse-proxy guidance covers them. The Sharing screen's
access audit also names every event the instance records — its labels had
drifted from the server's event names, so an owner would have seen codes such
as `page_view`. Remaining in C5: request-scoped threads (P5-c) with their
14-day deletion job, and the independent review.

### C5 request threads — 2026-09-26

P5-c is implemented: a visitor who asked for a time and the owner can exchange
short plain-text messages about that request. Threads are sealed in the portal
store with their thread and author bound in, need the link's messages grant
(offered only with requests), close when the request is answered, are deleted
two weeks later by the maintenance job or at once by the owner, and are bounded
per thread and per link. The owner reads and answers them on the request's card
in Approvals, with the shared deletion confirmation. With the live layer and the
recipient preview, C5's software is complete; its acceptance still needs the
independent review in `portal-design.md` section 12 and C6's notification of new
requests and messages while the desktop is closed.

### C6 notices about time requests — 2026-09-26

The owner is told, while no desktop window is open, when someone asks for a
time, writes about a request, or a request is answered elsewhere. The server
appends a content-free event (kind and request id only) in the same
transactions that create the visitor proposal and decide it, and through the
portal outbox for a visitor's message; `GET /v1/notifications` serves it to
enrolled devices with a head cursor for a device just turning notices on. The
Android companion reads it every 15 minutes while notices are on (off by
default, with Android's notification permission), words each notice itself,
hides it on the lock screen and retires it when the request is answered.
There is no vendor push service in the path, so there is no intermediary to
disclose; the delivery limits (15-minute minimum, battery deferral,
force-stop) are documented in `self-hosting.md`. The optional M-F missed-dose
signal is outside this feed: since 2026-10-11 (ADR-0058) it is a link grant
whose times are seen on the page, not pushed. Remaining for C6's acceptance: the
recorded device run (a portal request raising a notice on an enrolled phone
with the desktop closed), recorded on 2026-09-27 below.

### C3 medication and marker sync, server and desktop — 2026-09-26

Medication definitions, doses, dose corrections and rhythm context markers now
travel between the owner's computers through their own server (ADR-0048). The
server accepts four new record kinds, validated by the same code the desktop
store uses (now in `core/medication` and `core/markers`); erasing a definition
erases every revision, dose and correction of it and refuses later uploads for
it from a device that was offline. The desktop uploads definitions before doses
before corrections and keeps the highest revision of a definition. An edit made
while apart is rebased field by field, so a rename on one computer and a new
schedule on another both survive. A dose that arrives before its definition
waits and applies when the definition arrives, two computers' corrections of
one dose both apply in creation order, and deleting a definition, dose or
marker erases it on every device. A sync that downloads or erases anything now
refreshes every open view. Remaining for C3: the Android companion (accept these
kinds, log doses against synced definitions, handle erasure) and its device run.

### C4 one assistant snapshot — 2026-09-27

Everything an assistant may know about the owner's day now comes from one versioned, redacted
document (ADR-0049, `contracts/v1/assistant-snapshot.schema.json`), built from the same
computations Home draws. It covers:

- whether the estimate may describe now, and what the owner is doing;
- when sleep and waking are likely;
- the fitted rhythm and the fit rating's caveat;
- recent sleep as Home states it;
- the next three days: timeline, reachable hours, commitments with their conflicts, accepted
  times, suggestions awaiting a decision, and unplaced tasks;
- what needs the owner;
- tasks, medication timing, markers and sync state.

The local agent endpoint serves it as `get_snapshot`, listed first. Before this, the chat model
saw only tasks, sleep windows and busy intervals. It now also receives the planning view, which
it reads as enums, ids and civil times only; it still never sees a sleep record or health context.

The "sleep likely begins / waking likely" rule moved from the web code into `core/outlook`, and
Home's lead now reads its result. The screen and the assistant state the same ranges from one
computation.

Along the way:

- with sync on, Home's overview now carries this computer's freshness verdict. The server's
  overview has none, so Home used to read every synced estimate as "cannot confirm how recent"
  and never said what was happening now;
- the outlook's zone fallback no longer yields the non-zone "Local";
- the sync settings copy stopped saying medication stays on this computer, which C3 made
  untrue;
- agent confidence levels are lower-case everywhere.

Remaining in C4: the versioned action registry, propose-only dose logging, and the agent smoke
tests.

### C3 the companion reads medication records — 2026-09-27

Since the desktop half landed, desktops upload medication definitions, doses, dose corrections and
context markers. The companion refused every record kind it did not know, so an owner with medication
data would have had a companion that could no longer pull. ADR-0048 says clients must accept these
kinds before any device uploads them; merging the desktop half first broke that order, and this
restores it.

The companion now:

- checks each kind exactly as its contract defines it; a record it cannot read fails the page rather
  than being kept half-understood;
- stores definition revisions as it stores tasks (the highest revision is current), and keeps doses,
  corrections and markers against their targets;
- applies their tombstones: an erased revision takes the medication with its doses and corrections.

It does not show or log them yet. The Doses screen against synced medications is next, followed by
the recorded device run.

### C3 doses on the phone — 2026-09-27

The companion's Doses screen now lists the medications from the owner's computer, each with its
usual time and latest dose, and a Taken and a Skipped button, as the design sketches them. A tap
records the dose now in the home zone as an immutable `medication_event`. It shows as uploading
until the next sync sends it, and the desktop's history and clinician report pick it up. It is
corrected or deleted on the computer, and deleting the medication there also removes a tap still
waiting to upload.

The latest dose is the effective one: downloaded doses are taken after their corrections, applied
in creation order as the desktop applies them. Free-text events for anything not on the list stay
on the phone.

With this, C3's software is complete. The device run is recorded in `verification.md`: a dose tapped
on an enrolled phone appeared in the real desktop's history and clinician report.

### C4 one registry of agent actions — 2026-09-27

What an agent may ask ZeitBoard to do is now listed once, in `core/agentactions` (ADR-0050).
Each action records its kind, the surfaces that offer it, its tool wording and its card title.
The same package holds the task target's schema and rules. The chat assistant's schema prompt and
validation, both MCP tool lists and budgets, the local endpoint's dispatch and the proposal cards
all read the registry. A test holds the three contracts' action enums to it.

The copies had already drifted. The local endpoint accepted an empty timing bound the server
refused, and the two MCP endpoints described the same tool differently. Both now follow the
registry.

Remaining in C4: propose-only dose logging (the first action added through the registry), and the
agent smoke tests.

### C4 an agent can propose a dose — 2026-09-27

An agent on the owner's computer can now relay "I took my evening tablet" (ADR-0051). The local
endpoint's `propose_log_dose` names a medication by its opaque id, taken or skipped, and
optionally when. It is the first action added through the registry.

The dose waits on the computer, never synced or exported, and lapses after a day. It appears
first under "Waiting on you" on Home and in Plan's decision queue, counted with everything else
waiting on the owner, under the same Accept and Decline. Accepting records the dose a hand-logged
one would be, never marked scheduled, so it syncs and reaches the history and the clinician report
like any other.

Proposals now have a subject:

- the server resolves task proposals, and refuses a dose proposal even when it names a task;
- the chat model and the server's connector are never offered dose proposals.

The snapshot counts the doses still waiting.

Found along the way: on Windows the desktop's own zone resolves to the development default
(New York) rather than the computer's zone. Quick logging and proposed doses both record that
zone. It is fixed next.

Remaining in C4: the agent smoke tests.

### Consistency: the desktop names its own time zone — 2026-09-27

On Windows, and on Linux and macOS without `TZ`, Go reports the local zone only as "Local". The
desktop read that as unknown and fell back to New York. On any computer elsewhere, every time the
desktop itself stated or recorded was then under New York's name:

- the rhythm and outlook's clock;
- the snapshot the assistant reads;
- quick "going to sleep" taps;
- proposed doses;
- planner suggestions before an estimate existed;
- the empty-state "now".

The screens' own entries were right, since the browser named the zone. Records from the two
sources disagreed.

`core/platform/localzone` now asks the system. On Windows it asks the ICU that ships with
Windows 10 1903 and later, then falls back to the registry's zone name through the Windows table
(moved from the calendar importer into `core/timezones`). On Linux and macOS it takes `TZ`, then
the zone `/etc/localtime` links to. The desktop looks the zone up once per run, as Go reads the
local clock once per run. When the system names no zone the desktop uses UTC rather than a guessed
place. The tests pin their computer to New York, so they hold wherever they run.

### C4 the voice-client path survives restarts — 2026-09-27

The desktop's stdio bridge is what a voice client such as Claude Desktop runs. It read the
running app's address and token once, and exited on its first failed request. Two ordinary
events therefore ended a voice client's connection until the client itself was restarted:

- restarting ZeitBoard, which moves the endpoint to a new port and token;
- thirty idle minutes, after which the endpoint drops the session.

The bridge now finds the app again. It re-reads the descriptor, opens a new session the way the
client opened the first, and retries once. It retries only a request that certainly never ran
(connection refused, token refused, session gone), so a proposal is never made twice. While the
app is closed it answers "ZeitBoard is not running" and keeps serving. It may also start before
the app does.

Running the smoke for real found that PowerShell starts a child's input with a UTF-8 byte-order
mark, which the bridge could not parse. `scripts/smoke-local-mcp.ps1` had never completed a
session. The bridge now ignores the mark, as any client on a UTF-8 Windows console needs, and the
script writes without one.

The script now covers the documented voice-client path:

- the tool list, with nothing that decides;
- facts, from the snapshot and a question;
- the byte-identical refusal;
- a proposed dose waiting for the owner (`-ProposeDose`);
- a session carrying on across an app restart (`-Reconnect`).

Remaining in C4: the same coverage for the server's connector and the chat assistant with a
configured provider (below).

### C4 the server's connector and a configured model, smoke-tested — 2026-09-27

`scripts/smoke-backend-mcp.ps1` does for the server's connector (self-hosting.md, Option B) what
the local smoke does for the desktop. It runs the connector as a voice client would and checks:

- the tool list, with nothing that decides;
- facts from the server's projections;
- proposals listed without approval tokens;
- optionally, a proposal waiting for the owner (`-Propose`);
- optionally, the chat assistant (`-Assistant`): a dosing question refused without the model, and
  a planning question sent to the configured model;
- optionally, the session carrying on across a server restart (`-Reconnect`).

Building it found:

- **Approval tokens reached the model.** The connector handed each proposal's one-use approval
  token to the model, in propose results and in the proposal list, and a test asserted it. The
  model could not use one without the connector's device token, but approval material has no
  place in an agent's hands (ADR-0012). The connector now removes it; the desktop's endpoint never
  passed it.
- **A configured provider could not be local.** The only way to use a model the operator runs
  was to misuse the OpenCode Zen provider with a dummy key. `openai_compatible` now names any
  server speaking OpenAI's chat completions, such as Ollama or llama.cpp, with an endpoint and an
  optional key. With such a model the assistant's context never leaves the operator's machines.
  Every provider's endpoint must now be HTTPS, or plain HTTP only to the same machine.
- **The connector had the same byte-order-mark gap** as the desktop bridge. It is fixed the same
  way.

With C4's smoke coverage complete, what remains is its wording on "task/calendar operations",
reviewed next.

configured provider.

### C4 an agent can propose a task — 2026-09-27

"Remind me to call the pharmacy, fifteen minutes, before Friday" now reaches ZeitBoard from a
voice client. `propose_add_task` names a one-line title in the owner's words, its length and
optional bounds. Like a proposed dose, it waits on the owner's computer (ADR-0051, extended).
Accepting adds the task the owner would have typed, and the planner suggests a time for it at
once.

Doses and tasks now share one local queue (`local_agent_proposals`), one card, one Home entry and
one history row. A dose's approval records the event, and a task's adds the task, each in the
same transaction as the decision. The registry names three subjects: schedule changes (resolved
by the server), doses and new tasks (waiting on the desktop). Its test holds the desktop-only
subjects to the local endpoint.

Completing, editing or deleting a task stays the owner's. An agent cannot name a task by its
title, and these are not voice-first acts. With this, every C4 criterion is met:

- the action registry;
- structured state and proposals for the supported workflows, dose logging and task operations
  included;
- no private labels or notes in provider or MCP output;
- the appearance exception;
- no self-approval;
- smoke tests of the local and server paths.

### C6 the device run: a request reaches a phone while the desktop is closed — 2026-09-27

C6's last acceptance item is recorded (`verification.md`, "A time request reaches a phone").

The setup:

- the real `zeitboardd` with its portal, on a disposable data directory;
- the owner's desktop enrolled with it and used to create a share link, then closed;
- the Android companion enrolled with time-request notices on;
- a visitor who opened the link, entered the passcode and asked for a time.

On its regular 15-minute check the phone raised the notice. It carries no one's name, note or
requested window, it is private on the lock screen, and it is tagged to the request so that an
answer can retire it.

The run also found a compact-layout defect on the phone. The key column of its key–value rows was
a fixed 68 dp, narrower than the words "PERMISSION" and "CORRECTIONS", which broke mid-word
("PERMISSIO / N"). The column is now sized in sp, so it fits those words and grows with the font
size; longer keys still wrap between words.

With this, C6's acceptance is met.

### C2 suggested times are reviewed and decided together — 2026-09-27

Plan's decision queue can review every planner suggestion at once (ADR-0052). "Review N
suggestions together" lists each with its title, time and reasons. The owner leaves out any they
want to decide alone, then accepts or declines the rest in one step.

This was not possible before, and not for want of a button. Each suggestion's evidence includes
the calendar it was planned against. Accepting one changed that calendar, so every other
suggestion from the same plan went stale and was re-planned; only the refresh after each decision
hid it. The batch is one transaction. Every decision is checked against the evidence it was
planned on before any is recorded, and if anything changed since the review, none is. Each
decision keeps its own window, reasons and hashes, and each stays undoable on its own.

The feature specification's "approve all low-risk" batch would have trusted confidence buckets
that ADR-0022 found inverted, so it is not built. Visitor requests, task conflicts, assistant
proposals, and the doses and tasks an agent proposes keep their own decision paths.

Also reviewed for C2: task constraints. The editor offers the constraints the task contract
documents, except minimum confidence, which the 2026-09-08 review kept as a hidden policy for the
same ADR-0022 reason.

Remaining in C2: the explicitly enabled CalDAV write-back of approved blocks.

### C2 accepted times can be written to the owner's CalDAV calendar — 2026-09-27

Data Sources › Calendars can write accepted times to an imported CalDAV calendar (ADR-0053).
It is off until the owner turns it on for one calendar. The offer says what will be written and
who can see it, then checks the sign-in with a PROPFIND before keeping it: it must open a
calendar that holds events and may add them.

With writing on, every accepted time that has not ended goes to that calendar as ZeitBoard's own
event, and undoing it removes it. Store triggers queue each write, so accepting, deciding together
and undoing need no code of their own. A desktop worker writes at once after a decision, and
every minute otherwise.

The writes are made safe by the server's own checks, not by trust:

- Creating uses If-None-Match, so it cannot overwrite.
- A time already there under its own UID is kept, so a lost answer never writes twice.
- Removing uses If-Match, so an event the owner changed waits for them to choose "Remove it
  anyway" or "Keep it in the calendar".
- A failed write retries after 1, 5 and 30 minutes, then every 2 hours, or at once on "Try again
  now".

Importing or refreshing a calendar now leaves out ZeitBoard's own events, which are already on
the board. Kept, each would be busy time clashing with the placement it copies. An empty CalDAV
calendar, such as one made for ZeitBoard, can now be imported.

The real-app run found two faults that were older than this feature:

- **Error reasons never reached the owner.** Wails rejects a Go error with its bare message.
  Every screen tests for an Error, so each showed its generic fallback, never the reason. The
  bridge now turns the message into an Error, which fixes every screen at once. A Go error is a
  lowercase clause by convention, so the bridge also shows it as a sentence ("Wake time must be
  after sleep start."). With reasons shown, the sync failure in Plan read "backend request
  failed". It now reads "Could not reach ZeitBoard's server."
- **The import message said "1 events, 1 of them busy".** It now reads "1 event, 1 busy."

With this, C2's acceptance is met. A refresh action on each calendar's row, instead of adding the
same calendar again, followed the same day (below).

### C8 sleep history is read a page at a time — 2026-09-27

The architecture review's bounded-history finding (#16) is resolved for sleep. Four screens each
read the whole sleep history, a record for every night the owner has ever logged, although none
needed it:

- **Log** shows fifty nights at a time. It now reads that page from SQLite, newest first, with
  the total. A page past the end, as after deleting the last page's only night, comes back as
  the last page.
- **Week** draws the nights that touch its days. It now reads only those, by their corrected
  times, so a night an edit moved into view is drawn and one moved away is not.
- **Rhythm › Sources** and **Data Sources** show per-source counts. The desktop now counts them
  and sends the counts, with the newest corrected night for the correction inspector.

Deleting a night, or all sleep data, no longer returns the whole history either. Each view
re-reads what it shows when the change is announced. Along the way:

- Data Sources started out "empty", so until the counts arrived it said there were no sleep
  records. It now waits, like its other rows.
- Rhythm's correction inspector showed a night's first change as its latest. A night's history
  lists the newest change first.
- The sync row read "last Last synced Sep 27, 2:51 PM".

Medication history is next: each dose tap still returns every dose ever logged.

### C8 dose history is read a page at a time — 2026-09-27

The medication half of bounded history (review #12) is done, so no surface receives a whole
private history any more. Every medication read and every dose tap used to return every dose ever
logged, each placed against the rhythm. Two things used them:

- **The quick taps** on Home and Medications need only each medication's newest dose, for "Last
  taken today at 8:02 AM". Each medication now carries it. The desktop picks it by instant, so a
  dose logged in another zone with an earlier clock reading still counts as the newer one.
- **The history** shows fifty doses at a time. It now reads its page from the desktop, and again
  whenever medication data changes. The desktop folds the corrections once, then places only
  that page's doses against the rhythm.

The Week board needed neither: it draws schedules. The clinician report has its own bounded read.

### C8 databases from earlier builds upgrade cleanly — 2026-09-27

Software completion needs migration regressions to pass, and the owner's own profile was made by
an earlier build. The stores upgrade by creating whatever is missing (`CREATE ... IF NOT EXISTS`).
That adds new tables and indexes, but silently keeps an old definition if an existing one ever
changes.

So the schemas the stores created at five points were compared with today's, from 8 September
(the build this plan started from) to this morning:

- **Desktop store:** every change was an addition. The two exceptions were the unique indexes
  that allowed one correction chain per dose, which medication sync (ADR-0048) drops explicitly
  because two devices may each correct a dose.
- **Both server stores:** additions only.

That finding is now a standing test in each store:

- **Desktop:** a database written by the 8 September build's own store, with synthetic nights, a
  correction, a task, a medication with a corrected dose, a marker and an imported calendar,
  opens in the current build. Its schema matches a new database's exactly, every record reads as
  it did, and it takes new records.
- **Server:** the same schema check for both of its stores, from the same build.

Changing one column's default without a migration fails the desktop test by name. The first
change that is neither an addition nor a drop will need an ordered migration and a fixture of its
own.

### C8 a server that is off is not waited on again — 2026-09-27

The architecture review's finding #14 hid a daily-use defect, and walking the app found its reach.
With sync on and the server's machine off, several reads waited out the whole ten-second request
timeout before showing what they could, on every visit and every refresh:

- Home's overview;
- Rhythm's actogram;
- the decision queue's synced proposals and visitor requests, which also held back Plan's count;
- the Sharing screen's list of links.

A refused connection fails at once, which is why test runs never showed it; a machine that is off
does not refuse.

Now every request through the desktop's backend client says whether the server answered. No
answer marks it down for two minutes; any answer, even an error status, ends that. Reads that
happen on their own give up at once while it is down, and Home and Rhythm show the local estimate,
built from the same synced records. What the owner does, an older page they ask for, and sync
always try, so the server's return is noticed within a minute.

The same walk found how an unreachable server was worded:

- Plan's queue showed it as a red alert above the owner's own suggestions, the style kept for a
  decision that failed. It is now one calm line: "Could not reach ZeitBoard's server. Anything
  waiting there appears here once it answers."
- The window called it "the synced backend" and "your server" where the desktop said "ZeitBoard's
  server". Everything now says the latter.
- Sharing's list said links would appear "once sync is set up" whenever it could not show them.
  It now says why: sync is off, the portal is off, or the server has not answered.
- The last sync arrived as a phrase ("Last synced Sep 27, 2:51 PM"), which Settings set beside
  its own "Last sync" label and Data Sources reworded. The desktop now sends the instant, and
  both word it as the app words every time: "Today at 2:51 PM", "last synced today at 2:51 PM".

### C2 calendars refresh from their row — 2026-09-27

Refreshing a CalDAV calendar used to mean adding it again, address and all. Each CalDAV calendar's
row now has Refresh:

- **The calendar ZeitBoard writes accepted times to** keeps its sign-in, so Refresh imports it again
  at once and says what came back. ZeitBoard's own events are recognised as usual.
- **Any other calendar** has no kept sign-in, by design. Refresh opens the form titled "Refresh"
  with its address and name filled in and the cursor in the username, so the owner only signs in.
  The password is used once and cleared, as always.

A calendar file is refreshed by importing the updated file again, as before; the same name
replaces it.

### C7 every screen is checked as a screen reader meets it — 2026-10-08

A structural test renders all sixteen screens and tabs. On each, every control must have an
accessible name, no id may repeat, and there must be exactly one top heading. It uses Testing
Library's own role queries, so it adds no dependency. Removing the name from one icon button fails
every screen that shows it. The pass with real assistive technology is still the owner's step.

### A night's last edit is undone in one click — 2026-10-08

Taking back an edit used to mean opening the night and typing its old times in again. A night
with an edit to take back now has **Undo** beside Edit in Log › Sleep.

- **Undo adds a correction and removes none,** like every edit. The new correction supersedes the
  latest one and restores the night as it was before that edit.
- **Undoing again steps back further,** to the night as recorded. The history then reads "Restored
  the night as recorded", and Undo goes away.
- **Undo never redoes.** The earlier state is found by replaying the night's edits from the
  recorded night. An edit that returns the night to its state before the current one counts as a
  step back.
- **"As recorded" is the source's latest version.** If Health Connect revised a time, undoing the
  owner's edit keeps the revision.
- **Undo is offered only on a single line of edits with nothing waiting for review.** A night whose
  edits disagree with its source is still settled by editing it.

### Nights recorded more than once are listed — 2026-10-08

When two records describe one night, the estimator merges them at the middle of their times. Say
Health Connect and the owner's log both have it, or the night was logged twice. That was invisible:
the night counted once, at times neither record gave. Rhythm › Sources now lists these nights under
"Nights recorded more than once".

- **Each night shows every record and what the estimator uses.** For each record it gives the
  source and the times. It also gives the night the estimator uses in their place, and how far
  apart the starts and the ends are.
- **The disagreement is drawn.** Each record's bar sits on its own line, on one scale per night.
  The records are solid and the estimator's night is outlined, so they read apart without colour.
- **Exclude leaves a record out of estimates.** It writes the same correction as Exclude in the log,
  so Undo on that night's row takes it back.
- **The list shows the newest twenty and counts the rest.**
- **Data Sources links to the list** when any night has more than one record. It now also says
  "excluded" where it used to say "hidden".
- **The logic lives in the engine.** `sleepv1.Overlaps` reports what `ResolveOverlaps` merges, so
  the list cannot drift from the estimator. A suppressed record is never merged, and neither is a
  record of another kind.
- **Agents do not see it.** The list carries raw record times, which the agent projections never
  return.

### Sources say what they covered — 2026-10-11

When the newest record aged, the desktop said "The newest sleep record is 3 days old" and nothing
more. The cause and its remedy differ: the watch was not worn, the phone was off, a permission was
taken away, or Health Connect needs an update. ADR-0054 makes each one explicit.

- **The phone reports what each check found.** The outcomes are:
  - read in full over the last 30 days, with how many records it held back for their time zone;
  - permission missing;
  - Health Connect unavailable;
  - read failed.

  It reports again when anything changes, and otherwise every six hours. A report holds no sleep
  time.
- **One definition.** `core/sources` validates a report for the server's push and the desktop's
  store, and reads reports into a status. The status gives the newest outcome, since when that
  outcome has held, and the end of the newest read window.
- **Data Sources has a row for Health Connect on the phone.** It shows one of:
  - "Read today at 9:07 AM. Its newest night ends …";
  - "Permission off" since when, and where to allow it;
  - "Unavailable";
  - "Read failed";
  - "Not reading", when no report has come for twelve hours.

  It also says when the phone reads only while ZeitBoard is open on it.
- **A stale estimate says the likely cause.** Home's lead sentence and Week's warning add one
  sentence after the age. For example: "The phone read Health Connect today at 9:07 AM and found
  no sleep in the last 30 days."
- **An assistant sees the same.** The snapshot's sync section lists each source's outcome and
  check times, and no time of sleep.
- **Deleting all sleep data takes the reports with it,** here and on the server.

### Clients trust the owner's server by its certificate — 2026-10-11

A server with its own authority or a self-signed certificate could only be reached by skipping
verification. The desktop allowed that only for localhost; the MCP connector allowed it for any
address. The daemon also made a new localhost certificate on every start, so nothing could pin it.
ADR-0055 changes all three.

- **The desktop trusts a certificate the owner chooses.** Settings › Sync has a "Server
  certificate" row. The owner chooses a PEM or DER file and sees its name, its expiry and its full
  fingerprint. The desktop trusts it beside the system's roots, for the server only, with
  verification on. Enrolling again keeps it until the owner removes it.
- **The MCP connector takes a certificate file** in `ZEITBOARD_MCP_SERVER_CERTIFICATE_FILE`. It
  now refuses to skip verification for anything but localhost, and says which setting to use
  instead.
- **The daemon keeps its localhost certificate** in its data folder. It reuses it until a month
  before expiry, and logs the path and fingerprint at start, so local clients trust the file.
- **One definition.** `core/tlstrust` reads a certificate, builds the trust and gives the
  fingerprint, for the daemon, the connector and the desktop.
- **The phone trusts it too.** The companion's enrollment form picks a certificate file with the
  system's picker, and shows the same name, expiry and fingerprint. The phone keeps the certificate
  with the enrollment and uses it only for the server.

### The clinical chart saves as an image — 2026-10-11

A clinician who takes an image rather than a printed page can now have one. After the same typed
EXPORT confirmation, "Save chart as PNG" regenerates the report from the service and draws its
chart.

- **The image matches the printed page.** It uses the same colours, the same quarter or double-plot
  rules, the same marks for taken, skipped and started doses and for context, and the same legend.
  It draws at twice the resolution while the image stays within what a canvas can hold.
- **The boundary goes with it.** The report's interpretation boundary and the generated time are
  printed beneath the chart.
- **Long ranges are cut, and the image says so.** A range longer than 1,500 days keeps the newest
  1,500 and states how many it drew.
- **Layout is separate from painting.** The chart is laid out as plain shapes, so its geometry is
  tested without a canvas; painting only replays them.

### Several people to reach, each on their own hours — 2026-10-11

Reaching hours were one schedule with the same hours on every chosen day. They are now a list of up
to five people, each with their own zone and their own hours on each weekday (ADR-0056).

- **Settings edits the list as a whole.** Each person has a name, a zone, a row for each day of the
  week and a switch for Home. A day that opens takes the hours of the open day before it, so a run
  of days with the same hours is one click each. Every day carries a small 24-hour bar on a shared
  axis, so a week reads as a shape, and a closing time typed as 05:00 instead of 17:00 shows as the
  overnight stretch it would be. People can be moved up the list, which is the order of Home's rows.
- **Home draws a row for each person shown**, named for them, with their hours on hover. Someone
  with no hours in the three days keeps an empty row, and the words say so.
- **Switching someone off keeps their hours** without a row on Home. Only removing them drops the
  hours.
- **The old schedule carries over as one person**, with the same hours on each of its days. If it
  was off, the person is off. The list is written before the old file is removed.
- **Names stay on this computer.** The assistant snapshot names people as `contact_01` and so on,
  by their place in the list. The server's assistant passes that ref to the model and still accepts
  a snapshot without one.
- **`core/outlook` takes several schedules**, each with an opaque id that every window carries. It
  knows no names.

### More to set up after the first forecast — 2026-10-11

The first-run setup ends at the first forecast. After it, nothing pointed at the steps that let
Home say more. Home now lists them under the figure, on a note:

- **Four steps, each optional.** Nights from the phone, the people to reach, a calendar, and
  starting with Windows. Each has one sentence and a link to where it is done.
- **Done means done.** The desktop says which steps are done:
  - the phone step, once a night or a report has come from the phone;
  - the people step, once the list has been saved, since the working week a new install starts
    with is a placeholder;
  - the calendar step, once a calendar has been added;
  - the Windows step, once the login entry starts this copy of ZeitBoard.
- **A step the computer cannot take is left out.** Away from Windows there is no login step.
- **It reads as progress, then goes.** A done step keeps its place with a mark instead of its
  number, and the note counts the steps done. Once every step is done the note goes by itself.
  Closed with its ×, it stays closed, and Settings › Display can show it again.

### Times on the reader's clock — 2026-10-11

The desktop wrote every time on a twelve-hour clock, from about seventy fixed layouts in Go and a
few hand-built ones in the window (ADR-0057).

- **The clock follows Windows** by default, from its regional short-time format.
- **Settings › Display can choose instead.** Its three choices each show the same afternoon,
  "3:40 PM" or "15:40", on their own clock. The choice is kept on this computer, and changing it
  redraws the window at once.
- **One formatter per runtime.** `core/civilfmt` writes the time of day of any Go layout on a
  given clock. Core's rhythm projection takes the clock from its caller, and the server keeps the
  twelve-hour clock because it does not know its readers. In the window, every time goes through
  `utils/relativeTime.ts`: ranges, gutters, axes, the chart's hover readout and the printable
  blank log. The hover readout used to be 24-hour regardless.
- **One language for names.** Days, months and day periods are named in one place in the window
  (`utils/appLocale.ts`), so the browser's language no longer mixes with English.
- **The phone already followed its own setting.** Its two fixed layouts now do too.
- **Guards.** A Go test fails on a twelve-hour layout written directly in core or the desktop, and
  the UI lint fails on a time with AM or PM spelled out in the window.
- **Not done.** Translated text, other languages' day and month names, and plural rules need a
  message catalog. The visitor's availability page on the server still writes the twelve-hour clock.

### A share link can show the doses logged as skipped — 2026-10-11

Medisafe's "Medfriend" tells a chosen person about a missed dose. ZeitBoard's equivalent is a grant
on a share link, off unless the owner ticks it (ADR-0058).

- **What the link holder sees.** The page lists the times of doses logged as skipped in the last
  three days, or says there were none. A medication name, form, note, schedule, identifier or count
  of taken doses never appears.
- **Only what was logged as skipped, as it now stands.** The server reads doses through the same
  fold as the desktop, now in `core/medication`. A skip corrected to taken, or excluded, is not
  shown, and a dose with no record never is.
- **Said before it is chosen.** The Sharing screen says what the grant reveals beside the box: that
  the owner takes a medication on a schedule.
- **Seen, not sent.** The page refreshes as doses arrive, and drops each one 72 hours after it.
  Nothing is pushed to the link holder.
- **The boundary moved by one field, for chosen links only.** Doses feed the recompute inputs, so a
  new skip is published, but not the content stamp. The page's "updated" age therefore still
  measures the sleep evidence. The server may now read dose records (never medication
  definitions), and only when some link has the grant.

### Years of rhythm from a Google export — 2026-10-12

Recorded nights begin when someone starts recording, but a drifting rhythm usually shows years
earlier in when they used their phone. ZeitBoard now reads that from a Google Takeout export
(ADR-0059).

- **The person brings the export.** Data Sources takes a Takeout .zip, or a My Activity .json from
  one, through the system's file dialog. ZeitBoard never signs in to Google. An export made as web
  pages is refused with how to make it as JSON.
- **Time alone.** `core/takeout` reads My Activity as a stream and keeps the minute of each entry.
  The decoder sees only that a header is present and the time. Titles, links, products and places
  are passed over as the file is read, and an entry larger than a megabyte is not held whole.
  Folders are recognised by shape, since Takeout translates their names.
- **Shown before it is kept.** The preview counts the entries read, the minutes with activity, the
  span and the nights suggested. "Keep these times" stores only the minutes, with counts of what
  each import read. One action in Data Sources removes them all and compacts the database.
- **In the shadow.** Gaps of three to fourteen hours become suggested nights under ADR-0031's rules,
  with their own source kind. Rhythm › Sources draws them as a double-plotted actogram beside the
  recorded nights. It opens on the last twelve weeks, and the whole export is one press away. Below
  the figure it says how many recorded nights had a suggestion beside them, and how far apart the
  edges typically were. A night nobody recorded counts neither way.
- **Nothing else reads them.** The minutes are in no sync payload, share link, snapshot, agent
  projection, estimate or plan, and a test asserts that keeping them leaves the planning
  fingerprint unchanged.
- **Not done.** Taking a suggested night into the record needs a correction prompt. Other services'
  exports, and exports made as web pages, are not read.

### This computer between runs — 2026-10-12

The activity collector runs inside ZeitBoard, so it saw nothing after Quit, before ZeitBoard
started, or while the computer slept. Nothing showed what its records implied, either (ADR-0060).

- **Windows' own record, by choice.** With desktop activity on, Settings › This computer offers a
  second choice: read when the computer went to sleep, woke, shut down and started. It is off by
  default and says what it reads before it is chosen.
- **Six entries, by time alone.** ZeitBoard asks the System log for six entries by provider and
  number: Kernel-Power 42 and 506, 107 and 507, and Kernel-General 13 and 12. Of each it decodes
  the time and the record number. It reads as far back as Windows keeps them, then at each start
  and every half hour while it runs.
- **Kept with the activity records.** The entries become observed suspend, resume, shutdown and
  start transitions from the existing closed set, kept once each. They are counted, exported and
  erased with the collector's records. Erasing also turns the reading off.
- **What the computer suggests.** Quiet stretches become suggested nights by ADR-0031's rules. A
  logged shutdown counts as quiet until the computer starts, but ZeitBoard quitting does not. A
  wake of fifteen minutes or less with no sign of use does not end a night. Rhythm › Sources now
  offers this computer beside a Google export, with the same figure and agreement.
- **Not done.** Lock and unlock history from Windows' logs, whose event data is undocumented, and
  other operating systems.

### ZeitBoard packaged for Claude — 2026-10-12

ADR-0006 held cloud packaging back until it had its own privacy and threat review. ADR-0061 is
that review, and ships the packages.

- **Made by the desktop.** Settings › This computer › Use ZeitBoard with Claude saves two files:
  - A Claude Desktop extension (`ZeitBoard.mcpb`, MCP Bundle 0.3) holding the installed connector.
    Its manifest's tool list is generated from the registry the endpoint serves, so the two cannot
    drift apart.
  - A skill (`zeitboard-skill.zip`). It says how to use those tools:
    - read the snapshot first;
    - speak in ranges;
    - give ZeitBoard's medical answer word for word;
    - every change is a proposal;
    - treat results as data;
    - repeat no more health detail than a question needs.
- **One skill, kept in core.** `core/agentskill` holds `SKILL.md`. Tests hold it to Claude's
  frontmatter rules and check that it quotes `agentpolicy.MedicalRefusal`. They also check that it
  explains every registered action and names exactly the tools the connector serves.
- **Said before it is saved.** Settings says that what the tools return goes to the model Claude
  uses, in Anthropic's cloud, and that neither file holds a record or a password.
- **The guards stay in ZeitBoard.** Budgets, the refusal and proposals that wait for the owner do
  not depend on the skill.
- **Not done.** ChatGPT, and Claude on the web and phones, take remote MCP servers only. Offering
  one means exposing the owner's server with OAuth, a boundary that needs its own review.

### Calendars by their private address — 2026-10-12

Imported calendars were snapshots: plans could be made over a commitment added later until someone
imported again. ADR-0062 subscribes instead.

- **By the address the provider publishes.** Data Sources › Add a calendar › Private address takes
  a calendar's secret iCal address (`https://` or `webcal://`) from Google, Outlook or iCloud. It
  previews like a file, and "Subscribe" keeps it. No sign-in or OAuth registration is needed.
- **Read again every hour.** While ZeitBoard runs, every subscribed calendar is fetched and replaced
  whole, and "Refresh" on its row does the same at once. A failure keeps the last copy, and the row
  says why and from when the copy is. Open views re-read through a new announcement.
- **The address is a secret.** It is kept in its own table, `local_calendar_subscriptions`, which
  cascades with the calendar. It is shown by host only, and appears in no message, status, sync or
  export. Redirects must stay on HTTPS.
- **The same bounds as a file.** It uses the same parser and recurrence bounds, a 16 MiB response
  cap and a 20 second timeout.
- **Not done.** OAuth, for write access and calendars that publish no address, still waits on
  registrations.

### The local files encrypted by Windows — 2026-10-12

Encryption inside the database waits on a new driver. Windows' own file encryption needs nothing
new, so ADR-0063 offers it.

- **A choice in Settings › Your data, off by default.** Where Windows can encrypt files, "Encrypt
  with Windows" sits under the file list. Before the choice, Settings says what it protects and
  what it does not, and that without a backup of Windows' key the files could be lost. Elsewhere,
  Settings points to device encryption or BitLocker.
- **Applied at start.** Windows encrypts only files nothing has open, so the choice is carried out
  before the store opens. Folders go before their files, and come after them when decrypting.
  The folder is marked as soon as the choice is made. A file that could not be done is reported,
  and the next start tries again.
- **Only ZeitBoard's own choice.** Without a saved choice nothing is touched, so a folder the owner
  encrypted in Explorer stays as it is.
- **Read back.** Each file's state comes from Windows. The readout says the files are encrypted only
  when every one is, and names each encrypted file.
- **Tested without touching this account.** A stand-in plays Windows in the desktop tests. The real
  round trip runs on CI's throwaway Windows account, because encrypting for real makes a key in the
  account running it.

### Google and Microsoft calendars through the owner's registration — 2026-10-12

A calendar without a published address could not come in at all. ADR-0064 reads Google and
Microsoft calendars through OAuth, with an app the owner registers rather than one in
ZeitBoard's name.

- **The owner's own registration.** Data Sources › Add a calendar › Google or Microsoft asks for
  the client ID, and for Google the client secret, with the steps to register beside the form.
  ZeitBoard ships no client of its own.
- **Signed in in the browser.** PKCE and a state check, with the answer on a loopback port opened
  for it alone. The window shows that it waits for the browser, and can cancel. After five
  minutes it stops.
- **Chosen, previewed, kept.** The account lists its calendars, its main one first. Each is
  previewed and brought in as a read-only calendar, then read again every hour with the
  subscribed ones. Its row names the account.
- **The sign-in is a credential.** The refresh token is kept in the owner-protected database and
  never shown, synced, exported or logged; a rotated one replaces it. When the provider ends the
  sign-in, the account says so and "Sign in again" uses the kept registration. Removing the account
  erases it with its calendars, and Google is asked to revoke the grant.
- **Tested against stand-ins.** A synthetic provider plays Google and Microsoft, with a scripted
  browser, in the desktop tests; nothing reaches a real provider.
- **Not done yet.** Writing accepted times to these calendars, which the next decision takes up.

### Accepted times written to Google and Microsoft calendars — 2026-10-12

ADR-0064 read these calendars; ADR-0065 writes the owner's accepted times to one, with
ADR-0053's guarantees for CalDAV.

- **The same offer.** "Write accepted times here" sits on an account calendar's row. "Allow
  writing" signs the account in again to ask, once, for adding events: Google's
  `calendar.events.owned`, Microsoft's `Calendars.ReadWrite`. A permission left unticked, another
  account, or a calendar the owner does not own turns nothing on and says why.
- **ZeitBoard's own events, recognised.** Google events go under an ID derived from ZeitBoard's
  with a private mark, and Microsoft's carry ZeitBoard's UID as transaction ID. Reading the
  calendar back recognises them, so a refresh does not count them as busy time.
- **Preconditions.**
  - A retried create finds the first instead of writing twice.
  - A taken ID is kept only when it carries this time's mark.
  - A removal sends If-Match (Google) or compares the change key (Microsoft), so an event the
    owner changed waits for them.
- **ADR-0053's writer and queue.** Retries, conflicts, the status and stopping are unchanged. An
  account that cannot be signed in to leaves every due write waiting with the reason.
- **Tested against stand-ins.** Synthetic Google and Microsoft calendars honour each promise in the
  provider package. The desktop runs accepting, refreshing, undoing, a change by the owner, a lost
  answer, a refresh that must ask for writing again, an ended sign-in, refusals and removal.

### Calendar accounts after review — 2026-10-15

A code review of ADR-0064's change (PR #82) at high effort raised ten findings. Two were already
fixed by ADR-0065's change: an error starting with a host name no longer capitalises it, and
signing in again refuses a sign-in to another account. The other eight are fixed, each with a test:

- **All-day events are days.** Microsoft's were kept as instants at midnight UTC, so for an owner
  west of UTC a busy day blocked the evening before. Google's took the calendar's zone, not the
  one the calendar is kept in. Both are now dates in the zone the calendar is kept in, as a file's
  are. Microsoft's timed events show in that zone rather than in UTC.
- **A rotated token is never sent again.** A calendar deleted at the provider used to drop the
  account's access token, and the next calendar then refreshed with a copy of the account read
  before the provider rotated its token. Now only a refused token is dropped, and a refresh reads
  the sign-in as it is kept.
- **Consent and cancellation are said plainly.** A sign-in whose consent left reading unticked is
  refused and keeps nothing. A sign-in cancelled or out of time during the token exchange says so,
  not that the provider could not be reached.
- **Smaller fixes.**
  - An account name is bounded the way it is kept: in bytes, not characters.
  - An accepted time imported into Google from ZeitBoard's export is recognised by its UID.
  - A refresh pass cut short by quitting announces nothing.
  - After a failure, the accounts are read again, so an ended sign-in offers "Sign in again".
  - Signing in again waits on that account's row, not in a blank form for a new account.

### Writing to accounts after review — 2026-10-15

A code review of ADR-0065's change (PR #83) at high effort raised ten findings. One was already
fixed in the first review's round: a failed write drops an access token the provider refused.
The others are fixed, each with a test:

- **A time accepted again is written again.** An accepted time's event ID comes from its
  proposal. After an undo, accepting it again reused the ID of the removed event, which Google
  keeps as cancelled. The new acceptance was taken for written while the calendar showed nothing.
  Each acceptance is now written under a name of its own, by the time and the moment it was
  accepted, and ZeitBoard's own removed Google event is put back if met. On Microsoft the
  acceptance's moment also keeps a new transaction from being taken for the removed one's.
- **Microsoft's owner edits are told from reminders.** Graph turned reminders on by default, and
  dismissing one changed the change key, so undoing looked like a conflict. Events are now
  written without reminders on both providers, as CalDAV writes are.
- **Looking before writing.** A Microsoft create looked for an earlier attempt every time, on the
  first page only. It now looks only on a retry, over every page, and Graph's own transaction ID
  covers a first attempt.
- **What a sign-in again asks for.** Microsoft scopes were merged across sign-ins, even from
  another registration, and could claim writing never granted. What a sign-in states is now what
  is kept. Signing in again asks to add events only while the account is written to. A sign-in
  that would take that away from the account written to is refused.
- **Smaller fixes.**
  - Quitting while a pass refreshes the account's access leaves the due writes untouched, not
    failed.
  - An account that already allows writing turns it on without a browser prompt or a Cancel
    that does nothing.
  - A host name keeps its spelling at the start of an error only when it is one: "e.g." is not.
- **Left as a residual.** An acceptance's moment is kept to the second. A time undone and
  accepted again within one second has the same Microsoft transaction ID, and Graph could take
  it for the removed event. ADR-0065 records this. The second review below removed it.

### Writing to accounts after a second review — 2026-10-15

A fresh high-effort review of those fixes (PR #89) raised nine findings. Eight are fixed, each
with a test. Checking them turned up a ninth fault, older than both reviews, which is fixed too:

- **Each acceptance named by its approval.** The moment of acceptance, kept to the second, could
  be shared by two acceptances, so a removed Google event had to be put back when met. A retry
  could not tell that from an event the owner had removed after an attempt whose answer was lost,
  and put the owner's removal back. Each acceptance is now named by the approval that accepted
  it, which no other acceptance shares. ZeitBoard's own Google event is put back only if it was
  removed before writing to the calendar began. One removed since was the owner's doing and stays
  removed, as it would had the answer come back. The second-precision residual is gone. The third
  review below replaced the clocks this compared with whether a create was sent.
- **Every Microsoft create looks first.** Looking only on a retry missed an attempt cut off by
  quitting or a crash, which counts no failed attempt, and it searched again after every failed
  sign-in. Each create now looks, over every page, before posting. One read per write is cheap
  for times ZeitBoard writes a few of a day, and the retry flag is gone from every writer.
- **Every Google removal checks the mark.** An event's ID no longer names its time alone, so a
  removal now reads the event first and leaves one without this time's mark. A version it holds
  still goes as `If-Match`.
- **Accepted again during a removal.** Found while checking the above, in ADR-0053's queue for
  CalDAV and accounts alike: a time accepted again while its earlier acceptance was still being
  removed found that removal in its place, and was never written. A trigger now queues it once
  the removal settles. A time the owner left out stays out. The third review changed what keeping
  the changed event does.
- **Cancel only for a sign-in that opened.** Whether the browser would open came from a flag the
  sources list read earlier, which a sign-in made since could leave stale. Then a real sign-in ran
  with no Cancel. The desktop now announces a sign-in as it opens the browser, and only then does
  the window offer to cancel. A sign-in also reloads the calendar list, so the flag stays fresh.
  The third review replaced all of this with a Cancel that works at any point.
- **Advice that fits the path.** Refused a sign-in that would take adding events from the account
  written to, “Sign in again” was told to use “Sign in again”. It now asks the owner to allow
  adding events when the provider asks. The form, which asks only to read, still points to “Sign
  in again”.
- **Tests that test.** The quitting test cancelled before the pass began, so it never reached the
  refresh. It now quits during the refresh, and checks the pass got there. The stopping check read
  the status from before it stopped; it reads what stopping returned.
- **No change: names from before.** The first finding was that writes queued before an upgrade
  would get new names. No release or tag carried writing to an account, so no such write exists.

### Writing to accounts after a third review — 2026-10-16

A fresh high-effort review of the second round (PR #90, before it merged) raised nine findings.
Seven are fixed, each with a test; two are left as they are, with reasons:

- **Keeping a changed event is respected.** After the owner kept a changed event in a CalDAV
  calendar, the trigger queued the time accepted again. Its create then took the kept event over
  at its current version, so a later undo would have deleted it. The trigger now runs only after
  a removal is done. Keeping a changed event for a time accepted again leaves that event standing
  for the time, at the version ZeitBoard wrote, so removing it later asks the owner first.
- **A create landing for an undone acceptance.** A create still on its way when its acceptance
  was undone, and the time accepted again, was recorded as the new acceptance's write. A write is
  now recorded only for the acceptance in force. Otherwise what landed is removed first, and the
  new acceptance is written after, through the same trigger.
- **No clocks in putting an event back.** Putting a removed Google event back compared Google's
  time of removal with the computer's time of turning writing on, which a wrong clock could flip.
  This round recorded each create before sending it instead; the fourth review below replaced
  that record with a mark on the event itself.
- **Cancel at any point.** The window offered Cancel once the desktop announced that the browser
  had opened, and could miss that announcement or act on another sign-in's. Turning writing on is
  now held from start to finish, as a sign-in is. Cancel always stops it, whether or not the
  browser was asked, and no other sign-in starts meanwhile. The announcement is gone, and so is
  the flag saying an account already allows writing. So is the reload after every sign-in that
  kept that flag fresh.
- **A busy calendar.** A Microsoft create that found more events in its time than ZeitBoard reads
  failed every time. It now goes ahead, with Graph's own check of the transaction ID against
  writing twice. The one read before each create stays, as the second review chose.
- **A time with no approval.** One no approval records cannot be named, and was retried forever.
  It now waits for the owner as a conflict.
- **No change: a read before each Google removal.** The review suggested deleting first and
  reading only after a refusal. The read is what checks ZeitBoard's mark, which keeps a removal
  from taking another time's event. Removals come one per undo, so the request costs little.
- **No change: names from before.** As in the second review: no release carried writing to an
  account.

### Writing to accounts after a fourth review — 2026-10-16

A fresh high-effort review of the third round raised eight findings. Six are fixed, and two are
left as they are, with reasons:

- **The time of writing on the event.** A create recorded before sending counted as sent even
  when its answer said it had not landed, such as a refused token or a failed attempt to put an
  event back. A retry then took an event removed while writing was off for the owner's removal,
  and never put it back. The record is gone. Each Google event now carries a private mark naming
  the time of writing that wrote it, from turning writing on to stopping. A removed event that
  this time of writing wrote stays removed; one an earlier time wrote is put back. Nothing an
  attempt does or fails to do can change that.
- **A rotated refresh token survives a cancel.** Cancelling while turning writing on, just as
  Microsoft answered a refresh with a new refresh token, failed to keep it, and the sign-in would
  have ended. What a refresh answers is now kept whatever the caller does next.
- **Keeping a changed event in one step.** Keeping it, and forgetting it when the time was not
  accepted again, were two statements; a time accepted between them was never written. They are
  one transaction now.
- **Five minutes for the browser alone.** The limit for waiting in the browser covered all of
  turning writing on, and its message blamed the browser even when none was asked. It covers the
  wait for the browser only, and cancelling is the one way the rest stops.
- **“Sign in again” holds first.** It read whether the account is written to before holding the
  one sign-in that may run, so writing turned on in between could make it ask for too little.
- **One wording for cancelling.** A second helper for the same sign-in's errors is gone.
- **No change: the read before each create, on Microsoft.** It is what finds an event already
  written when writing is turned on again, as well as one a lost answer left. Graph moves a
  deleted event where ZeitBoard does not look, which ADR-0065 records as a residual.
- **No change: the read before each Google removal.** As in the third review.

### Writing to accounts after a fifth review — 2026-10-16

A fresh high-effort review of the fourth round raised ten candidate findings, unverified. Six
repeat ones the earlier rounds settled, and are left with the reasons given there. Four were acted
on:

- **Every outcome on the write carried.** Only a successful create checked that its acceptance
  was still the one in force. A refused or failed create for an acceptance undone meanwhile, the
  time accepted again, marked the new acceptance's write as a conflict or delayed it. Each write
  is now listed with the approval in force. Whatever becomes of it, written, conflict or failure,
  is recorded only while that approval still is, in one statement.
- **Writing on is never reported as cancelled.** A cancel arriving after writing was turned on
  could still be reported, had reading the status then failed. Only the steps before writing is
  on can now be cancelled.
- **Words for the wait.** While writing is turned on, the window now says so first. It no longer
  opens with what to do if the browser asks, which it may not.
- **What a cut-off refresh leaves.** The comment on keeping a refresh's answer said a rotated
  token was the only one still honoured. A refresh cut off before its answer leaves the token kept
  before, which still holds: Microsoft does not revoke a refresh token for being used, and Google's
  do not rotate.

### Writing to accounts after a sixth review — 2026-10-16

A fresh high-effort review of the fifth round raised ten candidate findings, unverified. Seven
repeat ones the earlier rounds settled, or ask for what the design chose on purpose: a kept event
standing for its time, and a time accepted again written even if it has ended meanwhile, as any
waiting create is. Each provider request is bounded by its own twenty seconds, so a request that
hangs cannot hold a sign-in for long. Three were acted on:

- **A pass keeps to its time of writing.** A pass still carrying writes when writing stopped and
  was turned on again, or for another calendar, recorded what it carried on the writes queued for
  the new time of writing, and carried them on with the old calendar's writer. A pass now checks
  before each write that writing is on as it began. It lists only writes of its own time of
  writing, and records every outcome only for that time, in the same statement.
- **Undoing settles a create's conflict.** A create that met a conflict outlived its time's undo,
  and a time accepted again found it in its place, never to be written once the owner left it out.
  A trigger now settles such a conflict when its time is undone.
- **The approval read for creates alone.** Listing writes read the approval in force for every
  write, though only a create's is ever used.

### An undone time's unconfirmed create is looked for — 2026-10-16

A review of the sixth round noted a gap older than ADR-0065. A create can land in the owner's
calendar with its answer lost, or as the app quits. If its time was undone before a retry
confirmed it, ZeitBoard forgot the write, and the event stayed in the calendar for good. It then
came back on every reading as busy time ZeitBoard did not recognise.

- Undoing a time whose create is not yet confirmed now turns its write into a removal of
  whatever that create wrote. A trigger records the approval it was written under, and the time
  it was written for, before the time goes.
- The removal looks where ZeitBoard writes the time: the CalDAV object named by it, the Google
  event under the acceptance's ID, or the Microsoft event with its transaction ID, over the time
  the approval records. Finding nothing, or another's event, it is done.
- Finding ZeitBoard's own event, it cannot tell whether the owner changed it since: it never knew
  the version it wrote. So the event waits for the owner, as a changed one does, at the address
  and version found. The owner removes it or keeps it.
- A create landing after the undo gives that removal the event's address and version, so it
  removes exactly what landed, with nothing to ask.
- A time accepted again meanwhile is written once the removal is done, as with any removal.
- A review of the first version found it removed what it found unasked, which could take an
  owner's change with it. What is found now waits for the owner.
- A second review found that a Microsoft lookup cut short by a time holding more events than
  ZeitBoard reads, retried, would wait for good. A time accepted again would wait behind it. The
  lookup now gives up there, as a create there goes ahead. What to look for keeps the event's own
  time, not one read back from the approval. One rule names the approval in force, for the
  trigger and the queue alike. A removal that knows its address uses it.
- An undo while the calendar cannot be reached leaves its lookup waiting with the other writes,
  and a time accepted again waits behind it. Nothing could be written then anyway, and once the
  calendar answers, the lookup settles and the time is written.
- A third review raised small points, now fixed:
  - A lookup that learns its address from a create landing late starts that removal afresh,
    with no backoff or reason left over.
  - A record of what to look for that cannot be read waits, saying so, instead of being tried
    again at once.
  - The rule for the approval in force is one constant again.
- The third review also asked for things left as they are, each with its reason:
  - A Microsoft time holding more events than ZeitBoard reads gives up the lookup. That is a
    calendar with thousands of events within one accepted block.
  - An event from an earlier time of writing that a lookup finds goes to the owner. Asking is
    safe, and the time was undone.
  - Three readings of a Google event remain, and each judges what it reads for its own purpose.

### The owner's choices — 2026-10-18

Asked what only they could decide, the owner chose:

- **Encryption.** Keep Windows' file encryption, not encryption inside the database, and keep
  ZeitBoard working on other systems. On those, Settings now names the system's own disk
  encryption, FileVault on a Mac or full-disk encryption elsewhere, and never Windows. A refusal
  to encrypt there says the same. The desktop vets clean for Linux and macOS, and CI already
  builds and tests it on Linux.
- **Checking the running app.** Through Claude in Chrome, against the desktop's development
  address and a throwaway data folder, never the owner's own.
- **The window's coverage.** Add `@vitest/coverage-v8` and record the figure.
- **The screen-reader pass.** Narrator, turned on for it, through the development window.

### The running app — 2026-10-18

Claude in Chrome could not reach the desktop's development address, because the browser connected
to it runs on another computer. The built-in browser pane now loads the Wails bridge it once
blocked, so the window was checked there instead, against a throwaway data folder holding 22
synthetic nights. What it found:

- **Home.** The label of Fig. 1's last day ran past the window's edge, and the page scrolled
  sideways. Each label now keeps to its own stretch of the axis, and one that does not fit is left
  off.
- **Plan › Week.** The time of now sat over the hour beside it, half hiding it, and ignored the
  24-hour clock. The hour now gives way, and the time follows the window's clock.
- **Rhythm.** The actogram's right half repeated each row's own sleep, where the time probe said
  the next day. With the forecast shown, time ran up the page and then down it. It is now a true
  double plot, a row for each day and the next, with the days to come above today. Sources no longer
  says every entry feeds the estimate, which reads only the latest 21 sleeps.
- **Settings › This computer.** Login startup spoke of Windows everywhere and offered a switch that
  could not work away from it. Each system is now told what starts ZeitBoard at sign-in.
- **Everything else.** Log, Sharing, Data Sources, the other settings and the assistant rail
  showed no defect. No screen is wider than the window, at the window's minimum size of 900 by 620
  as well.

The Narrator pass has not run. When the window and Narrator were asked for, on 2026-10-18,
control was declined, so the pass waits until the owner allows it. Nothing was turned on or typed.

## Where the completion goal stands — 2026-09-27

Each milestone's own record above says what was done; this is the summary, with what remains and
who it waits on.

| Milestone | Software | Remaining | Waits on |
|---|---|---|---|
| C0 baseline | Done | — | — |
| C1 connected foundation | Done | Native login-start, suspend and resume evidence on the owner's machine; supported-device resource measurements | The owner's machine and phone |
| C2 planning and approval | Acceptance met | — | — |
| C3 medication and context | Done, device run recorded | — | — |
| C4 assistant and agents | Every criterion met | — | — |
| C5 live portal | Done | The independent review and the exposure gate (`portal-design.md` section 12); the portal stays off by default until then | An independent reviewer |
| C6 notifications | Acceptance met | — | — |
| C7 daily-use UX | Done but for one check | A screen-reader pass on real assistive technology: NVDA or Narrator on the desktop, TalkBack on the phone | The owner allowing control of the window for the Narrator pass they chose on 2026-10-18 (declined when asked that day) |
| C8 release | Material review findings resolved; upgrade regressions in CI | Clean-machine install, update across two published commits with rollback, Android bootstrap, service install and purge ceremony (`install-update-design.md`); production APK signing; private pilot and its measures; release artifacts and disposition | A clean Windows 11 VM, the owner's keystore, pilot participants |

### Known limitations and their dispositions

None of these is a critical, high or core-workflow defect. Each is accepted for release as stated,
or deferred with its reason.

| # | Limitation | Disposition |
|---|---|---|
| 1 | ~~The local database is not encrypted at rest.~~ | Resolved 2026-10-12 on Windows editions with file encryption: by choice, Windows encrypts the data folder and every file in it (ADR-0063). Owner-only permissions remain everywhere. The owner chose, 2026-10-18, to keep Windows' file encryption rather than encrypt inside the database; other systems are pointed to their own disk encryption (ADR-0063's update). |
| 2 | ~~A calendar file is refreshed by importing the updated file again, under the same name.~~ | Resolved 2026-10-12: a calendar subscribed by its private address is read again every hour (ADR-0062). A file still refreshes by importing it again. |
| 3 | Accepted times go to one calendar at a time, by CalDAV or from a Google or Microsoft account, and the calendar's host can read their task names (ADR-0053, ADR-0065; disclosed in the offer). | Accepted by design. |
| 4 | Estimates use recorded sleep only. Multi-source inference stays in shadow mode after a negative promotion decision (ADR-0031). | Deferred analysis work. It ships only after a candidate passes the measured gate. |
| 5 | The server's projection folds all relevant sleep history on each request (review #2). | Accepted at the current scale. Revisit with production profiles. |
| 6 | ~~Older desktop methods pass `context.Background()` rather than the app's context (review #15).~~ | Resolved 2026-10-12: every bound method uses the app's context, so quitting cancels what is in flight. `context.Background()` remains only where one is needed: as the fallback before startup, for shutdown deadlines, in a pure computation and in the command-line tools. |
| 7 | The desktop `App` facade is not yet split into services (review #22). | Accepted. Extraction happens at feature boundaries, as the sleep log's reads did. |
| 8 | The calendar, medication and report validators are handwritten, outside generated schema ownership (review #24). | Accepted. They are strict and tested. |
| 9 | There are no ordered migrations yet. Every schema change so far was additive or an explicit drop (review #25). | Accepted. The upgrade tests fail on the first change that is neither, which then needs its migration. |
| 10 | ~~The clinical chart prints, or saves as PDF, but produces no PNG directly.~~ | Resolved 2026-10-11: "Save chart as PNG". |
| 11 | English only. ~~Onboarding is the first-run setup.~~ | English only is accepted for the private release. Times have followed the reader's 12- or 24-hour clock since 2026-10-11 (ADR-0057). Onboarding resolved 2026-10-11: after the first forecast, Home lists what else there is to set up. |
| 12 | ~~Google Takeout and My Activity import are not implemented.~~ | Resolved 2026-10-12: a Takeout export's times become suggested nights beside the recorded ones, in the shadow (ADR-0059). |
| 13 | ~~A correction is undone by adding another, not with one click.~~ | Resolved 2026-10-08: Undo on the night's row takes back its latest edit. |
| 14 | Notices reach a phone through the OS push transport, which the owner's own server does not replace (C6, disclosed). | Accepted and disclosed. |

## The planned functions — 2026-10-08

The goal now covers every planned function, not only the release milestones. This table lists
each function the roadmap and the plans above still name. A function stays open until it ships with
tests and a verification entry. Where it waits on something outside the code, the table says what.

**Standing, 2026-10-12.** Every function in the table is delivered. One possible extension waits
on the owner rather than on code:
- ~~Encryption inside the database needs a new SQLite driver, which is a download and a
  dependency, and a decision on who holds the key.~~ Decided 2026-10-18: the owner keeps Windows'
  file encryption (ADR-0063's update).

| Function | Named in | Status |
|---|---|---|
| Undo a night's last edit in one click | Roadmap, phase 2 | Delivered 2026-10-08 |
| A real list of nights where two sources disagree, in Rhythm › Sources | Roadmap, slice 5 | Delivered 2026-10-08 |
| Missingness recorded per source | Roadmap, phase 2 | Delivered 2026-10-11 for Health Connect (ADR-0054). Manual nights have no coverage to state |
| The clinical chart saved directly as PNG | Roadmap, phase 2; limitation 10 | Delivered 2026-10-11 |
| A self-hosted server trusted through its own CA certificate rather than skipped verification | Roadmap, small debts | Delivered 2026-10-11 for the desktop, the phone and the MCP connector (ADR-0055) |
| Reaching hours for more than one contact, and per-day hours | Roadmap, slice 16 | Delivered 2026-10-11 (ADR-0056) |
| Onboarding beyond the first-run setup | Roadmap, phase 2; limitation 11 | Delivered 2026-10-11 |
| Localization readiness | Roadmap, phase 2; limitation 11 | Delivered 2026-10-11 for times (ADR-0057). Translated text waits for a message catalog |
| Google Takeout and My Activity import, with inference kept in shadow mode | Roadmap, slice 7; limitation 12 | Delivered 2026-10-12 (ADR-0059). Suggestions stay beside the recorded nights until they pass the backtest gate |
| Missed-dose sharing, off by default per link | Medication plan, M-F | Delivered 2026-10-11 (ADR-0058). Only doses logged as skipped are shown |
| Activity collected while the app is closed | Roadmap, slice 11 | Delivered 2026-10-12 (ADR-0060) from Windows' own power log, by a separate choice. The collector itself still stops when ZeitBoard quits |
| OAuth calendar providers | Roadmap, slice 6 and phase 3 | Delivered 2026-10-12: Google and Microsoft accounts sign in through an app the owner registers, with no registration in ZeitBoard's name, and are read every hour (ADR-0064). Accepted times can be written to one of their calendars, with CalDAV write-back's guarantees (ADR-0065). Calendars that publish an address can also subscribe by it (ADR-0062) |
| Packaged cloud skills for assistants | Roadmap, phase 4 | Delivered 2026-10-12 for Claude (ADR-0061), with its privacy and threat review. ChatGPT and Claude on the web take remote servers only, which ZeitBoard does not expose |
| Encrypting the local database | Roadmap, phase 2; limitation 1 | Delivered 2026-10-12 with Windows' file encryption, by choice (ADR-0063). Encryption inside the database, with a key ZeitBoard holds, would need `ncruces/go-sqlite3`'s encrypting VFS: a new pre-v1 dependency to download, and a key-custody decision for the owner. The owner chose not to, 2026-10-18 (ADR-0063's update) |
