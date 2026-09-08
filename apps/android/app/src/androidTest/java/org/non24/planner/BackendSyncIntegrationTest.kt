package org.non24.planner

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.time.Instant
import java.time.ZoneOffset
import java.time.temporal.ChronoUnit
import java.net.HttpURLConnection
import java.net.URL
import java.util.UUID
import kotlinx.serialization.json.*
import kotlinx.coroutines.runBlocking
import org.junit.Assert.*
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.non24.planner.data.*
import org.non24.planner.domain.*

/** Opt-in smoke against a disposable Go API reached through adb reverse, never an owner's server. */
@RunWith(AndroidJUnit4::class)
class BackendSyncIntegrationTest {
    @Test
    fun preEnrollmentCorrectionHandoffPreservesUnseenRemoteChangesForReview() = runBlocking {
        assumeTrue(InstrumentationRegistry.getArguments().getString("zeitboardSyncIntegration") == "true")
        val context: Context = ApplicationProvider.getApplicationContext()
        val name = "zeitboard-pre-enrollment-correction-test.db"
        val config = SharedPreferencesSyncConfigStore(context, "zeitboard_pre_enrollment_test")
        val address = "http://127.0.0.1:18767"
        val client = HttpBackendSyncClient()
        val source = SleepEpisode("synthetic-local-before-enrollment", "synthetic-handoff-${UUID.randomUUID()}|sleep",
            Instant.parse("2026-08-01T02:00:00.123456789Z"), Instant.parse("2026-08-01T10:00:00.987654321Z"),
            null, ZoneOffset.UTC, ZoneOffset.UTC, Provenance(AcquisitionMethod.HEALTH_CONNECT, EvidenceStatus.IMPORTED, "synthetic-handoff", "sleep", Instant.parse("2026-08-01T11:00:00Z")))
        val edit = SleepCorrection("cor-local-${UUID.randomUUID()}", source.id, source.logicalSourceId, source.start.plusSeconds(60), source.end.plusNanos(1), null, ZoneOffset.UTC, ZoneOffset.UTC,
            source.end.plusSeconds(7200), Provenance(AcquisitionMethod.MANUAL, EvidenceStatus.USER_CORRECTED, "local-user"))
        val id = SyncContract.observationId(source.logicalSourceId)
        context.deleteDatabase(name); config.clear()
        var cleanupToken: String? = null
        try {
            SQLiteLocalUserDataStore(context, name).use { store ->
                store.replaceHealthConnectSleepSnapshot(listOf(source))
                store.appendSleepCorrection(edit)
                // Aging out of the recent provider snapshot must not strand its saved correction.
                store.replaceHealthConnectSleepSnapshot(emptyList())
                assertEquals(0, SQLiteSyncOutboxStore({ store.readableDatabase }, { store.writableDatabase }).pendingCount())
            }
            val remoteToken = client.enroll(address, "synthetic-completion-secret", "Synthetic other device").getOrThrow()
            cleanupToken = remoteToken
            val changedSource = source.copy(start = source.start.plusSeconds(120), provenance = source.provenance.copy(sourceUpdatedAt = source.provenance.sourceUpdatedAt!!.plusSeconds(600)))
            client.push(address, remoteToken, SyncContract.map(listOf(changedSource), java.time.ZoneId.of("UTC"), emptyMap(), Instant.now()).records).getOrThrow()
            val remoteReview = parseSleepReview(client.sleepReview(address, remoteToken, id).getOrThrow())
            client.push(address, remoteToken, listOf(manualCorrection(remoteReview, "cor-remote-${UUID.randomUUID()}", Instant.now(), changedSource.start, changedSource.end, "nap", false))).getOrThrow()
            SQLiteLocalUserDataStore(context, name).use { store ->
                val queue = SQLiteSyncOutboxStore({ store.readableDatabase }, { store.writableDatabase })
                val sync = BackendSyncRepository(queue, config, client, replica = SQLiteSyncReplicaStore { store.writableDatabase })
                sync.enroll(address, "synthetic-completion-secret", "UTC", "Synthetic handoff phone").getOrThrow()
                // No import, permission grant, explicit enqueue or separate upload method is needed.
                sync.synchronize().getOrThrow()
                assertTrue(queue.contains(edit.id))
                sync.loadSleepReview(id).getOrThrow()
                val review = requireNotNull(sync.sleepReview.value.context)
                assertTrue(review.needsReview); assertEquals(2, review.edits.size)
                assertEquals(changedSource.start, review.source.start)
                assertEquals(edit.correctedEnd, review.edits.single { it.id == edit.id }.end)
                sync.saveSleepReview(review, edit.correctedStart, edit.correctedEnd, "principal", false).getOrThrow()
                sync.synchronize().getOrThrow(); sync.loadSleepReview(id).getOrThrow()
                assertFalse(requireNotNull(sync.sleepReview.value.context).needsReview)
                assertEquals(0, sync.synchronize().getOrThrow())
            }
        } finally {
            cleanupToken?.let { eraseRemote(it, listOf(id)) }
            config.clear(); context.deleteDatabase(name)
        }
    }

