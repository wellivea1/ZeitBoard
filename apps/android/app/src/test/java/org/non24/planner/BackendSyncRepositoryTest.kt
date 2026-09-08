package org.non24.planner

import java.time.Instant
import java.time.ZoneId
import java.time.ZoneOffset
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.CancellationException
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
import org.non24.planner.data.MAX_SYNC_BATCHES
import org.non24.planner.data.SyncContract
import org.non24.planner.data.SyncConfig
import org.non24.planner.data.SyncConfigStore
import org.non24.planner.data.SyncOutboxStore
import org.non24.planner.data.SyncState
import org.non24.planner.data.SourceSyncRevision
import org.non24.planner.data.sourceRevisionFromPayload
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.non24.planner.domain.AcquisitionMethod
import org.non24.planner.domain.EvidenceStatus
import org.non24.planner.domain.Provenance
import org.non24.planner.domain.SleepEpisode

/** An in-memory outbox with the same ordering and idempotency contract. */
internal class FakeOutbox : SyncOutboxStore {
    private val rows = LinkedHashMap<String, Pair<OutboxRecord, Instant?>>()
    private var scope = "synthetic-scope"
    override fun activateScope(scope: String) {
        if (scope != this.scope) rows.clear()
        this.scope = scope
    }

    override fun pending(limit: Int): List<OutboxRecord> =
        rows.values.filter { it.second == null }.map { it.first }
            .take(limit)

    override fun enqueue(records: List<OutboxRecord>) {
        for (record in records) {
            if (!rows.containsKey(record.recordId)) {
                rows[record.recordId] = record to null
            }
        }
    }

    override fun contains(recordId: String) = rows.containsKey(recordId)
    override fun hasPendingManualCorrection(observationId: String) = rows.values.any { (record, accepted) ->
        val payload = Json.parseToJsonElement(record.payload).jsonObject
        accepted == null && record.kind == "correction" && payload["acquisition_method"]?.jsonPrimitive?.content != "health_connect" &&
            payload["target_observation_id"]?.jsonPrimitive?.content == observationId
    }

    override fun markSynced(recordIds: List<String>, at: Instant) {
        for (id in recordIds) {
            rows[id]?.let { rows[id] = it.first to at }
        }
    }

    override fun knownSources(): Map<String, SourceSyncRevision> = buildMap {
        rows.values.forEach { (record, _) ->
            val payload = Json.parseToJsonElement(record.payload).jsonObject
            val isCorrection = record.kind == "correction"
            if (isCorrection && payload["acquisition_method"]?.jsonPrimitive?.content != "health_connect") return@forEach
            val key = payload.getValue(if (isCorrection) "target_observation_id" else "observation_id").jsonPrimitive.content
            val revision = SourceSyncRevision(sourceRevisionFromPayload(record.kind, record.payload), if (isCorrection) record.recordId else null)
            if (get(key)?.revision?.isAfter(revision.revision) != true) put(key, revision)
        }
    }

    override fun pendingCount(): Int = rows.values.count { it.second == null }

    override fun lastSyncedAt(): Instant? = rows.values.mapNotNull { it.second }.maxOrNull()

    override fun clear() = rows.clear()

    fun total(): Int = rows.size
}

internal class FakeConfigStore(private var config: SyncConfig? = null) : SyncConfigStore {
    override fun load(): SyncConfig? = config

    override fun save(config: SyncConfig) {
        this.config = config
    }

    override fun clear() {
        config = null
    }
}

internal class FakeClient(
    var failPush: Exception? = null,
    var enrollToken: String = "device-token",
    var failEnroll: Exception? = null,
    var acknowledgedIds: List<String>? = null,
) : BackendSyncClient {
    val pushedBatches = mutableListOf<List<OutboxRecord>>()
    var reviewResponse: Result<kotlinx.serialization.json.JsonObject> = Result.failure(IllegalStateException("No review configured"))
    override suspend fun sleepReview(baseUrl: String, token: String, observationId: String) = reviewResponse
    override suspend fun pull(baseUrl: String, token: String, since: Long) = Result.success(org.non24.planner.data.PullPage(since, emptyList()))
    override suspend fun companion(baseUrl: String, token: String) = Result.success(emptyServerProjection())

    override suspend fun enroll(baseUrl: String, enrollmentSecret: String, label: String): Result<String> =
        failEnroll?.let { Result.failure(it) } ?: Result.success(enrollToken)

    override suspend fun push(
        baseUrl: String,
        token: String,
        records: List<OutboxRecord>,
    ): Result<List<String>> {
        failPush?.let { return Result.failure(it) }
        pushedBatches += records
        return Result.success(acknowledgedIds ?: records.map { it.recordId })
    }
}

