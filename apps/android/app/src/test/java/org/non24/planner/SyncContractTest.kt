package org.non24.planner

import java.time.Instant
import java.time.ZoneId
import java.time.ZoneOffset
import java.io.File
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.non24.planner.data.SyncContract
import org.non24.planner.data.SourceSyncRevision
import org.non24.planner.domain.AcquisitionMethod
import org.non24.planner.domain.EvidenceStatus
import org.non24.planner.domain.Provenance
import org.non24.planner.domain.SleepEpisode

class SyncContractTest {

    @Test
    fun `Kotlin emits the shared Go wire fixture including nanosecond revision provenance`() {
        val file = generateSequence(File(System.getProperty("user.dir")).absoluteFile) { it.parentFile }
            .map { File(it, "testdata/v1/sync-android-source-revision.json") }.first { it.isFile }
        val expected = Json.parseToJsonElement(file.readText()).jsonObject.getValue("records").jsonArray
        val original = episode(
            logicalSourceId = "synthetic-http-provider|sleep-1",
            start = Instant.parse("2026-09-02T02:00:00Z"), end = Instant.parse("2026-09-02T10:00:00Z"),
            revision = Instant.parse("2026-09-02T11:00:00.123456789Z"), offset = ZoneOffset.UTC, sourceRecordId = "sleep-1",
        )
        val first = SyncContract.map(listOf(original), ZoneId.of("UTC"), emptyMap(), Instant.parse("2026-09-02T11:01:00Z")).records.single()
        val revised = original.copy(start = original.start.plusSeconds(120), provenance = original.provenance.copy(sourceUpdatedAt = first.sourceRevision.plusNanos(1)))
        val second = SyncContract.map(listOf(revised), ZoneId.of("UTC"), mapOf(first.recordId to SourceSyncRevision(first.sourceRevision)), Instant.parse("2026-09-02T11:02:00Z")).records.single()
        listOf(first, second).forEachIndexed { index, record ->
            val wire = expected[index].jsonObject
            assertEquals(wire.getValue("recordId").jsonPrimitive.content, record.recordId)
            assertEquals(wire.getValue("createdAt").jsonPrimitive.content, record.createdAt.toString())
            assertEquals(wire.getValue("payload"), Json.parseToJsonElement(record.payload))
        }
    }

    private val newYork = ZoneId.of("America/New_York")

    /** August in New York is UTC-4. */
    private fun episode(
        logicalSourceId: String = "com.fitbit|abc",
        start: Instant = Instant.parse("2026-08-04T04:00:00Z"),
        end: Instant = Instant.parse("2026-08-04T12:00:00Z"),
        revision: Instant? = Instant.parse("2026-08-04T12:05:00Z"),
        offset: ZoneOffset? = ZoneOffset.ofHours(-4),
        sourceRecordId: String? = "abc",
    ) = SleepEpisode(
        id = "revision-1",
        logicalSourceId = logicalSourceId,
        start = start,
        end = end,
        ianaTimeZoneId = null,
        startZoneOffset = offset,
        endZoneOffset = offset,
        provenance = Provenance(
            acquisitionMethod = AcquisitionMethod.HEALTH_CONNECT,
            evidenceStatus = EvidenceStatus.IMPORTED,
            sourceId = "com.fitbit",
            sourceRecordId = sourceRecordId,
            sourceUpdatedAt = revision,
        ),
    )

    @Test
    fun `first sight of an episode produces one observation`() {
        val mapping = SyncContract.map(listOf(episode()), newYork, emptyMap(), Instant.now())
        assertEquals(1, mapping.records.size)
        assertEquals("observation", mapping.records[0].kind)
        assertTrue(mapping.held.isEmpty())
        assertTrue(mapping.records[0].payload.contains("\"kind\":\"sleep_episode\""))
        assertTrue(mapping.records[0].payload.contains("\"zone_id\":\"America/New_York\""))
    }

    @Test
    fun `ids satisfy the server identifier rule`() {
        val mapping = SyncContract.map(listOf(episode()), newYork, emptyMap(), Instant.now())
        assertTrue(SyncContract.isValidIdentifier(mapping.records[0].recordId))

        // A source package with characters the rule forbids must still produce
        // a valid id, which is why the id is hashed rather than sanitised.
        val awkward = episode(logicalSourceId = "com.Example_App/Sleep Data (v2)|ID#42")
        val hostile = SyncContract.map(listOf(awkward), newYork, emptyMap(), Instant.now())
        assertTrue(SyncContract.isValidIdentifier(hostile.records[0].recordId))
        assertFalse(hostile.records[0].recordId.contains("Example"))
    }

