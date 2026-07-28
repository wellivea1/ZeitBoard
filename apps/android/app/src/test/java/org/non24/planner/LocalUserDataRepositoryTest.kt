package org.non24.planner

import java.time.Instant
import java.time.ZoneOffset
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.non24.planner.data.InMemoryLocalUserDataStore
import org.non24.planner.data.LocalUserDataRepository
import org.non24.planner.domain.AcquisitionMethod
import org.non24.planner.domain.EvidenceStatus
import org.non24.planner.domain.MedicationEvent
import org.non24.planner.domain.Provenance
import org.non24.planner.domain.SleepCorrection
import org.non24.planner.domain.SleepEpisode

class LocalUserDataRepositoryTest {
    @Test
    fun hydratesSleepCorrectionsAndMedicationEventsAcrossRepositoryInstances() = runTest {
        val store = InMemoryLocalUserDataStore()
        val writer = LocalUserDataRepository(store)
        val episode = episode("episode-1", "2026-06-14T03:00:00Z")
        val correction = correction(episode.id)
        val medication = medication("medication-1", "2026-06-14T13:00:00Z")

        writer.replaceHealthConnectSleepSnapshot(listOf(episode, episode))
        writer.appendSleepCorrection(correction)
        writer.appendMedicationEvent(medication)

        val reader = LocalUserDataRepository(store)
        reader.initialize()

        assertEquals(listOf(episode), reader.healthEpisodes.value)
        assertEquals(listOf(correction), reader.correctionHistory.value)
        assertEquals(listOf(medication), reader.medicationEvents.value)
    }

    @Test
    fun replacementIsDeduplicatedAndSortedNewestFirst() = runTest {
        val repository = LocalUserDataRepository(InMemoryLocalUserDataStore())
        val older = episode("older", "2026-06-13T03:00:00Z")
        val newer = episode("newer", "2026-06-14T03:00:00Z")

        repository.replaceHealthConnectSleepSnapshot(listOf(older, newer, older))

        assertEquals(listOf("newer", "older"), repository.healthEpisodes.value.map { it.id })
    }

    @Test
    fun conflictingImmutableIdsAreRejectedWithoutReplacingSnapshot() = runTest {
        val repository = LocalUserDataRepository(InMemoryLocalUserDataStore())
        val original = episode("same-id", "2026-06-13T03:00:00Z")
        repository.replaceHealthConnectSleepSnapshot(listOf(original))

        val result = runCatching {
            repository.replaceHealthConnectSleepSnapshot(
                listOf(original, original.copy(end = original.end.plusSeconds(60))),
            )
        }

        assertTrue(result.isFailure)
        assertEquals(listOf(original), repository.healthEpisodes.value)
    }

    @Test
    fun oversizedSnapshotIsRejectedWithoutReplacingLastGoodData() = runTest {
        val repository = LocalUserDataRepository(InMemoryLocalUserDataStore())
        val original = episode("original", "2026-06-13T03:00:00Z")
        repository.replaceHealthConnectSleepSnapshot(listOf(original))
        val oversized = List(10_001) { index ->
            original.copy(id = "episode-$index")
        }

        val result = runCatching {
            repository.replaceHealthConnectSleepSnapshot(oversized)
        }

        assertTrue(result.isFailure)
        assertEquals(listOf(original), repository.healthEpisodes.value)
    }

    private fun episode(id: String, start: String): SleepEpisode {
        val startInstant = Instant.parse(start)
        return SleepEpisode(
            id = id,
            logicalSourceId = id,
            start = startInstant,
            end = startInstant.plusSeconds(8 * 60 * 60),
            ianaTimeZoneId = null,
            startZoneOffset = ZoneOffset.ofHours(-4),
            endZoneOffset = ZoneOffset.ofHours(-4),
            provenance = Provenance(
                acquisitionMethod = AcquisitionMethod.HEALTH_CONNECT,
                evidenceStatus = EvidenceStatus.IMPORTED,
                sourceId = "synthetic.health.provider",
                sourceRecordId = id,
                sourceUpdatedAt = Instant.parse("2026-06-15T12:00:00Z"),
            ),
        )
    }

    private fun correction(targetEpisodeId: String): SleepCorrection = SleepCorrection(
        id = "correction-1",
        targetEpisodeId = targetEpisodeId,
        targetLogicalSourceId = targetEpisodeId,
        correctedStart = Instant.parse("2026-06-14T03:15:00Z"),
        correctedEnd = Instant.parse("2026-06-14T11:15:00Z"),
        ianaTimeZoneId = null,
        startZoneOffset = ZoneOffset.ofHours(-4),
        endZoneOffset = ZoneOffset.ofHours(-4),
        createdAt = Instant.parse("2026-06-14T12:00:00Z"),
        provenance = Provenance(
            acquisitionMethod = AcquisitionMethod.MANUAL,
            evidenceStatus = EvidenceStatus.USER_CORRECTED,
            sourceId = "local-user",
        ),
    )

    private fun medication(id: String, occurredAt: String): MedicationEvent = MedicationEvent(
        id = id,
        displayName = "Synthetic medication",
        occurredAt = Instant.parse(occurredAt),
        timeZoneId = "America/New_York",
        createdAt = Instant.parse(occurredAt),
        provenance = Provenance(
            acquisitionMethod = AcquisitionMethod.MANUAL,
            evidenceStatus = EvidenceStatus.USER_CORRECTED,
            sourceId = "local-user",
        ),
    )
}
