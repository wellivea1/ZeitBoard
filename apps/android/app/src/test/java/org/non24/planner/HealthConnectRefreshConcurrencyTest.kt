package org.non24.planner

import java.time.Instant
import java.time.ZoneOffset
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.async
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Test
import org.non24.planner.data.HealthConnectClientAdapter
import org.non24.planner.data.HealthConnectPermissions
import org.non24.planner.data.HealthConnectSleepRepository
import org.non24.planner.data.InMemoryLocalUserDataStore
import org.non24.planner.data.LocalUserDataRepository
import org.non24.planner.data.LocalUserDataStore
import org.non24.planner.domain.AcquisitionMethod
import org.non24.planner.domain.EvidenceStatus
import org.non24.planner.domain.HealthConnectAvailability
import org.non24.planner.domain.Provenance
import org.non24.planner.domain.SleepEpisode

@OptIn(ExperimentalCoroutinesApi::class)
class HealthConnectRefreshConcurrencyTest {
    @Test
    fun overlappingRequestsSerializeThroughCommitAndCoalesceToOneTrailingRefresh() = runTest {
        val older = episode("revision-old", "2026-06-01T11:00:00Z")
        val newer = episode("revision-new", "2026-06-01T11:01:00Z")
        val adapter = SequencedAdapter(listOf(listOf(older), listOf(newer)))
        val store = BlockingFirstCommitStore(InMemoryLocalUserDataStore())
        val repository = HealthConnectSleepRepository(
            adapter,
            LocalUserDataRepository(store),
        )

        val first = async { repository.refresh() }
        store.firstCommitStarted.await()
        val second = async { repository.refresh() }
        val third = async { repository.refresh() }
        runCurrent()

        assertEquals(1, adapter.readCount)
        assertEquals(1, store.replaceCount)

        store.releaseFirstCommit.complete(Unit)
        first.await()
        second.await()
        third.await()

        assertEquals(2, adapter.readCount)
        assertEquals(2, store.replaceCount)
        assertEquals(listOf(newer), repository.sourceEpisodes.value)
    }

    private class SequencedAdapter(
        private val snapshots: List<List<SleepEpisode>>,
    ) : HealthConnectClientAdapter {
        var readCount = 0
            private set

        override fun availability(): HealthConnectAvailability = HealthConnectAvailability.AVAILABLE

        override suspend fun grantedPermissions(): Set<String> = HealthConnectPermissions.required

        override suspend fun readRecentSleep(): List<SleepEpisode> {
            val index = readCount
            readCount += 1
            return snapshots[index]
        }
    }

    private class BlockingFirstCommitStore(
        private val delegate: LocalUserDataStore,
    ) : LocalUserDataStore by delegate {
        val firstCommitStarted = CompletableDeferred<Unit>()
        val releaseFirstCommit = CompletableDeferred<Unit>()
        var replaceCount = 0
            private set

        override suspend fun replaceHealthConnectSleepSnapshot(episodes: List<SleepEpisode>) {
            replaceCount += 1
            if (replaceCount == 1) {
                firstCommitStarted.complete(Unit)
                releaseFirstCommit.await()
            }
            delegate.replaceHealthConnectSleepSnapshot(episodes)
        }
    }

    private fun episode(id: String, end: String): SleepEpisode = SleepEpisode(
        id = id,
        logicalSourceId = "provider\u001frecord-1",
        start = Instant.parse("2026-06-01T03:00:00Z"),
        end = Instant.parse(end),
        ianaTimeZoneId = null,
        startZoneOffset = ZoneOffset.ofHours(-4),
        endZoneOffset = ZoneOffset.ofHours(-4),
        provenance = Provenance(
            acquisitionMethod = AcquisitionMethod.HEALTH_CONNECT,
            evidenceStatus = EvidenceStatus.IMPORTED,
            sourceId = "provider",
            sourceRecordId = "record-1",
            sourceUpdatedAt = if (id == "revision-old") {
                Instant.parse("2026-06-01T12:00:00Z")
            } else {
                Instant.parse("2026-06-01T12:01:00Z")
            },
        ),
    )
}
