package org.non24.planner

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import java.time.Instant
import java.time.ZoneId
import java.time.ZoneOffset
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.*
import org.junit.After
import org.junit.Assert.*
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.non24.planner.data.*
import org.non24.planner.domain.*

@RunWith(AndroidJUnit4::class)
class SQLiteSyncReplicaTest {
    private val context: Context = ApplicationProvider.getApplicationContext()
    private val databaseName = "zeitboard-replica-test.db"
    private val at = Instant.parse("2026-09-02T12:00:00Z")
    private val episode = SleepEpisode("synthetic-local", "synthetic-http-provider|sleep-1",
        Instant.parse("2026-09-02T02:00:00Z"), Instant.parse("2026-09-02T10:00:00Z"),
        null, ZoneOffset.UTC, ZoneOffset.UTC,
        Provenance(AcquisitionMethod.HEALTH_CONNECT, EvidenceStatus.IMPORTED, "synthetic-http-provider", "sleep-1", at))

    @Before fun setup() { context.deleteDatabase(databaseName) }
    @After fun cleanup() { context.deleteDatabase(databaseName) }
    private fun replica(store: SQLiteLocalUserDataStore) = SQLiteSyncReplicaStore { store.writableDatabase }.apply { activateScope("synthetic-scope") }
    private fun task(seq: Long, revision: Int, title: String = "Synthetic task") = PulledRecord(seq, "task_synthetic_r$revision", "task", buildJsonObject {
        put("task_id", "task_synthetic"); put("revision", revision); put("title", title); put("duration_minutes", 30)
        put("status", if (revision == 1) "open" else "done"); put("created_at", at.toString()); put("updated_at", at.toString())
    })
    private fun tombstone(seq: Long, id: String) = PulledRecord(seq, id, "tombstone", buildJsonObject { put("record_id", id) })

    @Test fun revisionsAndTombstonesSurviveReopenWithoutTaskResurrection() {
        SQLiteLocalUserDataStore(context, databaseName).use { store ->
            val replica = replica(store)
            replica.apply(PullPage(2, listOf(task(1, 2), task(2, 1))), at)
            assertEquals(2L, replica.state().tasks.single().revision)
            assertEquals("done", replica.state().tasks.single().status)
        }
        SQLiteLocalUserDataStore(context, databaseName).use { store ->
            val replica = replica(store)
            assertEquals(2L, replica.cursor())
            replica.apply(PullPage(3, listOf(tombstone(3, "task_synthetic_r1"))), at)
            replica.apply(PullPage(4, listOf(task(4, 3))), at)
            assertTrue(replica.state().tasks.isEmpty())
        }
    }

    @Test fun invalidPageRollsBackRowsAndCursorTogether() {
        SQLiteLocalUserDataStore(context, databaseName).use { store ->
            val replica = replica(store)
            val broken = task(2, 2).copy(payload = buildJsonObject { put("task_id", "bad") })
            assertTrue(runCatching { replica.apply(PullPage(2, listOf(task(1, 1), broken)), at) }.isFailure)
            assertEquals(0L, replica.cursor())
            assertTrue(replica.state().tasks.isEmpty())
        }
    }

    @Test fun observationWithTaskShapedIdDoesNotEraseAnUnrelatedTask() {
        SQLiteLocalUserDataStore(context, databaseName).use { store ->
            val replica = replica(store)
            replica.apply(PullPage(1, listOf(task(1, 1))), at)
            val id = "task_synthetic_r9"
            val erased = tombstone(2, id).copy(payload = buildJsonObject {
                put("record_id", id); put("record_kind", "observation")
            })
            replica.apply(PullPage(2, listOf(erased)), at)
            assertEquals("task_synthetic", replica.state().tasks.single().id)
        }
    }

    @Test fun erasedHealthSourceIsPurgedAndCannotBeReimportedAfterReenrollment() = runBlocking {
        SQLiteLocalUserDataStore(context, databaseName).use { store ->
            store.replaceHealthConnectSleepSnapshot(listOf(episode))
            val record = SyncContract.map(listOf(episode), ZoneId.of("UTC"), emptyMap(), at).records.single()
            val outbox = SQLiteSyncOutboxStore({ store.readableDatabase }, { store.writableDatabase }).apply { activateScope("legacy"); enqueue(listOf(record)) }
            val replica = replica(store)
            replica.apply(PullPage(1, listOf(tombstone(1, record.recordId))), at)
            assertTrue(store.loadHealthConnectSleepSnapshot(10).isEmpty())
            assertEquals(0, outbox.pendingCount())
            replica.clear(); replica.activateScope("different-enrollment")
            store.replaceHealthConnectSleepSnapshot(listOf(episode))
            outbox.enqueue(listOf(record))
            assertTrue(store.loadHealthConnectSleepSnapshot(10).isEmpty())
            assertEquals(0, outbox.pendingCount())
        }
    }

