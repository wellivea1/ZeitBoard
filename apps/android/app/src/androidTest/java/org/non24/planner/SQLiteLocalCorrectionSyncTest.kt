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
class SQLiteLocalCorrectionSyncTest {
    private val context: Context = ApplicationProvider.getApplicationContext()
    private val name = "zeitboard-local-correction-sync-test.db"
    private val at = Instant.parse("2026-09-01T12:00:00Z")
    private val source = SleepEpisode("synthetic-revision", "synthetic-provider|sleep", at.minusSeconds(36000), at.minusSeconds(7200), null, ZoneOffset.UTC, ZoneOffset.UTC,
        Provenance(AcquisitionMethod.HEALTH_CONNECT, EvidenceStatus.IMPORTED, "synthetic-provider", "sleep", at.minusSeconds(3600)))
    private fun correction(id: String, parent: String? = null, time: Instant = at, target: SleepEpisode = source) = SleepCorrection(
        id, target.id, target.logicalSourceId, target.start.plusNanos(123456789), target.end.plusSeconds(60), null, target.startZoneOffset, target.endZoneOffset,
        time, Provenance(AcquisitionMethod.MANUAL, EvidenceStatus.USER_CORRECTED, "local-user"), listOfNotNull(parent))
    private fun outbox(store: SQLiteLocalUserDataStore) = SQLiteSyncOutboxStore({ store.readableDatabase }, { store.writableDatabase }).apply { activateScope("synthetic-scope") }
    @Before fun setup() { context.deleteDatabase(name) }
    @After fun cleanup() { context.deleteDatabase(name) }

    @Test fun localChainKeepsItsReviewedSourceAndParentAcrossReopenAndClockChange() = runBlocking {
        val first = correction("cor_first")
        val second = correction("cor_second", first.id, at.minusSeconds(100))
        SQLiteLocalUserDataStore(context, name).use { store ->
            store.replaceHealthConnectSleepSnapshot(listOf(source))
            store.appendSleepCorrection(first); store.appendSleepCorrection(second)
            assertTrue(runCatching { store.appendSleepCorrection(correction("cor_stale")) }.isFailure)
        }
        SQLiteLocalUserDataStore(context, name).use { store ->
            val local = LocalUserDataRepository(store)
            local.initialize()
            assertEquals(second, local.activeCorrections.value[source.id])
            // Even a source outside the current provider snapshot remains available for this handoff.
            store.replaceHealthConnectSleepSnapshot(emptyList())
            val queue = outbox(store)
            assertFalse(queue.queueLocalCorrections(ZoneId.of("UTC"), emptyMap(), at, 100).hasMore)
            val records = queue.pending(100)
            assertEquals(listOf("observation", "correction", "correction"), records.map { it.kind })
            val payload = Json.parseToJsonElement(records.last().payload).jsonObject
            assertEquals(source.provenance.sourceUpdatedAt, payload.instant("based_on_source_revision"))
            assertEquals(first.id, payload.getValue("supersedes_correction_ids").jsonArray.single().jsonPrimitive.content)
            assertEquals(second.correctedStart, payload.getValue("changes").jsonObject.instant("start_at"))
            queue.queueLocalCorrections(ZoneId.of("UTC"), emptyMap(), at.plusSeconds(60), 100)
            assertEquals(records, queue.pending(100))
            assertEquals(source.provenance.sourceUpdatedAt, queue.knownSources()[SyncContract.observationId(source.logicalSourceId)]?.revision)
        }
    }

