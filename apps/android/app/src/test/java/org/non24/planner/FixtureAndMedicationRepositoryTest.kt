package org.non24.planner

import java.time.Instant
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.non24.planner.data.FixtureSleepRepository
import org.non24.planner.data.LocalMedicationRepository
import org.non24.planner.data.fixtureEstimateRepository
import org.non24.planner.domain.AcquisitionMethod
import org.non24.planner.domain.EvidenceStatus
import org.non24.planner.domain.MedicationEvent
import org.non24.planner.domain.Provenance

class FixtureAndMedicationRepositoryTest {
    @Test
    fun fixtureProvidesSyntheticImportedSnapshotWithoutCalculatingIt() {
        val sleepRepository = FixtureSleepRepository()
        val estimate = fixtureEstimateRepository().estimate.value

        assertTrue(sleepRepository.sourceEpisodes.value.isNotEmpty())
        assertNotNull(estimate)
        assertEquals("fixture-contract-v1", estimate?.algorithmVersion)
        assertEquals(AcquisitionMethod.FIXTURE, estimate?.provenance?.acquisitionMethod)
        assertEquals(EvidenceStatus.SYNTHETIC, estimate?.provenance?.evidenceStatus)
    }

    @Test
    fun medicationEventsAreAppendedAndReturnedNewestFirst() = runTest {
        val repository = LocalMedicationRepository()
        repository.append(event("first", "2026-06-15T10:00:00Z"))
        repository.append(event("second", "2026-06-15T11:00:00Z"))

        assertEquals(listOf("second", "first"), repository.events.value.map { it.id })
    }

    private fun event(id: String, occurredAt: String) = MedicationEvent(
        id = id,
        displayName = "Synthetic medication",
        occurredAt = Instant.parse(occurredAt),
        timeZoneId = "UTC",
        createdAt = Instant.parse(occurredAt),
        provenance = Provenance(
            acquisitionMethod = AcquisitionMethod.MANUAL,
            evidenceStatus = EvidenceStatus.USER_CORRECTED,
            sourceId = "local-user",
        ),
    )
}
