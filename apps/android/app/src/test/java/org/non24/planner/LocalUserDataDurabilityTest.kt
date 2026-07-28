package org.non24.planner

import java.time.Instant
import java.time.ZoneOffset
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.non24.planner.data.DurableLocalDataState
import org.non24.planner.data.InMemoryLocalUserDataStore
import org.non24.planner.data.LocalUserDataRepository
import org.non24.planner.data.LocalUserDataStore
import org.non24.planner.domain.AcquisitionMethod
import org.non24.planner.domain.EvidenceStatus
import org.non24.planner.domain.MedicationEvent
import org.non24.planner.domain.Provenance
import org.non24.planner.domain.SleepCorrection
import org.non24.planner.domain.SleepCorrectionPolicy
import org.non24.planner.domain.SleepEpisode

class LocalUserDataDurabilityTest {
    @Test
    fun activeCorrectionSurvivesBeyondBoundedHistory() = runTest {
        val store = InMemoryLocalUserDataStore()
        val current = episode("revision-current", "logical-current")
        val active = correction(
            id = "active-old",
            targetEpisodeId = current.id,
            targetLogicalSourceId = current.logicalSourceId,
            createdAt = "2026-06-01T00:00:00Z",
        )
        store.replaceHealthConnectSleepSnapshot(listOf(current))
        store.appendSleepCorrection(active)
        repeat(5) { index ->
            store.appendSleepCorrection(
                correction(
                    id = "unrelated-$index",
                    targetEpisodeId = "unrelated-revision-$index",
                    targetLogicalSourceId = "unrelated-logical-$index",
                    createdAt = "2026-06-0${index + 2}T00:00:00Z",
                ),
            )
        }

        val repository = LocalUserDataRepository(store, correctionHistoryLimit = 2)
        repository.initialize()

        assertEquals(listOf("unrelated-3", "unrelated-4"), repository.correctionHistory.value.map { it.id })
        assertTrue(active.id !in repository.correctionHistory.value.map { it.id })
        assertEquals(active, repository.activeCorrections.value[current.id])
        assertEquals(
            active.id,
            SleepCorrectionPolicy.effectiveAll(
                listOf(current),
                repository.activeCorrections.value,
            ).single().appliedCorrection?.id,
        )
    }

    @Test
    fun sourceRevisionChangeOrphansCorrectionUntilUserCreatesANewOne() = runTest {
        val store = InMemoryLocalUserDataStore()
        val repository = LocalUserDataRepository(store)
        val firstRevision = episode("revision-1", "logical-source")
        val oldCorrection = correction(
            id = "correction-old",
            targetEpisodeId = firstRevision.id,
            targetLogicalSourceId = firstRevision.logicalSourceId,
            createdAt = "2026-06-01T00:00:00Z",
        )
        repository.replaceHealthConnectSleepSnapshot(listOf(firstRevision))
        repository.appendSleepCorrection(oldCorrection)

        val currentRevision = firstRevision.copy(
            id = "revision-2",
            end = firstRevision.end.plusSeconds(60),
            provenance = firstRevision.provenance.copy(
                sourceUpdatedAt = firstRevision.provenance.sourceUpdatedAt?.plusSeconds(1),
            ),
        )
        repository.replaceHealthConnectSleepSnapshot(listOf(currentRevision))

        assertNull(repository.activeCorrections.value[currentRevision.id])
        assertEquals(oldCorrection, repository.correctionReviews.value.single().correction)
        assertEquals(currentRevision.id, repository.correctionReviews.value.single().currentEpisodeId)
        assertNull(
            SleepCorrectionPolicy.effectiveAll(
                listOf(currentRevision),
                repository.activeCorrections.value,
            ).single().appliedCorrection,
        )

        val reviewedCorrection = correction(
            id = "correction-current",
            targetEpisodeId = currentRevision.id,
            targetLogicalSourceId = currentRevision.logicalSourceId,
            createdAt = "2026-06-02T00:00:00Z",
        )
        repository.appendSleepCorrection(reviewedCorrection)

        assertEquals(reviewedCorrection, repository.activeCorrections.value[currentRevision.id])
        assertTrue(repository.correctionReviews.value.isEmpty())
    }

    @Test
    fun initializationFailurePublishesFailedAndRetryHydratesAtomically() = runTest {
        val delegate = InMemoryLocalUserDataStore()
        val saved = medication("saved")
        delegate.appendMedicationEvent(saved)
        val store = FaultInjectingStore(delegate).apply { failLoads = true }
        val repository = LocalUserDataRepository(store)

        assertEquals(DurableLocalDataState.Loading, repository.state.value)
        assertTrue(runCatching { repository.initialize() }.isFailure)
        assertTrue(repository.state.value is DurableLocalDataState.Failed)
        assertTrue(repository.medicationEvents.value.isEmpty())

        store.failLoads = false
        repository.initialize()

        assertEquals(DurableLocalDataState.Ready, repository.state.value)
        assertEquals(listOf(saved), repository.medicationEvents.value)
    }

