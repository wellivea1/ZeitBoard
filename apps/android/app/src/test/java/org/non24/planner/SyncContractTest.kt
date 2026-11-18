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
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test
import org.non24.planner.data.CompleteRead
import org.non24.planner.data.OwnSource
import org.non24.planner.data.ReadWindow
import org.non24.planner.data.SourceCheck
import org.non24.planner.data.SourceOutcome
import org.non24.planner.data.SourceReporting
import org.non24.planner.data.SyncContract
import org.non24.planner.data.SourceSyncRevision
import org.non24.planner.data.validatePulledPayload
import org.non24.planner.domain.AcquisitionMethod
import org.non24.planner.domain.EvidenceStatus
import org.non24.planner.domain.Provenance
import org.non24.planner.domain.SleepEpisode

class SyncContractTest {

    // The same bytes are validated by the server in
    // apps/server/internal/sync/android_contract_test.go.
    @Test
    fun `a source report states what the check found and never a sleep time`() {
        val read = SyncContract.sourceReport(
            SourceCheck(
                outcome = SourceOutcome.READ,
                checkedAt = Instant.parse("2026-09-01T12:00:00Z"),
                backgroundReads = true,
                covered = ReadWindow(Instant.parse("2026-08-02T12:00:00Z"), Instant.parse("2026-09-01T12:00:00Z")),
            ),
            held = 1,
        )
        assertEquals("srcrep-b660e86eecf313b7b6062447", read.recordId)
        assertEquals(SourceReporting.KIND, read.kind)
        assertEquals(
            "{\"report_id\":\"srcrep-b660e86eecf313b7b6062447\",\"source\":\"health_connect\",\"outcome\":\"read\"," +
                "\"reported_at\":\"2026-09-01T12:00:00Z\",\"background_reads\":true," +
                "\"covered\":{\"start_at\":\"2026-08-02T12:00:00Z\",\"end_at\":\"2026-09-01T12:00:00Z\"},\"held\":1}",
            read.payload,
        )
        val missing = SyncContract.sourceReport(
            SourceCheck(SourceOutcome.PERMISSION_MISSING, Instant.parse("2026-09-01T18:00:00Z"), backgroundReads = false),
            held = 0,
        )
        assertEquals(
            "{\"report_id\":\"srcrep-e838fda25037eedb0f2f0072\",\"source\":\"health_connect\",\"outcome\":\"permission_missing\"," +
                "\"reported_at\":\"2026-09-01T18:00:00Z\",\"background_reads\":false}",
            missing.payload,
        )
        assertTrue(SyncContract.isValidIdentifier(read.recordId) && SyncContract.isValidIdentifier(missing.recordId))
        // What the phone sends, it would accept back.
        validatePulledPayload(read.recordId, read.kind, Json.parseToJsonElement(read.payload).jsonObject)
        validatePulledPayload(missing.recordId, missing.kind, Json.parseToJsonElement(missing.payload).jsonObject)
    }

    @Test
    fun `a check that does not read cannot claim a window`() {
        val window = ReadWindow(Instant.parse("2026-08-02T12:00:00Z"), Instant.parse("2026-09-01T12:00:00Z"))
        assertThrows(IllegalArgumentException::class.java) {
            SourceCheck(SourceOutcome.READ_FAILED, Instant.parse("2026-09-01T12:00:00Z"), true, window)
        }
        assertThrows(IllegalArgumentException::class.java) {
            SourceCheck(SourceOutcome.READ, Instant.parse("2026-09-01T12:00:00Z"), true, covered = null)
        }
        assertThrows(IllegalArgumentException::class.java) {
            // A read cannot cover time after it.
            SourceCheck(SourceOutcome.READ, Instant.parse("2026-09-01T11:00:00Z"), true, window)
        }
        // A pulled report that is not one is refused.
        val tampered = Json.parseToJsonElement(
            "{\"report_id\":\"srcrep-e838fda25037eedb0f2f0072\",\"source\":\"health_connect\",\"outcome\":\"permission_missing\"," +
                "\"reported_at\":\"2026-09-01T18:00:00Z\",\"background_reads\":false,\"held\":3}",
        ).jsonObject
        assertThrows(IllegalArgumentException::class.java) {
            validatePulledPayload("srcrep-e838fda25037eedb0f2f0072", SourceReporting.KIND, tampered)
        }
    }

