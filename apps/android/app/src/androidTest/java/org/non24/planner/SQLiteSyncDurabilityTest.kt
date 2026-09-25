package org.non24.planner

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import java.time.Instant
import java.time.ZoneId
import java.time.ZoneOffset
import org.junit.After
import org.junit.Assert.*
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.non24.planner.data.SQLiteLocalUserDataStore
import org.non24.planner.data.SQLiteSyncOutboxStore
import org.non24.planner.data.SyncContract
import org.non24.planner.domain.*

@RunWith(AndroidJUnit4::class)
class SQLiteSyncDurabilityTest {
    private val context: Context = ApplicationProvider.getApplicationContext()
    private val databaseName = "zeitboard-sync-durability-test.db"
    private val original = SleepEpisode(
        id = "synthetic-revision", logicalSourceId = "synthetic-provider|sleep-1",
        start = Instant.parse("2026-09-01T02:00:00Z"), end = Instant.parse("2026-09-01T10:00:00Z"),
        ianaTimeZoneId = null, startZoneOffset = ZoneOffset.UTC, endZoneOffset = ZoneOffset.UTC,
        provenance = Provenance(AcquisitionMethod.HEALTH_CONNECT, EvidenceStatus.IMPORTED, "synthetic-provider", "sleep-1", Instant.parse("2026-09-01T11:00:00.123456789Z")),
    )

    @Before fun prepare() { context.deleteDatabase(databaseName) }
    @After fun clean() { context.deleteDatabase(databaseName) }

    private fun outbox(store: SQLiteLocalUserDataStore) = SQLiteSyncOutboxStore({ store.readableDatabase }, { store.writableDatabase })

    @Test
    fun pendingSourceRevisionSurvivesReopenWithoutRoundingOrDuplication() {
        val record = SyncContract.map(listOf(original), ZoneId.of("UTC"), emptyMap(), Instant.EPOCH).records.single()
        SQLiteLocalUserDataStore(context, databaseName).use { store ->
            outbox(store).apply { activateScope("legacy"); enqueue(listOf(record)) }
        }
        SQLiteLocalUserDataStore(context, databaseName).use { store ->
            val queue = outbox(store).apply { activateScope("legacy") }
            assertEquals(record.sourceRevision, queue.pending(10).single().sourceRevision)
            assertTrue(SyncContract.map(listOf(original), ZoneId.of("UTC"), queue.knownSources(), Instant.now()).records.isEmpty())
            val revised = original.copy(provenance = original.provenance.copy(sourceUpdatedAt = record.sourceRevision.plusNanos(1)))
            val correction = SyncContract.map(listOf(revised), ZoneId.of("UTC"), queue.knownSources(), Instant.now()).records.single()
            queue.enqueue(listOf(correction))
            assertEquals(listOf(record.recordId, correction.recordId), queue.pending(10).map { it.recordId })
            queue.markSynced(listOf(record.recordId, correction.recordId), Instant.now())
        }
        SQLiteLocalUserDataStore(context, databaseName).use { store ->
            val queue = outbox(store).apply { activateScope("legacy") }
            assertEquals(0, queue.pendingCount())
            assertEquals(record.sourceRevision.plusNanos(1), queue.knownSources().getValue(record.recordId).revision)
        }
    }

    @Test
    fun restartAfterEnrollmentCommitCannotUploadTheOldServerQueue() {
        val record = SyncContract.map(listOf(original), ZoneId.of("UTC"), emptyMap(), Instant.EPOCH).records.single()
        SQLiteLocalUserDataStore(context, databaseName).use { store ->
            outbox(store).apply { activateScope("old-server-generation"); enqueue(listOf(record)) }
        }
        // Simulates process death after persisting new enrollment but before queue cleanup.
        SQLiteLocalUserDataStore(context, databaseName).use { store ->
            val queue = outbox(store).apply { activateScope("new-server-generation") }
            assertEquals(0, queue.pendingCount())
            assertTrue(queue.knownSources().isEmpty())
            queue.enqueue(listOf(record))
            assertEquals(1, queue.pendingCount())
        }
    }

}