    @Test
    fun `an unchanged episode produces nothing on a second pass`() {
        val now = Instant.now()
        val first = SyncContract.map(listOf(episode()), newYork, emptyMap(), now)
        val synced = mapOf(first.records[0].recordId to SourceSyncRevision(first.records[0].sourceRevision))

        val second = SyncContract.map(listOf(episode()), newYork, synced, now)
        assertTrue(second.records.isEmpty())
    }

    /**
     * The important one. A revised source record must supersede rather than
     * arrive as a second episode: two observations for one night would shift a
     * drift fit that indexes by cycle.
     */
    @Test
    fun `a revised episode becomes a correction, not a second observation`() {
        val now = Instant.now()
        val original = episode()
        val first = SyncContract.map(listOf(original), newYork, emptyMap(), now)
        val synced = mapOf(first.records[0].recordId to SourceSyncRevision(first.records[0].sourceRevision))

        val revised = episode(
            start = Instant.parse("2026-08-04T04:30:00Z"),
            revision = Instant.parse("2026-08-04T13:00:00Z"),
        )
        val second = SyncContract.map(listOf(revised), newYork, synced, now)

        assertEquals(1, second.records.size)
        assertEquals("correction", second.records[0].kind)
        val payload = second.records[0].payload
        assertTrue(payload.contains("\"target_observation_id\":\"${first.records[0].recordId}\""))
        assertFalse(payload.contains("\"supersedes_correction_id\""))
        assertTrue(payload.contains("\"reason\":\"source_conflict\""))
        assertTrue(payload.contains("\"start_at\":\"2026-08-04T04:30:00Z\""))
        assertTrue(SyncContract.isValidIdentifier(second.records[0].recordId))
    }

    /**
     * Health Connect stores offsets, not zones. An episode whose offset
     * disagrees with the configured home zone is held back rather than
     * labelled with a zone that contradicts its own evidence.
     */
    @Test
    fun `an episode recorded in another offset is held, not guessed`() {
        val travelling = episode(offset = ZoneOffset.ofHours(2))
        val mapping = SyncContract.map(listOf(travelling), newYork, emptyMap(), Instant.now())

        assertTrue(mapping.records.isEmpty())
        assertEquals(1, mapping.held.size)
        assertEquals(SyncContract.HoldReason.ZONE_OFFSET_MISMATCH, mapping.held[0].reason)
    }

    @Test
    fun `an episode without offsets is held rather than assumed`() {
        val mapping = SyncContract.map(listOf(episode(offset = null)), newYork, emptyMap(), Instant.now())
        assertEquals(1, mapping.held.size)
        assertEquals(SyncContract.HoldReason.MISSING_OFFSET, mapping.held[0].reason)
    }

    @Test
    fun `an episode matching the home zone is not held`() {
        assertNull(SyncContract.holdReason(episode(), newYork))
    }

    /**
     * Daylight saving is the case a naive fixed-offset comparison gets wrong:
     * the same zone has a different offset in January.
     */
    @Test
    fun `the offset check follows daylight saving`() {
        val winter = episode(
            start = Instant.parse("2026-01-15T05:00:00Z"),
            end = Instant.parse("2026-01-15T13:00:00Z"),
            offset = ZoneOffset.ofHours(-5),
        )
        assertNull(SyncContract.holdReason(winter, newYork))

        val winterWithSummerOffset = episode(
            start = Instant.parse("2026-01-15T05:00:00Z"),
            end = Instant.parse("2026-01-15T13:00:00Z"),
            offset = ZoneOffset.ofHours(-4),
        )
        assertNotNull(SyncContract.holdReason(winterWithSummerOffset, newYork))
    }

    @Test
    fun `a missing source revision falls back to the episode end`() {
        val mapping = SyncContract.map(
            listOf(episode(revision = null)),
            newYork,
            emptyMap(),
            Instant.now(),
        )
        assertEquals(Instant.parse("2026-08-04T12:00:00Z"), mapping.records[0].sourceRevision)
    }

    /**
     * The source record id is the one caller-controlled string that reaches the
     * wire verbatim; a logical source id is only ever hashed. Quoting has to
     * hold here, or a hostile value breaks the JSON the server parses.
     */
    @Test
    fun `payload quoting survives a hostile source record id`() {
        val awkward = episode(sourceRecordId = "src\"with\\quotes")
        val mapping = SyncContract.map(listOf(awkward), newYork, emptyMap(), Instant.now())
        val payload = mapping.records[0].payload

        assertTrue(payload.endsWith("}}"))
        assertTrue(payload.contains("\\\""))
        assertTrue(payload.contains("\\\\"))
        // The raw, unescaped form must not survive into the payload.
        assertFalse(payload.contains("id\":\"src\"with"))
    }
}