    @Test
    fun failedWriteKeepsLastGoodProjectionUntilStorageRecovers() = runTest {
        val store = FaultInjectingStore(InMemoryLocalUserDataStore())
        val repository = LocalUserDataRepository(store)
        val saved = medication("saved")
        repository.appendMedicationEvent(saved)
        store.failMedicationWrites = true

        val result = runCatching { repository.appendMedicationEvent(medication("not-saved")) }

        assertTrue(result.isFailure)
        assertTrue(repository.state.value is DurableLocalDataState.Failed)
        assertEquals(listOf(saved), repository.medicationEvents.value)

        store.failMedicationWrites = false
        repository.initialize()
        assertEquals(DurableLocalDataState.Ready, repository.state.value)
        assertEquals(listOf(saved), repository.medicationEvents.value)
    }

    @Test
    fun concurrentAppendsDoNotLoseMedicationEvents() = runTest {
        val repository = LocalUserDataRepository(InMemoryLocalUserDataStore())

        coroutineScope {
            repeat(100) { index ->
                launch { repository.appendMedicationEvent(medication("medication-$index")) }
            }
        }

        assertEquals(100, repository.medicationEvents.value.size)
        assertEquals(100, repository.medicationEvents.value.map { it.id }.toSet().size)
    }

    private class FaultInjectingStore(
        private val delegate: LocalUserDataStore,
    ) : LocalUserDataStore by delegate {
        var failLoads = false
        var failMedicationWrites = false

        override suspend fun loadMedicationEvents(limit: Int): List<MedicationEvent> {
            if (failLoads) error("synthetic storage read failure")
            return delegate.loadMedicationEvents(limit)
        }

        override suspend fun appendMedicationEvent(event: MedicationEvent) {
            if (failMedicationWrites) error("synthetic storage write failure")
            delegate.appendMedicationEvent(event)
        }
    }

    private fun episode(revisionId: String, logicalSourceId: String): SleepEpisode =
        SleepEpisode(
            id = revisionId,
            logicalSourceId = logicalSourceId,
            start = Instant.parse("2026-06-01T03:00:00Z"),
            end = Instant.parse("2026-06-01T11:00:00Z"),
            ianaTimeZoneId = null,
            startZoneOffset = ZoneOffset.ofHours(-4),
            endZoneOffset = ZoneOffset.ofHours(-4),
            provenance = Provenance(
                acquisitionMethod = AcquisitionMethod.HEALTH_CONNECT,
                evidenceStatus = EvidenceStatus.IMPORTED,
                sourceId = "synthetic.health.provider",
                sourceRecordId = logicalSourceId,
                sourceUpdatedAt = Instant.parse("2026-06-01T12:00:00Z"),
            ),
        )

    private fun correction(
        id: String,
        targetEpisodeId: String,
        targetLogicalSourceId: String,
        createdAt: String,
    ): SleepCorrection = SleepCorrection(
        id = id,
        targetEpisodeId = targetEpisodeId,
        targetLogicalSourceId = targetLogicalSourceId,
        correctedStart = Instant.parse("2026-06-01T03:15:00Z"),
        correctedEnd = Instant.parse("2026-06-01T10:45:00Z"),
        ianaTimeZoneId = null,
        startZoneOffset = ZoneOffset.ofHours(-4),
        endZoneOffset = ZoneOffset.ofHours(-4),
        createdAt = Instant.parse(createdAt),
        provenance = Provenance(
            acquisitionMethod = AcquisitionMethod.MANUAL,
            evidenceStatus = EvidenceStatus.USER_CORRECTED,
            sourceId = "local-user",
        ),
    )

    private fun medication(id: String): MedicationEvent = MedicationEvent(
        id = id,
        displayName = "Synthetic medication",
        occurredAt = Instant.parse("2026-06-01T12:00:00Z").plusSeconds(id.hashCode().toLong()),
        timeZoneId = "America/New_York",
        createdAt = Instant.parse("2026-06-01T12:00:00Z"),
        provenance = Provenance(
            acquisitionMethod = AcquisitionMethod.MANUAL,
            evidenceStatus = EvidenceStatus.USER_CORRECTED,
            sourceId = "local-user",
        ),
    )
}
