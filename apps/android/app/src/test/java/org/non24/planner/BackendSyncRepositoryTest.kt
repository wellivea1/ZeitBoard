package org.non24.planner

import java.time.Instant
import java.time.ZoneId
import java.time.ZoneOffset
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.non24.planner.data.BackendSyncClient
import org.non24.planner.data.BackendSyncRepository
import org.non24.planner.data.OutboxRecord
import org.non24.planner.data.SYNC_BATCH_LIMIT
import org.non24.planner.data.SyncConfig
import org.non24.planner.data.SyncConfigStore
import org.non24.planner.data.SyncOutboxStore
import org.non24.planner.data.SyncState
import org.non24.planner.domain.AcquisitionMethod
import org.non24.planner.domain.EvidenceStatus
import org.non24.planner.domain.Provenance
import org.non24.planner.domain.SleepEpisode

/** An in-memory outbox with the same ordering and idempotency contract. */
private class FakeOutbox : SyncOutboxStore {
    private val rows = LinkedHashMap<String, Pair<OutboxRecord, Instant?>>()

    override fun pending(limit: Int): List<OutboxRecord> =
        rows.values.filter { it.second == null }.map { it.first }
            .sortedWith(compareBy({ it.createdAt }, { it.recordId }))
            .take(limit)

    override fun enqueue(records: List<OutboxRecord>) {
        for (record in records) {
            if (!rows.containsKey(record.recordId)) {
                rows[record.recordId] = record to null
            }
        }
    }

    override fun markSynced(recordIds: List<String>, at: Instant) {
        for (id in recordIds) {
            rows[id]?.let { rows[id] = it.first to at }
        }
    }

    override fun syncedRevisions(): Map<String, Instant> =
        rows.values.filter { it.second != null }
            .associate { it.first.recordId to it.first.sourceRevision }

    override fun pendingCount(): Int = rows.values.count { it.second == null }

    override fun lastSyncedAt(): Instant? = rows.values.mapNotNull { it.second }.maxOrNull()

    override fun clear() = rows.clear()

    fun total(): Int = rows.size
}

private class FakeConfigStore(private var config: SyncConfig? = null) : SyncConfigStore {
    override fun load(): SyncConfig? = config

    override fun save(config: SyncConfig) {
        this.config = config
    }

    override fun clear() {
        config = null
    }
}

private class FakeClient(
    var failPush: Exception? = null,
    var enrollToken: String = "device-token",
) : BackendSyncClient {
    val pushedBatches = mutableListOf<List<OutboxRecord>>()

    override suspend fun enroll(baseUrl: String, enrollmentSecret: String, label: String): Result<String> =
        Result.success(enrollToken)

    override suspend fun push(
        baseUrl: String,
        token: String,
        records: List<OutboxRecord>,
    ): Result<List<String>> {
        failPush?.let { return Result.failure(it) }
        pushedBatches += records
        return Result.success(records.map { it.recordId })
    }
}

class BackendSyncRepositoryTest {

    private val zone = "America/New_York"

    // Offsets are derived from the zone rather than pinned, because a long run
    // of episodes crosses a daylight-saving boundary and a fixed offset would
    // be held back — correctly, but for a reason the test is not about.
    private fun episode(index: Int): SleepEpisode {
        val start = Instant.parse("2026-08-04T04:00:00Z").plusSeconds(index * 90_000L)
        val end = Instant.parse("2026-08-04T12:00:00Z").plusSeconds(index * 90_000L)
        val zoneRules = ZoneId.of(zone).rules
        return episodeAt(index, start, end, zoneRules.getOffset(start), zoneRules.getOffset(end))
    }

    private fun episodeAt(
        index: Int,
        start: Instant,
        end: Instant,
        startOffset: ZoneOffset,
        endOffset: ZoneOffset,
    ) = SleepEpisode(
        id = "rev-$index",
        logicalSourceId = "com.fitbit|episode-$index",
        start = start,
        end = end,
        ianaTimeZoneId = null,
        startZoneOffset = startOffset,
        endZoneOffset = endOffset,
        provenance = Provenance(
            acquisitionMethod = AcquisitionMethod.HEALTH_CONNECT,
            evidenceStatus = EvidenceStatus.IMPORTED,
            sourceId = "com.fitbit",
            sourceRecordId = "episode-$index",
            sourceUpdatedAt = end.plusSeconds(300),
        ),
    )

    private fun repository(
        outbox: FakeOutbox = FakeOutbox(),
        config: SyncConfigStore = FakeConfigStore(SyncConfig("https://host.test", "token", zone)),
        client: FakeClient = FakeClient(),
    ) = Triple(BackendSyncRepository(outbox, config, client), outbox, client)

    @Test
    fun `local only mode is a supported state, not an error`() = runTest {
        val (repository, _, _) = repository(config = FakeConfigStore(null))
        repository.initialise()
        assertEquals(SyncState.OFF, repository.status.value.state)

        // Enqueuing without configuration is a no-op rather than a crash.
        assertEquals(0, repository.enqueue(listOf(episode(0))))
    }

