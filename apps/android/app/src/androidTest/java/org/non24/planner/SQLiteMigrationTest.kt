package org.non24.planner

import android.content.ContentValues
import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import java.time.Instant
import java.time.ZoneOffset
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.non24.planner.data.LocalUserDataRepository
import org.non24.planner.data.SQLiteLocalUserDataStore
import org.non24.planner.domain.AcquisitionMethod
import org.non24.planner.domain.EvidenceStatus
import org.non24.planner.domain.Provenance
import org.non24.planner.domain.SleepCorrection
import org.non24.planner.domain.SleepEpisode

@RunWith(AndroidJUnit4::class)
class SQLiteMigrationTest {
    private val context: Context = ApplicationProvider.getApplicationContext()
    private val databaseName = "zeitboard-migration-test.db"

    @Before
    fun deleteDatabaseBeforeTest() {
        context.deleteDatabase(databaseName)
    }

    @After
    fun deleteDatabaseAfterTest() {
        context.deleteDatabase(databaseName)
    }

    @Test
    fun versionOneMigrationBackfillsLogicalSourceIdentityForEpisodeAndCorrection() = runBlocking {
        createVersionOneDatabase()

        SQLiteLocalUserDataStore(context, databaseName).use { store ->
            val repository = LocalUserDataRepository(store)
            repository.initialize()

            val expectedLogicalSourceId = "synthetic.health.provider\u001fprovider-record-1"
            assertEquals(2, store.readableDatabase.version)
            assertEquals(expectedLogicalSourceId, repository.healthEpisodes.value.single().logicalSourceId)
            assertEquals(
                expectedLogicalSourceId,
                repository.correctionHistory.value.single().targetLogicalSourceId,
            )
            assertEquals(
                "correction-v1",
                repository.activeCorrections.value["health-revision-v1"]?.id,
            )
        }
    }

    @Test
    fun sqliteActiveProjectionSurvivesBeyondHistoryCap() = runBlocking {
        val current = episode()
        val active = correction(
            id = "active-old",
            targetEpisodeId = current.id,
            targetLogicalSourceId = current.logicalSourceId,
            createdAt = Instant.parse("2026-06-01T12:00:00Z"),
        )
        SQLiteLocalUserDataStore(context, databaseName).use { store ->
            store.replaceHealthConnectSleepSnapshot(listOf(current))
            store.appendSleepCorrection(active)
            repeat(4) { index ->
                store.appendSleepCorrection(
                    correction(
                        id = "unrelated-$index",
                        targetEpisodeId = "other-revision-$index",
                        targetLogicalSourceId = "other-logical-$index",
                        createdAt = Instant.parse("2026-06-0${index + 2}T12:00:00Z"),
                    ),
                )
            }

            val repository = LocalUserDataRepository(store, correctionHistoryLimit = 1)
            repository.initialize()

            assertEquals(listOf("unrelated-3"), repository.correctionHistory.value.map { it.id })
            assertEquals(active, repository.activeCorrections.value[current.id])
        }
    }

