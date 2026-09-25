# Android companion

Native Kotlin and Jetpack Compose companion for ZeitBoard.

## Toolchain

- Android Gradle Plugin 9.2.1 with built-in Kotlin
- Gradle wrapper 9.4.1
- JDK 17 or newer
- compile SDK 36.1, target SDK 36, min SDK 26
- Compose BOM 2026.05.01
- Activity Compose 1.13.0, Navigation Compose 2.9.8, Lifecycle 2.10.0
- Health Connect 1.1.0
- WorkManager 2.11.2; kotlinx.serialization JSON 1.9.0

## Architecture

- `domain`: repository-neutral observations, corrections, events, and imported estimate snapshots.
- `data`: fixture repositories, a durable local SQLite projection, settings persistence, and the
  Health Connect adapter.
- `ui`: one application view model and five Compose destinations.

The Android app does not implement estimation. Fixture mode exposes a labeled synthetic estimate. My
data mode imports recent sleep after READ_SLEEP permission and downloads estimates from the Go core
on the enrolled server. It works as a read-only forecast/task companion without Health Connect
permission. Cached forecasts show their age, expiry and uncertainty; refused or unavailable server
data never becomes a sample forecast.

## Connect and refresh

In Settings, enter your own server's HTTPS address, enrollment secret and home IANA zone. Connecting
selects My data and permits forecast/task downloads plus upload of the recent Health Connect sleep
snapshot and provider revisions when sleep access is granted. Corrections made in the connected
Correct screen also upload. Saved local corrections and their source observations join sync on
enrollment, including sources outside the recent provider snapshot. Medication events remain local.
The screen shows queued and held records, last upload and recovery errors. A last upload is not
proof of a current estimate. Use the current backend, desktop and Android contracts together.

Automatic refresh is separately off by default. Enable it and grant the optional background sleep
permission to allow hourly WorkManager import attempts. Android may delay them; force-stop pauses
jobs until the app reopens. Unsupported devices retain foreground refresh. Imports can queue
offline, and uploads wait for a connected network with bounded retries. Uploads already started in
the foreground can finish while closed. Disconnect cancels jobs and removes the local enrollment and
queue; it retains local evidence and does not erase server data or revoke the device remotely.
Downloaded caches are removed on disconnect; suppression markers for erased Health Connect records
persist. Pure sample mode pauses periodic imports. Sync pulls erasures before uploads and reconciles
restored server history after re-enrollment. Status and Tasks display downloaded state.

For local development only, use `adb reverse tcp:18767 tcp:18767` and a disposable API on
`http://127.0.0.1:18767`. LAN HTTP and emulator-host aliases are refused; ordinary connections
require validated TLS.

## Local persistence

ZeitBoard stores Health Connect snapshot membership, immutable imported source versions,
append-only sleep corrections, and append-only medication events in the app-private
`zeitboard_local.db` database. A successful Health Connect refresh replaces snapshot
membership transactionally; a provider, permission, paging, or storage failure retains
the last successful snapshot. Current snapshots keep only the newest revision for each
stable logical source identity. Immutable revision IDs combine that logical identity with
the provider's modification time.

The runtime projections are explicitly bounded to 10,000 current sleep episodes,
50,000 recent correction-history entries, and 10,000 recent medication events. The
effective correction for every current target is loaded separately through indexed,
bounded queries, so trimming visible history cannot revert a correction. When a provider
changes a source revision, a correction attached to the prior immutable revision is
listed for review and is never silently applied to the new revision.

The current schema 5 creates the downloaded replica/cursor, forecast cache and erasure
suppression directly. Obsolete development databases are explicitly refused and must be
reset intentionally; no prior packaged release needs a migration (ADR-0039).
Unknown upgrades and downgrades fail closed instead of recreating or discarding health-related data.
Android cloud backup and device transfer exclude the database and preferences through the module's
backup rules. Uninstalling removes this app-private data. Opt-in synchronization sends supported
sleep observations and corrections to the owner's enrolled backend; this is not a complete backup of
every local record type. See [ADR-0032](../../docs/decisions/0032-android-sleep-synchronisation.md)
for durable outbox/revision handling and
[ADR-0037](../../docs/decisions/0037-android-automatic-upload-and-source-provenance.md) for
automatic execution and provenance.
[ADR-0038](../../docs/decisions/0038-android-companion-cache-and-erasure.md) covers downloaded
state, erasure and restore reconciliation. [ADR-0039](../../docs/decisions/0039-pre-release-contracts-and-streamlining.md)
keeps one current pre-release contract and removes obsolete prototype compatibility paths.
[ADR-0040](../../docs/decisions/0040-sleep-correction-review-context.md) defines private review
snapshots and connected manual correction sync. Correct shows the original source and competing
edits, preserves timestamp precision and saves classification/exclusion with the resolution.
Offline saves use cached review and the durable queue. New downloaded changes require review;
erasure clears open and cached editing context. One pending edit per source prevents duplicate
offline submissions. The source list shows the latest 500 synced sleep observations.
[ADR-0041](../../docs/decisions/0041-local-corrections-join-enrollment.md) connects pre-enrollment
corrections through the same encoder and outbox. Each exchange queues a bounded page, preserves
the original reviewed revision, and skips held sources so other corrections can progress. Local
heads use a durable append sequence, preserving the latest save across clock changes and compaction.

Local hydration starts independently of Health Connect and publishes an explicit
`Loading`, `Ready`, or `Failed` state. Sleep, correction, and medication screens retain
any last-good projection, label storage failure, disable writes, and avoid presenting an
initialization failure as an empty record set.

## Health Connect ingestion

- Serializes permission checks, provider reads, and transactional commits, coalescing a
  burst of overlapping requests into at most one trailing refresh.
- Reads a 30-day window in 1,000-record pages and consumes every page token.
- Rejects repeated page tokens, more than 100 pages, or more than 10,000 unique records.
- Deduplicates incrementally by source identity and keeps the newest source revision
  before building the in-memory projection.
- Preserves each record's start and end `ZoneOffset`, including different endpoint
  offsets across travel or daylight-saving transitions.
- Leaves the IANA zone unset for Health Connect records because Health Connect supplies
  offsets, not a trustworthy region identifier.

## Civil time and writes

- Named-zone local times in daylight-saving gaps are rejected.
- Repeated local times require an explicit offset such as `-04:00` unless the source
  endpoint offset already disambiguates them.
- Health Connect endpoint offsets remain authoritative when no trustworthy IANA zone is
  supplied by the provider.
- Medication submission has explicit pending, success, and failure states. Duplicate
  taps are ignored while persistence is pending, failed input remains on screen, and the
  form clears only after confirmed persistence. Retries of the same payload reuse the
  immutable event ID, making an uncertain completion idempotent.

## Build

```powershell
$env:JAVA_HOME = 'C:\Program Files\Android\Android Studio\jbr'
./gradlew.bat testDebugUnitTest lintDebug assembleDebug
./gradlew.bat connectedDebugAndroidTest # with an emulator or device connected
```