class BackendSyncRepositoryTest {

    @Test
    fun `failed enrollment preserves the previous server and its queue`() = runTest {
        val config = FakeConfigStore(SyncConfig("https://first.test", "old-token", zone, "synthetic-scope"))
        val client = FakeClient(failEnroll = IllegalStateException("unreachable"))
        val (repository, outbox, _) = repository(config = config, client = client)
        repository.enqueue(listOf(episode(0)))
        assertTrue(repository.enroll("https://second.test", "secret", zone, "phone").isFailure)
        assertEquals("https://first.test", config.load()?.baseUrl)
        assertEquals(1, outbox.pendingCount())
        client.failEnroll = null
        assertEquals(1, repository.synchronize().getOrNull())
    }

    @Test
    fun `canonical reenrollment retains pending revisions and upload history`() = runTest {
        val (repository, outbox, _) = repository()
        repository.enqueue(listOf(episode(0)))
        assertTrue(repository.enroll("https://HOST.test:443/", "secret", zone, "phone").isSuccess)
        assertEquals(1, outbox.pendingCount())
    }

    @Test
    fun `invalid home zone cannot replace enrollment`() = runTest {
        val (repository, outbox, _) = repository()
        repository.enqueue(listOf(episode(0)))
        assertTrue(repository.enroll("https://second.test", "secret", "invalid/zone", "phone").isFailure)
        assertEquals(1, outbox.pendingCount())
    }

    @Test
    fun `source revisions queued offline form one stable correction chain`() = runTest {
        val (repository, outbox, client) = repository()
        val original = episode(0)
        val firstRevision = original.provenance.sourceUpdatedAt!!.plusNanos(1)
        val revised = original.copy(
            start = original.start.plusSeconds(60),
            provenance = original.provenance.copy(sourceUpdatedAt = firstRevision),
        )
        assertEquals(1, repository.enqueue(listOf(original)))
        assertEquals(1, repository.enqueue(listOf(revised)))
        assertEquals(0, repository.enqueue(listOf(revised)))
        assertEquals(2, repository.synchronize().getOrNull())
        val firstCorrection = client.pushedBatches.flatten().single { it.kind == "correction" }
        assertFalse(firstCorrection.payload.contains("supersedes_correction_id"))

        val next = revised.copy(provenance = revised.provenance.copy(sourceUpdatedAt = firstRevision.plusNanos(1)))
        assertEquals(1, repository.enqueue(listOf(next)))
        val secondCorrection = outbox.pending(10).single()
        assertFalse(secondCorrection.payload.contains("supersedes_correction_id"))
        repository.synchronize()
        repeat(3) { assertEquals(0, repository.enqueue(listOf(next))) }
        assertEquals(3, outbox.total())
        assertEquals(firstRevision.plusNanos(1), outbox.knownSources()[SyncContract.observationId(original.logicalSourceId)]?.revision)
    }

    @Test
    fun `missing acknowledgment cannot loop or discard queued records`() = runTest {
        val (repository, outbox, client) = repository(client = FakeClient(acknowledgedIds = emptyList()))
        repository.enqueue(listOf(episode(0)))
        assertTrue(repository.synchronize().isFailure)
        assertEquals(1, client.pushedBatches.size)
        assertEquals(1, outbox.pendingCount())
        assertEquals(SyncState.ERROR, repository.status.value.state)
    }

    @Test
    fun `each upload invocation has a finite request budget`() = runTest {
        val (repository, outbox, client) = repository()
        repository.enqueue(List(MAX_SYNC_BATCHES * SYNC_BATCH_LIMIT + 3) { episode(it) })
        assertTrue(repository.synchronize().isFailure)
        assertEquals(MAX_SYNC_BATCHES, client.pushedBatches.size)
        assertEquals(3, outbox.pendingCount())
        assertEquals(SyncState.QUEUED, repository.status.value.state)
    }