    @Test fun equalProviderRevisionWithDifferentTimestampsRetainsTheQueueAndRefuses() {
        SQLiteLocalUserDataStore(context, databaseName).use { store ->
            val original = SyncContract.map(listOf(episode), ZoneId.of("UTC"), emptyMap(), at).records.single()
            val revision = at.plusSeconds(60)
            val remote = SyncContract.sourceCorrection(original.recordId, episode.start.plusSeconds(120), episode.end, revision, at)
            val contradictory = episode.copy(start = episode.start.plusSeconds(240), provenance = episode.provenance.copy(sourceUpdatedAt = revision))
            val queued = SyncContract.map(listOf(contradictory), ZoneId.of("UTC"), emptyMap(), at).records.single()
            val outbox = SQLiteSyncOutboxStore({ store.readableDatabase }, { store.writableDatabase }).apply { activateScope("legacy"); enqueue(listOf(queued)) }
            val replica = replica(store)
            replica.apply(PullPage(2, listOf(original, remote).mapIndexed { index, record ->
                PulledRecord(index + 1L, record.recordId, record.kind, Json.parseToJsonElement(record.payload).jsonObject)
            }), at)
            assertTrue(runCatching { outbox.prepareBatch(100, replica.knownSources()) }.isFailure)
            assertEquals(listOf(queued), outbox.pending(100))
        }
    }

    @Test fun restoredServerRequeuesMissingAcceptedRecordsWithoutRevivingErasure() {
        SQLiteLocalUserDataStore(context, databaseName).use { store ->
            val record = SyncContract.map(listOf(episode), ZoneId.of("UTC"), emptyMap(), at).records.single()
            val outbox = SQLiteSyncOutboxStore({ store.readableDatabase }, { store.writableDatabase }).apply {
                activateScope("legacy"); enqueue(listOf(record)); markSynced(listOf(record.recordId), at)
            }
            val replica = replica(store)
            outbox.reconcileAccepted()
            assertEquals(1, outbox.pendingCount())
            outbox.markSynced(listOf(record.recordId), at)
            replica.apply(PullPage(1, listOf(PulledRecord(1, record.recordId, record.kind, Json.parseToJsonElement(record.payload).jsonObject))), at)
            outbox.reconcileAccepted()
            assertEquals(0, outbox.pendingCount())
            replica.apply(PullPage(2, listOf(tombstone(2, record.recordId))), at)
            replica.clear(); replica.activateScope("restored-enrollment")
            outbox.enqueue(listOf(record)); outbox.reconcileAccepted()
            assertEquals(0, outbox.pendingCount())
        }
    }

    @Test fun reenrollmentRebasesQueuedObservationOntoTheExistingServerSource() {
        SQLiteLocalUserDataStore(context, databaseName).use { store ->
            val original = SyncContract.map(listOf(episode), ZoneId.of("UTC"), emptyMap(), at).records.single()
            val revised = episode.copy(start = episode.start.plusSeconds(120), provenance = episode.provenance.copy(sourceUpdatedAt = at.plusSeconds(60)))
            val queued = SyncContract.map(listOf(revised), ZoneId.of("UTC"), emptyMap(), at).records.single()
            val outbox = SQLiteSyncOutboxStore({ store.readableDatabase }, { store.writableDatabase }).apply { activateScope("legacy"); enqueue(listOf(queued)) }
            val replica = replica(store)
            replica.apply(PullPage(1, listOf(PulledRecord(1, original.recordId, "observation", Json.parseToJsonElement(original.payload).jsonObject))), at)
            val prepared = outbox.prepareBatch(100, replica.knownSources()).single()
            assertEquals("correction", prepared.kind)
            assertEquals(original.recordId, Json.parseToJsonElement(prepared.payload).jsonObject.string("target_observation_id"))
            assertEquals(1, outbox.pendingCount())
        }
    }
}
