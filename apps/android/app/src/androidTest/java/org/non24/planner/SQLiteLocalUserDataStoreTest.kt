package org.non24.planner

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import java.time.Instant
import java.time.ZoneOffset
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.non24.planner.data.LocalUserDataRepository
import org.non24.planner.data.SQLiteLocalUserDataStore
import org.non24.planner.domain.AcquisitionMethod
import org.non24.planner.domain.EvidenceStatus
import org.non24.planner.domain.MedicationEvent
import org.non24.planner.domain.Provenance
import org.non24.planner.domain.SleepCorrection
import org.non24.planner.domain.SleepEpisode

@RunWith(AndroidJUnit4::class)
class SQLiteLocalUserDataStoreTest {
    private val context: Context = ApplicationProvider.getApplicationContext()
    private val databaseName = "zeitboard-local-test.db"

    @Before
    fun deleteDatabaseBeforeTest() {
        context.deleteDatabase(databaseName)
    }

    @After
    fun deleteDatabaseAfterTest() {
        context.deleteDatabase(databaseName)
    }

    @Test
    fun persistsSnapshotCorrectionsAndMedicationEventsAcrossStoreInstances() = runBlocking {
        val episode = episode()
        val correction = correction(episode.id)
        val medication = medication()
        SQLiteLocalUserDataStore(context, databaseName).use { store ->
            val repository = LocalUserDataRepository(store)
            repository.replaceHealthConnectSleepSnapshot(listOf(episode))
            repository.appendSleepCorrection(correction)
            repository.appendMedicationEvent(medication)
        }

        SQLiteLocalUserDataStore(context, databaseName).use { store ->
            val restored = LocalUserDataRepository(store)
            restored.initialize()

            assertEquals(listOf(episode), restored.healthEpisodes.value)
            assertEquals(listOf(correction), restored.correctionHistory.value)
            assertEquals(listOf(medication), restored.medicationEvents.value)
        }
    }

    @Test
    fun conflictingObservationRollsBackSnapshotReplacement() = runBlocking {
        val original = episode()
        SQLiteLocalUserDataStore(context, databaseName).use { store ->
            store.replaceHealthConnectSleepSnapshot(listOf(original))

            val result = runCatching {
                store.replaceHealthConnectSleepSnapshot(
                    listOf(original.copy(end = original.end.plusSeconds(60))),
                )
            }

            assertTrue(result.isFailure)
            assertEquals(listOf(original), store.loadHealthConnectSleepSnapshot(limit = 10))
        }
    }

    private fun episode(): SleepEpisode = SleepEpisode(
        id = "health-connect-version-1",
        logicalSourceId = "synthetic.health.provider\u001fprovider-record-1",
        start = Instant.parse("2026-11-01T05:30:00Z"),
        end = Instant.parse("2026-11-01T13:30:00Z"),
        ianaTimeZoneId = null,
        startZoneOffset = ZoneOffset.ofHours(-4),
        endZoneOffset = ZoneOffset.ofHours(-5),
        provenance = Provenance(
            acquisitionMethod = AcquisitionMethod.HEALTH_CONNECT,
            evidenceStatus = EvidenceStatus.IMPORTED,
            sourceId = "synthetic.health.provider",
            sourceRecordId = "provider-record-1",
            sourceUpdatedAt = Instant.parse("2026-11-01T14:00:00.123456789Z"),
        ),
    )

    private fun correction(targetEpisodeId: String): SleepCorrection = SleepCorrection(
        id = "correction-1",
        targetEpisodeId = targetEpisodeId,
        targetLogicalSourceId = "synthetic.health.provider\u001fprovider-record-1",
        correctedStart = Instant.parse("2026-11-01T05:45:00Z"),
        correctedEnd = Instant.parse("2026-11-01T13:15:00Z"),
        ianaTimeZoneId = null,
        startZoneOffset = ZoneOffset.ofHours(-4),
        endZoneOffset = ZoneOffset.ofHours(-5),
        createdAt = Instant.parse("2026-11-01T14:15:00.987654321Z"),
        provenance = Provenance(
            acquisitionMethod = AcquisitionMethod.MANUAL,
            evidenceStatus = EvidenceStatus.USER_CORRECTED,
            sourceId = "local-user",
        ),
    )

    private fun medication(): MedicationEvent = MedicationEvent(
        id = "medication-1",
        displayName = "Synthetic medication",
        occurredAt = Instant.parse("2026-11-01T12:00:00.111222333Z"),
        timeZoneId = "America/New_York",
        createdAt = Instant.parse("2026-11-01T12:01:00.444555666Z"),
        provenance = Provenance(
            acquisitionMethod = AcquisitionMethod.MANUAL,
            evidenceStatus = EvidenceStatus.USER_CORRECTED,
            sourceId = "local-user",
        ),
    )
}