    @Test
    fun manualReviewSurvivesRestartResolvesConcurrentEditsAndClearsOnErasure() = runBlocking {
        assumeTrue(InstrumentationRegistry.getArguments().getString("zeitboardSyncIntegration") == "true")
        val context: Context = ApplicationProvider.getApplicationContext()
        val name = "zeitboard-manual-review-integration-test.db"
        val config = SharedPreferencesSyncConfigStore(context, "zeitboard_manual_review_test")
        val address = "http://127.0.0.1:18767"
        val client = HttpBackendSyncClient()
        val source = SleepEpisode("synthetic-manual-source", "synthetic-review-${UUID.randomUUID()}|sleep",
            Instant.parse("2026-09-01T02:00:00.123456789Z"), Instant.parse("2026-09-01T10:00:00.987654321Z"),
            null, ZoneOffset.UTC, ZoneOffset.UTC, Provenance(AcquisitionMethod.HEALTH_CONNECT, EvidenceStatus.IMPORTED, "synthetic-review", "sleep", Instant.parse("2026-09-01T11:00:00Z")))
        val id = SyncContract.observationId(source.logicalSourceId)
        context.deleteDatabase(name); config.clear()
        var initial: SleepReview? = null
        try {
            SQLiteLocalUserDataStore(context, name).use { store ->
                val sync = BackendSyncRepository(SQLiteSyncOutboxStore({ store.readableDatabase }, { store.writableDatabase }), config, client, replica = SQLiteSyncReplicaStore { store.writableDatabase })
                sync.enroll(address, "synthetic-completion-secret", "UTC", "Synthetic review phone").getOrThrow()
                sync.enqueue(listOf(source)); sync.synchronize().getOrThrow()
                assertTrue(sync.companion.value.sleepSources.any { it.id == id })
                sync.loadSleepReview(id).getOrThrow()
                val review = requireNotNull(sync.sleepReview.value.context)
                initial = review
                sync.saveSleepReview(review, source.start, source.end.plusNanos(1), "nap", true).getOrThrow()
                assertTrue(sync.sleepReview.value.pending)
                assertEquals(1, sync.status.value.queuedCount)
            }
            SQLiteLocalUserDataStore(context, name).use { store ->
                val outbox = SQLiteSyncOutboxStore({ store.readableDatabase }, { store.writableDatabase })
                val replica = SQLiteSyncReplicaStore { store.writableDatabase }
                val sync = BackendSyncRepository(outbox, config, client, replica = replica)
                sync.initialise()
                assertEquals(1, sync.status.value.queuedCount)
                assertEquals(source.provenance.sourceUpdatedAt, outbox.knownSources()[id]?.revision)
                sync.synchronize().getOrThrow()
                sync.loadSleepReview(id).getOrThrow()
                var review = requireNotNull(sync.sleepReview.value.context)
                assertFalse(review.needsReview); assertTrue(review.excluded)
                assertEquals("nap", review.classification)
                assertEquals(source.start, review.effective.start)
                assertEquals(source.end.plusNanos(1), review.effective.end)
                val token = requireNotNull(config.load()).token
                val competing = manualCorrection(requireNotNull(initial), "cor-competing-${UUID.randomUUID()}", Instant.now(), source.start.plusSeconds(60), source.end, "principal", false)
                client.push(address, token, listOf(competing)).getOrThrow()
                sync.synchronize().getOrThrow()
                assertTrue(sync.sleepReview.value.stale)
                assertTrue(sync.saveSleepReview(review, source.start, source.end, "principal", false).isFailure)
                sync.loadSleepReview(id).getOrThrow()
                review = requireNotNull(sync.sleepReview.value.context)
                assertTrue(review.needsReview); assertEquals(2, review.edits.size)
                sync.saveSleepReview(review, source.start, source.end, "principal", false).getOrThrow()
                sync.synchronize().getOrThrow()
                assertFalse(sync.sleepReview.value.stale)
                sync.loadSleepReview(id).getOrThrow()
                assertFalse(requireNotNull(sync.sleepReview.value.context).needsReview)
                val connection = URL("$address/v1/sync/erase").openConnection() as HttpURLConnection
                try {
                    connection.requestMethod = "POST"; connection.doOutput = true; connection.instanceFollowRedirects = false
                    connection.connectTimeout = 15000; connection.readTimeout = 30000
                    connection.setRequestProperty("Authorization", "Bearer $token"); connection.setRequestProperty("Content-Type", "application/json")
                    connection.outputStream.use { it.write(buildJsonObject { put("schema_version", "v1"); put("record_ids", JsonArray(listOf(JsonPrimitive(id)))) }.toString().toByteArray()) }
                    assertEquals(200, connection.responseCode)
                } finally { connection.disconnect() }
                sync.synchronize().getOrThrow()
                assertNull(sync.sleepReview.value.context); assertNull(replica.cachedReview(id))
                assertFalse(sync.companion.value.sleepSources.any { it.id == id })
            }
        } finally {
            config.load()?.let { eraseRemote(it.token, listOf(id)) }
            config.clear(); context.deleteDatabase(name)
        }
    }