    private fun createVersionOneDatabase() {
        context.openOrCreateDatabase(databaseName, Context.MODE_PRIVATE, null).use { db ->
            db.execSQL(
                """
                CREATE TABLE health_sleep_episodes (
                    id TEXT PRIMARY KEY NOT NULL,
                    start_epoch_second INTEGER NOT NULL,
                    start_nano INTEGER NOT NULL,
                    end_epoch_second INTEGER NOT NULL,
                    end_nano INTEGER NOT NULL,
                    iana_zone_id TEXT,
                    start_offset_seconds INTEGER,
                    end_offset_seconds INTEGER,
                    acquisition_method TEXT NOT NULL,
                    evidence_status TEXT NOT NULL,
                    source_id TEXT NOT NULL,
                    source_record_id TEXT,
                    source_updated_epoch_second INTEGER,
                    source_updated_nano INTEGER
                )
                """.trimIndent(),
            )
            db.execSQL(
                """
                CREATE TABLE health_sleep_snapshot (
                    episode_id TEXT PRIMARY KEY NOT NULL
                )
                """.trimIndent(),
            )
            db.execSQL(
                """
                CREATE TABLE sleep_corrections (
                    id TEXT PRIMARY KEY NOT NULL,
                    target_episode_id TEXT NOT NULL,
                    corrected_start_epoch_second INTEGER NOT NULL,
                    corrected_start_nano INTEGER NOT NULL,
                    corrected_end_epoch_second INTEGER NOT NULL,
                    corrected_end_nano INTEGER NOT NULL,
                    iana_zone_id TEXT,
                    start_offset_seconds INTEGER,
                    end_offset_seconds INTEGER,
                    created_epoch_second INTEGER NOT NULL,
                    created_nano INTEGER NOT NULL,
                    acquisition_method TEXT NOT NULL,
                    evidence_status TEXT NOT NULL,
                    source_id TEXT NOT NULL,
                    source_record_id TEXT,
                    source_updated_epoch_second INTEGER,
                    source_updated_nano INTEGER
                )
                """.trimIndent(),
            )
            db.execSQL(
                """
                CREATE TABLE medication_events (
                    id TEXT PRIMARY KEY NOT NULL,
                    display_name TEXT NOT NULL,
                    occurred_epoch_second INTEGER NOT NULL,
                    occurred_nano INTEGER NOT NULL,
                    time_zone_id TEXT NOT NULL,
                    created_epoch_second INTEGER NOT NULL,
                    created_nano INTEGER NOT NULL,
                    acquisition_method TEXT NOT NULL,
                    evidence_status TEXT NOT NULL,
                    source_id TEXT NOT NULL,
                    source_record_id TEXT,
                    source_updated_epoch_second INTEGER,
                    source_updated_nano INTEGER
                )
                """.trimIndent(),
            )
            db.execSQL(
                "CREATE INDEX idx_sleep_corrections_target_created " +
                    "ON sleep_corrections(" +
                    "target_episode_id, created_epoch_second DESC, created_nano DESC)",
            )
            db.execSQL(
                "CREATE INDEX idx_medication_events_occurred " +
                    "ON medication_events(occurred_epoch_second DESC, occurred_nano DESC)",
            )

            val episode = episode()
            db.insertOrThrow(
                "health_sleep_episodes",
                null,
                ContentValues().apply {
                    put("id", episode.id)
                    put("start_epoch_second", episode.start.epochSecond)
                    put("start_nano", episode.start.nano)
                    put("end_epoch_second", episode.end.epochSecond)
                    put("end_nano", episode.end.nano)
                    putNull("iana_zone_id")
                    put("start_offset_seconds", episode.startZoneOffset?.totalSeconds)
                    put("end_offset_seconds", episode.endZoneOffset?.totalSeconds)
                    put("acquisition_method", AcquisitionMethod.HEALTH_CONNECT.name)
                    put("evidence_status", EvidenceStatus.IMPORTED.name)
                    put("source_id", episode.provenance.sourceId)
                    put("source_record_id", episode.provenance.sourceRecordId)
                    put("source_updated_epoch_second", episode.provenance.sourceUpdatedAt?.epochSecond)
                    put("source_updated_nano", episode.provenance.sourceUpdatedAt?.nano)
                },
            )
            db.insertOrThrow(
                "health_sleep_snapshot",
                null,
                ContentValues().apply { put("episode_id", episode.id) },
            )
            val correction = correction(
                id = "correction-v1",
                targetEpisodeId = episode.id,
                targetLogicalSourceId = episode.logicalSourceId,
                createdAt = Instant.parse("2026-06-01T12:30:00Z"),
            )
            db.insertOrThrow(
                "sleep_corrections",
                null,
                ContentValues().apply {
                    put("id", correction.id)
                    put("target_episode_id", correction.targetEpisodeId)
                    put("corrected_start_epoch_second", correction.correctedStart.epochSecond)
                    put("corrected_start_nano", correction.correctedStart.nano)
                    put("corrected_end_epoch_second", correction.correctedEnd.epochSecond)
                    put("corrected_end_nano", correction.correctedEnd.nano)
                    putNull("iana_zone_id")
                    put("start_offset_seconds", correction.startZoneOffset?.totalSeconds)
                    put("end_offset_seconds", correction.endZoneOffset?.totalSeconds)
                    put("created_epoch_second", correction.createdAt.epochSecond)
                    put("created_nano", correction.createdAt.nano)
                    put("acquisition_method", AcquisitionMethod.MANUAL.name)
                    put("evidence_status", EvidenceStatus.USER_CORRECTED.name)
                    put("source_id", "local-user")
                    putNull("source_record_id")
                    putNull("source_updated_epoch_second")
                    putNull("source_updated_nano")
                },
            )
            db.version = 1
        }
    }

    private fun episode(): SleepEpisode = SleepEpisode(
        id = "health-revision-v1",
        logicalSourceId = "synthetic.health.provider\u001fprovider-record-1",
        start = Instant.parse("2026-06-01T03:00:00.123456789Z"),
        end = Instant.parse("2026-06-01T11:00:00.987654321Z"),
        ianaTimeZoneId = null,
        startZoneOffset = ZoneOffset.ofHours(-4),
        endZoneOffset = ZoneOffset.ofHours(-4),
        provenance = Provenance(
            acquisitionMethod = AcquisitionMethod.HEALTH_CONNECT,
            evidenceStatus = EvidenceStatus.IMPORTED,
            sourceId = "synthetic.health.provider",
            sourceRecordId = "provider-record-1",
            sourceUpdatedAt = Instant.parse("2026-06-01T12:00:00.111222333Z"),
        ),
    )

    private fun correction(
        id: String,
        targetEpisodeId: String,
        targetLogicalSourceId: String,
        createdAt: Instant,
    ): SleepCorrection = SleepCorrection(
        id = id,
        targetEpisodeId = targetEpisodeId,
        targetLogicalSourceId = targetLogicalSourceId,
        correctedStart = Instant.parse("2026-06-01T03:15:00Z"),
        correctedEnd = Instant.parse("2026-06-01T10:45:00Z"),
        ianaTimeZoneId = null,
        startZoneOffset = ZoneOffset.ofHours(-4),
        endZoneOffset = ZoneOffset.ofHours(-4),
        createdAt = createdAt,
        provenance = Provenance(
            acquisitionMethod = AcquisitionMethod.MANUAL,
            evidenceStatus = EvidenceStatus.USER_CORRECTED,
            sourceId = "local-user",
        ),
    )
}