    @Test
    fun `records reach the server and are marked once accepted`() = runTest {
        val (repository, outbox, client) = repository()
        repository.enqueue(List(3) { episode(it) })
        assertEquals(3, outbox.pendingCount())

        val pushed = repository.push()
        assertEquals(3, pushed.getOrNull())
        assertEquals(0, outbox.pendingCount())
        assertEquals(SyncState.SYNCED, repository.status.value.state)
        assertNotNull(repository.status.value.lastSyncedAt)
        assertEquals(1, client.pushedBatches.size)
    }

    /**
     * The failure that actually happens on a phone: the network drops. The
     * queue must survive intact, and the user must be told without losing the
     * last successful time.
     */
    @Test
    fun `a failed push keeps the queue and reports it`() = runTest {
        val outbox = FakeOutbox()
        val client = FakeClient()
        val repository = BackendSyncRepository(outbox, FakeConfigStore(SyncConfig("https://host.test", "t", zone)), client)

        repository.enqueue(List(2) { episode(it) })
        repository.push()
        val successAt = repository.status.value.lastSyncedAt
        assertNotNull(successAt)

        repository.enqueue(listOf(episode(5)))
        client.failPush = IllegalStateException("network unreachable")
        val result = repository.push()

        assertTrue(result.isFailure)
        assertEquals(SyncState.ERROR, repository.status.value.state)
        assertEquals(1, outbox.pendingCount())
        assertEquals(successAt, repository.status.value.lastSyncedAt)
        assertNotNull(repository.status.value.lastError)

        // Recovery needs no resubmission by the user.
        client.failPush = null
        assertEquals(1, repository.push().getOrNull())
        assertEquals(SyncState.SYNCED, repository.status.value.state)
    }

    @Test
    fun `repeated enqueues of unchanged episodes add nothing`() = runTest {
        val (repository, outbox, _) = repository()
        val episodes = List(4) { episode(it) }

        repository.enqueue(episodes)
        repository.push()
        val afterFirst = outbox.total()

        repeat(3) { repository.enqueue(episodes) }
        assertEquals(afterFirst, outbox.total())
        assertEquals(0, outbox.pendingCount())
    }

    @Test
    fun `pushes are batched within the wire limit`() = runTest {
        val (repository, _, client) = repository()
        repository.enqueue(List(SYNC_BATCH_LIMIT + 25) { episode(it) })

        assertEquals(SYNC_BATCH_LIMIT + 25, repository.push().getOrNull())
        assertTrue(client.pushedBatches.size >= 2)
        assertTrue(client.pushedBatches.all { it.size <= SYNC_BATCH_LIMIT })
    }

    /**
     * Re-enrolling against a different instance must not deliver records
     * captured for the previous one.
     */
    @Test
    fun `changing server clears the queue`() = runTest {
        val outbox = FakeOutbox()
        val config = FakeConfigStore(SyncConfig("https://first.test", "token", zone))
        val repository = BackendSyncRepository(outbox, config, FakeClient())

        repository.enqueue(List(2) { episode(it) })
        assertEquals(2, outbox.pendingCount())

        repository.enroll("https://second.test", "secret", zone, "phone")
        assertEquals(0, outbox.pendingCount())
        assertEquals(0, outbox.total())
    }

    @Test
    fun `disabling sync forgets the queue and the token`() = runTest {
        val outbox = FakeOutbox()
        val config = FakeConfigStore(SyncConfig("https://host.test", "token", zone))
        val repository = BackendSyncRepository(outbox, config, FakeClient())

        repository.enqueue(List(2) { episode(it) })
        repository.disable()

        assertEquals(SyncState.OFF, repository.status.value.state)
        assertEquals(0, outbox.total())
        assertFalse(config.load() != null)
    }

    @Test
    fun `travelling episodes are counted as held rather than pushed`() = runTest {
        val (repository, outbox, _) = repository()
        val travelling = episode(0).copy(
            startZoneOffset = ZoneOffset.ofHours(9),
            endZoneOffset = ZoneOffset.ofHours(9),
        )
        repository.enqueue(listOf(travelling, episode(1)))

        assertEquals(1, outbox.pendingCount())
        assertEquals(1, repository.status.value.heldCount)
    }

    @Test
    fun `status reports queued work after a restart`() = runTest {
        val outbox = FakeOutbox()
        val repository = BackendSyncRepository(
            outbox,
            FakeConfigStore(SyncConfig("https://host.test", "token", zone)),
            FakeClient(),
        )
        repository.enqueue(List(2) { episode(it) })

        // A fresh repository over the same durable outbox, as after a process
        // death.
        val restarted = BackendSyncRepository(
            outbox,
            FakeConfigStore(SyncConfig("https://host.test", "token", zone)),
            FakeClient(),
        )
        restarted.initialise()
        assertEquals(SyncState.QUEUED, restarted.status.value.state)
        assertEquals(2, restarted.status.value.queuedCount)
    }
}