    @Test
    fun `cancellation leaves work queued and propagates to the scheduler`() = runTest {
        val (repository, outbox, _) = repository(client = FakeClient(failPush = CancellationException("stopped")))
        repository.enqueue(listOf(episode(0)))
        try {
            repository.synchronize()
            throw AssertionError("Expected cancellation")
        } catch (_: CancellationException) {
            assertEquals(1, outbox.pendingCount())
            assertEquals(SyncState.QUEUED, repository.status.value.state)
        }
    }

    @Test
    fun `empty uploads do not manufacture a successful upload time`() = runTest {
        val (repository, _, client) = repository()
        repository.initialise()
        assertEquals(SyncState.READY, repository.status.value.state)
        assertEquals(0, repository.synchronize().getOrNull())
        assertNull(repository.status.value.lastSyncedAt)
        assertFalse(repository.status.value.hasUploadedKnownRecords)
        assertTrue(client.pushedBatches.isEmpty())
    }

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
        config: SyncConfigStore = FakeConfigStore(SyncConfig("https://host.test", "token", zone, "synthetic-scope")),
        client: FakeClient = FakeClient(),
    ) = Triple(BackendSyncRepository(outbox, config, client, replica = FakeReplica()), outbox, client)

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

        val pushed = repository.synchronize()
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
        val repository = BackendSyncRepository(outbox, FakeConfigStore(SyncConfig("https://host.test", "t", zone, "synthetic-scope")), client, replica = FakeReplica())

        repository.enqueue(List(2) { episode(it) })
        repository.synchronize()
        val successAt = repository.status.value.lastSyncedAt
        assertNotNull(successAt)

        repository.enqueue(listOf(episode(5)))
        client.failPush = IllegalStateException("network unreachable")
        val result = repository.synchronize()

        assertTrue(result.isFailure)
        assertEquals(SyncState.ERROR, repository.status.value.state)
        assertEquals(1, outbox.pendingCount())
        assertEquals(successAt, repository.status.value.lastSyncedAt)
        assertNotNull(repository.status.value.lastError)

        // Recovery needs no resubmission by the user.
        client.failPush = null
        assertEquals(1, repository.synchronize().getOrNull())
        assertEquals(SyncState.SYNCED, repository.status.value.state)
    }

    @Test
    fun `repeated enqueues of unchanged episodes add nothing`() = runTest {
        val (repository, outbox, _) = repository()
        val episodes = List(4) { episode(it) }

        repository.enqueue(episodes)
        repository.synchronize()
        val afterFirst = outbox.total()

        repeat(3) { repository.enqueue(episodes) }
        assertEquals(afterFirst, outbox.total())
        assertEquals(0, outbox.pendingCount())
    }

    @Test
    fun `pushes are batched within the wire limit`() = runTest {
        val (repository, _, client) = repository()
        repository.enqueue(List(SYNC_BATCH_LIMIT + 25) { episode(it) })

        assertEquals(SYNC_BATCH_LIMIT + 25, repository.synchronize().getOrNull())
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
        val config = FakeConfigStore(SyncConfig("https://first.test", "token", zone, "synthetic-scope"))
        val repository = BackendSyncRepository(outbox, config, FakeClient(), replica = FakeReplica())

        repository.enqueue(List(2) { episode(it) })
        assertEquals(2, outbox.pendingCount())

        repository.enroll("https://second.test", "secret", zone, "phone")
        assertEquals(0, outbox.pendingCount())
        assertEquals(0, outbox.total())
    }

    @Test
    fun `disabling sync forgets the queue and the token`() = runTest {
        val outbox = FakeOutbox()
        val config = FakeConfigStore(SyncConfig("https://host.test", "token", zone, "synthetic-scope"))
        val repository = BackendSyncRepository(outbox, config, FakeClient(), replica = FakeReplica())

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
            FakeConfigStore(SyncConfig("https://host.test", "token", zone, "synthetic-scope")),
            FakeClient(), replica = FakeReplica())
        repository.enqueue(List(2) { episode(it) })

        // A fresh repository over the same durable outbox, as after a process
        // death.
        val restarted = BackendSyncRepository(
            outbox,
            FakeConfigStore(SyncConfig("https://host.test", "token", zone, "synthetic-scope")),
            FakeClient(), replica = FakeReplica())
        restarted.initialise()
        assertEquals(SyncState.QUEUED, restarted.status.value.state)
        assertEquals(2, restarted.status.value.queuedCount)
    }
}