    // ADR-0073: a correction waits only when its source has no clock, and one
    // held under the earlier rule, which held nights away, uploads with the
    // night's own clock.
    @Test fun aSourceWithNoClockWaitsWithoutStarvingOthersAndANightAwayUploads() = runBlocking {
        SQLiteLocalUserDataStore(context, name).use { store ->
            val unclocked = source.copy(id = "synthetic-unclocked", logicalSourceId = "synthetic-provider|unclocked", startZoneOffset = null, endZoneOffset = null)
            val away = source.copy(id = "synthetic-travel", logicalSourceId = "synthetic-provider|travel", startZoneOffset = ZoneOffset.ofHours(9), endZoneOffset = ZoneOffset.ofHours(9))
            store.replaceHealthConnectSleepSnapshot(listOf(unclocked, away, source))
            store.appendSleepCorrection(correction("cor_unclocked", target = unclocked))
            store.appendSleepCorrection(correction("cor_home"))
            store.appendSleepCorrection(correction("cor_away", target = away))
            store.writableDatabase.execSQL("UPDATE sleep_corrections SET sync_hold_zone = 'UTC' WHERE id = 'cor_away'")
            val queue = outbox(store)
            assertEquals(1, queue.queueLocalCorrections(ZoneId.of("UTC"), emptyMap(), at, 1).held)
            assertTrue(queue.pending(100).isEmpty())
            queue.queueLocalCorrections(ZoneId.of("UTC"), emptyMap(), at, 100)
            assertTrue(queue.contains("cor_home")); assertTrue(queue.contains("cor_away")); assertFalse(queue.contains("cor_unclocked"))
            val night = queue.pending(100).single { it.recordId == SyncContract.observationId(away.logicalSourceId) }
            val clock = Json.parseToJsonElement(night.payload).jsonObject.getValue("source_offset").jsonObject
            assertEquals(32400, clock.getValue("start_seconds").jsonPrimitive.int)
            assertEquals(1, queue.queueLocalCorrections(ZoneId.of("Asia/Tokyo"), emptyMap(), at, 100).held)
        }
    }

    // ADR-0072: only a source this phone read and synced can be found gone,
    // and the provider's last word, uploaded or not, says whether it is.
    @Test fun ownSourcesAreThisPhonesSyncedReadsWithTheProvidersLastWord() = runBlocking {
        SQLiteLocalUserDataStore(context, name).use { store ->
            val unsynced = source.copy(id = "synthetic-unsynced", logicalSourceId = "synthetic-provider|unsynced")
            store.replaceHealthConnectSleepSnapshot(listOf(source, unsynced))
            val queue = outbox(store)
            val id = SyncContract.observationId(source.logicalSourceId)
            queue.enqueue(SyncContract.map(listOf(source), ZoneId.of("UTC"), emptyMap(), at).records)
            assertEquals(mapOf(id to OwnSource(source.start, removed = false)), queue.ownSources())
            queue.enqueue(listOf(SyncContract.sourcePresence(id, removed = true, at)))
            assertEquals(OwnSource(source.start, removed = true), queue.ownSources()[id])
            // A record gone is no revision of its times.
            assertEquals(source.provenance.sourceUpdatedAt, queue.knownSources()[id]?.revision)
            queue.enqueue(listOf(SyncContract.sourcePresence(id, removed = false, at.plusSeconds(60))))
            assertFalse(queue.ownSources().getValue(id).removed)
        }
    }

    @Test fun restoredMissingSourceIsRequeuedAndErasureRemovesTheLocalCorrection() = runBlocking {
        SQLiteLocalUserDataStore(context, name).use { store ->
            store.replaceHealthConnectSleepSnapshot(listOf(source))
            val edit = correction("cor_restored")
            store.appendSleepCorrection(edit)
            val queue = outbox(store)
            queue.queueLocalCorrections(ZoneId.of("UTC"), emptyMap(), at, 100)
            queue.markSynced(queue.pending(100).map { it.recordId }, at)
            val id = SyncContract.observationId(source.logicalSourceId)
            store.writableDatabase.delete("sync_outbox", "record_id = ?", arrayOf(id))
            queue.queueLocalCorrections(ZoneId.of("UTC"), emptyMap(), at, 100)
            assertTrue(queue.contains(id))
            val replica = SQLiteSyncReplicaStore { store.writableDatabase }.apply { activateScope("synthetic-replica") }
            replica.apply(PullPage(1, listOf(PulledRecord(1, edit.id, "tombstone", buildJsonObject { put("record_id", edit.id); put("record_kind", "correction") }))), at)
            assertTrue(store.loadSleepCorrections(100).isEmpty())
            queue.queueLocalCorrections(ZoneId.of("UTC"), emptyMap(), at, 100)
            assertFalse(queue.contains(edit.id))
        }
    }

    @Test fun sampleCorrectionsNeverEnterTheEnrollmentQueue() = runBlocking {
        SQLiteLocalUserDataStore(context, name).use { store ->
            val fixture = fixtureSleepEpisodes().first()
            store.appendSleepCorrection(correction("cor_fixture_only", target = fixture))
            val queue = outbox(store)
            val result = queue.queueLocalCorrections(ZoneId.of("UTC"), emptyMap(), at, 100)
            assertFalse(result.hasMore); assertEquals(0, result.held)
            assertEquals(0, queue.pendingCount())
            assertEquals(1, store.loadSleepCorrections(100).size)
        }
    }
}