    @Test
    fun `Kotlin emits the shared Go wire fixture including nanosecond revision provenance`() {
        val file = generateSequence(File(requireNotNull(System.getProperty("user.dir"))).absoluteFile) { it.parentFile }
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

    // ADR-0072 and ADR-0073: a night recorded on another clock, which Health
    // Connect then deleted and later returned. The server validates and folds
    // the same bytes in apps/server/internal/sync/android_contract_test.go.
    @Test
    fun `Kotlin emits the shared presence fixture for a night away`() {
        val file = generateSequence(File(requireNotNull(System.getProperty("user.dir"))).absoluteFile) { it.parentFile }
            .map { File(it, "testdata/v1/sync-android-source-presence.json") }.first { it.isFile }
        val expected = Json.parseToJsonElement(file.readText()).jsonObject.getValue("records").jsonArray
        val away = episode(
            logicalSourceId = "synthetic-http-provider|sleep-away",
            start = Instant.parse("2026-09-04T21:00:00Z"), end = Instant.parse("2026-09-05T05:00:00Z"),
            revision = Instant.parse("2026-09-05T05:10:00Z"), offset = ZoneOffset.ofHours(2), sourceRecordId = "sleep-away",
        )
        val first = SyncContract.map(listOf(away), newYork, emptyMap(), Instant.parse("2026-09-05T05:11:00Z"))
        assertTrue("a night away is not held", first.held.isEmpty())
        val observation = first.records.single()
        val synced = mapOf(observation.recordId to SourceSyncRevision(observation.sourceRevision))

        // A complete read a day later no longer returns it.
        val gone = SyncContract.map(
            emptyList(), newYork, synced, Instant.parse("2026-09-06T12:00:01Z"),
            read = CompleteRead(ReadWindow(Instant.parse("2026-08-07T12:00:00Z"), Instant.parse("2026-09-06T12:00:00Z")), emptySet()),
            own = mapOf(observation.recordId to OwnSource(away.start, removed = false)),
        ).records.single()
        // And the next returns it again.
        val back = SyncContract.map(
            listOf(away), newYork, synced, Instant.parse("2026-09-07T12:00:01Z"),
            read = CompleteRead(ReadWindow(Instant.parse("2026-08-08T12:00:00Z"), Instant.parse("2026-09-07T12:00:00Z")), setOf(observation.recordId)),
            own = mapOf(observation.recordId to OwnSource(away.start, removed = true)),
        ).records.single()

        listOf(observation, gone, back).forEachIndexed { index, record ->
            val wire = expected[index].jsonObject
            assertEquals(wire.getValue("recordId").jsonPrimitive.content, record.recordId)
            assertEquals(wire.getValue("kind").jsonPrimitive.content, record.kind)
            assertEquals(wire.getValue("createdAt").jsonPrimitive.content, record.createdAt.toString())
            assertEquals(wire.getValue("payload"), Json.parseToJsonElement(record.payload))
            validatePulledPayload(record.recordId, record.kind, Json.parseToJsonElement(record.payload).jsonObject)
        }
    }

    /**
     * Absence means something only for a source this phone read, inside the
     * window a read covered in full, and only once.
     */
    @Test
    fun `a source is said to be gone once, and only where a read could have returned it`() {
        val read = CompleteRead(ReadWindow(Instant.parse("2026-08-02T12:00:00Z"), Instant.parse("2026-09-01T12:00:00Z")), setOf("hc-present"))
        val own = mapOf(
            // Began less than a day inside the window: it may have fallen out of the read.
            "hc-edge" to OwnSource(Instant.parse("2026-08-03T11:00:00Z"), removed = false),
            "hc-gone" to OwnSource(Instant.parse("2026-08-20T04:00:00Z"), removed = false),
            "hc-said" to OwnSource(Instant.parse("2026-08-21T04:00:00Z"), removed = true),
            "hc-present" to OwnSource(Instant.parse("2026-08-22T04:00:00Z"), removed = false),
        )
        val records = SyncContract.map(emptyList(), newYork, emptyMap(), Instant.parse("2026-09-01T12:00:01Z"), read, own).records
        val words = records.map { Json.parseToJsonElement(it.payload).jsonObject }
        assertEquals(listOf("hc-gone"), words.map { it.getValue("target_observation_id").jsonPrimitive.content })
        assertEquals("source_deleted", words.single().getValue("reason").jsonPrimitive.content)
        assertEquals("2026-09-01T12:00:00Z", words.single().getValue("created_at").jsonPrimitive.content)
        // Without a complete read, nothing is judged.
        assertTrue(SyncContract.map(emptyList(), newYork, emptyMap(), Instant.now(), read = null, own = own).records.isEmpty())
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
     * Health Connect stores offsets, not zones. A night recorded on another
     * clock keeps that clock beside the home zone it is read in, and counts
     * (ADR-0073); it is no longer held back.
     */
    @Test
    fun `an episode recorded on another clock syncs with that clock`() {
        val travelling = episode(offset = ZoneOffset.ofHours(2))
        val mapping = SyncContract.map(listOf(travelling), newYork, emptyMap(), Instant.now())

        assertTrue(mapping.held.isEmpty())
        val payload = Json.parseToJsonElement(mapping.records.single().payload).jsonObject
        assertEquals("America/New_York", payload.getValue("zone_id").jsonPrimitive.content)
        val clock = payload.getValue("source_offset").jsonObject
        assertEquals(7200, clock.getValue("start_seconds").jsonPrimitive.content.toInt())
        assertEquals(7200, clock.getValue("end_seconds").jsonPrimitive.content.toInt())
        // A night at home says nothing of its clock.
        val home = SyncContract.map(listOf(episode()), newYork, emptyMap(), Instant.now()).records.single()
        assertFalse(home.payload.contains("source_offset"))
    }

    @Test
    fun `an episode without offsets is held rather than assumed`() {
        val mapping = SyncContract.map(listOf(episode(offset = null)), newYork, emptyMap(), Instant.now())
        assertEquals(1, mapping.held.size)
        assertEquals(SyncContract.HoldReason.MISSING_OFFSET, mapping.held[0].reason)
    }

    @Test
    fun `an episode with offsets is not held`() {
        assertNull(SyncContract.holdReason(episode()))
        assertNull(SyncContract.holdReason(episode(offset = ZoneOffset.ofHours(9))))
    }

    /**
     * Daylight saving is the case a naive fixed-offset comparison gets wrong:
     * the same zone has a different offset in January.
     */
    @Test
    fun `the away check follows daylight saving`() {
        val winter = episode(
            start = Instant.parse("2026-01-15T05:00:00Z"),
            end = Instant.parse("2026-01-15T13:00:00Z"),
            offset = ZoneOffset.ofHours(-5),
        )
        assertNull(SyncContract.awayOffset(winter, newYork))

        val winterWithSummerOffset = episode(
            start = Instant.parse("2026-01-15T05:00:00Z"),
            end = Instant.parse("2026-01-15T13:00:00Z"),
            offset = ZoneOffset.ofHours(-4),
        )
        assertNotNull(SyncContract.awayOffset(winterWithSummerOffset, newYork))
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
