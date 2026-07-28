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

## Architecture

- `domain`: repository-neutral observations, corrections, events, and imported estimate snapshots.
- `data`: fixture repositories, a durable local SQLite projection, settings persistence, and the Health Connect adapter.
- `ui`: one application view model and four Compose destinations.

The Android app does not implement estimation. Fixture mode exposes a prebuilt synthetic estimate snapshot. Health Connect mode imports recent sleep sessions after the user grants only `READ_SLEEP` and leaves estimation to the shared core.

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

Schema version 2 persists logical source identity separately and migrates version 1 in
place inside `SQLiteOpenHelper`'s upgrade transaction. Unknown upgrades and downgrades
fail closed instead of recreating or discarding health-related data. Android cloud backup
and device transfer exclude the database and preferences through the module's backup
rules. Uninstalling removes this app-private data; ZeitBoard does not sync Android records
to another device.

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