    @Test
    fun nativePullCachesForecastAndTasksThenAppliesRemoteErasure() = runBlocking {
        assumeTrue(InstrumentationRegistry.getArguments().getString("zeitboardSyncIntegration") == "true")
        val context: Context = ApplicationProvider.getApplicationContext()
        val databaseName = "zeitboard-companion-integration-test.db"
        val config = SharedPreferencesSyncConfigStore(context, "zeitboard_companion_integration_test")
        val runId = UUID.randomUUID().toString()
        val lastStart = Instant.now().truncatedTo(ChronoUnit.DAYS).plusSeconds(3 * 3600)
        val sources = (0..6).map { index ->
            val start = lastStart.minusSeconds((6 - index) * 86400L)
            SleepEpisode("synthetic-$index", "synthetic-companion-$runId|sleep-$index", start, start.plusSeconds(8 * 3600),
                null, ZoneOffset.UTC, ZoneOffset.UTC,
                Provenance(AcquisitionMethod.HEALTH_CONNECT, EvidenceStatus.IMPORTED, "synthetic-companion", "sleep-$index", start.plusSeconds(9 * 3600)))
        }
        val taskId = "task_" + runId.replace("-", "").take(16)
        context.deleteDatabase(databaseName)
        config.clear()
        try {
            SQLiteLocalUserDataStore(context, databaseName).use { store ->
                val client = HttpBackendSyncClient()
                val sync = BackendSyncRepository(
                    SQLiteSyncOutboxStore({ store.readableDatabase }, { store.writableDatabase }), config, client,
                    replica = SQLiteSyncReplicaStore { store.writableDatabase },
                )
                sync.enroll("http://127.0.0.1:18767", "synthetic-completion-secret", "UTC", "Synthetic companion test").getOrThrow()
                store.replaceHealthConnectSleepSnapshot(sources)
                sync.enqueue(sources)
                val token = requireNotNull(config.load()).token
                fun task(revision: Int) = OutboxRecord("${taskId}_r$revision", "task", lastStart, lastStart, buildJsonObject {
                    put("task_id", taskId); put("revision", revision); put("title", "Synthetic companion task")
                    put("duration_minutes", 30); put("status", if (revision == 1) "open" else "done")
                    put("created_at", lastStart.toString()); put("updated_at", lastStart.plusSeconds(revision.toLong()).toString())
                }.toString())
                client.push("http://127.0.0.1:18767", token, listOf(task(1))).getOrThrow()
                sync.synchronize().getOrThrow()
                assertEquals("estimated", sync.companion.value.projection?.status)
                assertNotNull(sync.companion.value.projection?.estimate(Instant.now()))
                assertEquals("open", sync.companion.value.tasks.single { it.id == taskId }.status)
                client.push("http://127.0.0.1:18767", token, listOf(task(2))).getOrThrow()
                sync.synchronize().getOrThrow()
                assertEquals("done", sync.companion.value.tasks.single { it.id == taskId }.status)
                val erasedIds = sources.map { SyncContract.observationId(it.logicalSourceId) } + "${taskId}_r1"
                val body = buildJsonObject {
                    put("schema_version", "v1")
                    put("record_ids", JsonArray(erasedIds.map(::JsonPrimitive)))
                }
                val connection = URL("http://127.0.0.1:18767/v1/sync/erase").openConnection() as HttpURLConnection
                try {
                    connection.requestMethod = "POST"; connection.doOutput = true; connection.instanceFollowRedirects = false
                    connection.connectTimeout = 15000; connection.readTimeout = 30000
                    connection.setRequestProperty("Authorization", "Bearer $token")
                    connection.setRequestProperty("Content-Type", "application/json")
                    connection.outputStream.use { it.write(body.toString().toByteArray(Charsets.UTF_8)) }
                    assertEquals(200, connection.responseCode)
                } finally { connection.disconnect() }
                sync.synchronize().getOrThrow()
                assertTrue(sync.companion.value.tasks.none { it.id == taskId })
                assertTrue(store.loadHealthConnectSleepSnapshot(100).isEmpty())
                assertEquals(0, sync.enqueue(sources))
            }
        } finally {
            config.load()?.let { saved -> eraseRemote(saved.token, sources.map { SyncContract.observationId(it.logicalSourceId) } + "${taskId}_r1") }
            config.clear(); context.deleteDatabase(databaseName)
        }
    }

