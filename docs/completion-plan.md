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
current UX repairs and recovered owner direction. Preserve that uncommitted work.

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
- Local medication M-A through M-C exists. Synchronization and complete propose-only agent coverage
  do not; generic statements that the assistant or disease-management phase is complete conceal
  those gaps.
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
availability DTO. Any new private-data boundary needs its versioned contract, ADR, redaction tests
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