    @Test
    fun nativeHttpEnrollmentUploadRevisionAndRestart() = runBlocking {
        assumeTrue(InstrumentationRegistry.getArguments().getString("zeitboardSyncIntegration") == "true")
        val context: Context = ApplicationProvider.getApplicationContext()
        val databaseName = "zeitboard-http-integration-test.db"
        val config = SharedPreferencesSyncConfigStore(context, "zeitboard_http_integration_test")
        context.deleteDatabase(databaseName)
        config.clear()
        val source = SleepEpisode(
            id = "synthetic-http-revision", logicalSourceId = "synthetic-http-provider-${UUID.randomUUID()}|sleep-1",
            start = Instant.parse("2026-09-02T02:00:00Z"), end = Instant.parse("2026-09-02T10:00:00Z"),
            ianaTimeZoneId = null, startZoneOffset = ZoneOffset.UTC, endZoneOffset = ZoneOffset.UTC,
            provenance = Provenance(AcquisitionMethod.HEALTH_CONNECT, EvidenceStatus.IMPORTED, "synthetic-http-provider", "sleep-1", Instant.parse("2026-09-02T11:00:00.123456789Z")),
        )
        val revised = source.copy(
            start = source.start.plusSeconds(120),
            provenance = source.provenance.copy(sourceUpdatedAt = source.provenance.sourceUpdatedAt!!.plusNanos(1)),
        )
        try {
            SQLiteLocalUserDataStore(context, databaseName).use { store ->
                val sync = BackendSyncRepository(SQLiteSyncOutboxStore({ store.readableDatabase }, { store.writableDatabase }), config, HttpBackendSyncClient(), replica = SQLiteSyncReplicaStore { store.writableDatabase })
                assertTrue(sync.enroll("http://127.0.0.1:18767", "synthetic-completion-secret", "UTC", "Synthetic instrumentation").isSuccess)
                assertEquals(1, sync.run { enqueue(listOf(source)); synchronize() }.getOrNull())
                assertEquals(1, sync.run { enqueue(listOf(revised)); synchronize() }.getOrNull())
                assertEquals(0, sync.status.value.queuedCount)
            }
            SQLiteLocalUserDataStore(context, databaseName).use { store ->
                val sync = BackendSyncRepository(SQLiteSyncOutboxStore({ store.readableDatabase }, { store.writableDatabase }), config, HttpBackendSyncClient(), replica = SQLiteSyncReplicaStore { store.writableDatabase })
                sync.initialise()
                assertNotNull(sync.status.value.lastSyncedAt)
                assertEquals(0, sync.run { enqueue(listOf(revised)); synchronize() }.getOrNull())
            }
        } finally {
            config.load()?.let { eraseRemote(it.token, listOf(SyncContract.observationId(source.logicalSourceId))) }
            config.clear()
            context.deleteDatabase(databaseName)
        }
    }

    private fun eraseRemote(token: String, ids: List<String>) {
        val connection = URL("http://127.0.0.1:18767/v1/sync/erase").openConnection() as HttpURLConnection
        try {
            connection.requestMethod = "POST"; connection.doOutput = true; connection.instanceFollowRedirects = false
            connection.connectTimeout = 15000; connection.readTimeout = 30000
            connection.setRequestProperty("Authorization", "Bearer $token"); connection.setRequestProperty("Content-Type", "application/json")
            connection.outputStream.use { it.write(buildJsonObject {
                put("schema_version", "v1"); put("record_ids", JsonArray(ids.map(::JsonPrimitive)))
            }.toString().toByteArray()) }
            assertEquals(200, connection.responseCode)
        } finally { connection.disconnect() }
    }
}
