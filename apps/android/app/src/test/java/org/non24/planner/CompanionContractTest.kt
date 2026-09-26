package org.non24.planner

import java.io.File
import java.time.Instant
import kotlinx.serialization.json.*
import org.junit.Assert.*
import org.junit.Test
import org.non24.planner.data.*

internal fun companionFixture(name: String = "companion.json"): JsonObject {
    val file = generateSequence(File(requireNotNull(System.getProperty("user.dir"))).absoluteFile) { it.parentFile }
        .map { File(it, "testdata/v2/$name") }.first { it.isFile }
    return Json.parseToJsonElement(file.readText()).jsonObject
}

class CompanionContractTest {
    @Test fun `cached forecasts retain typed windows and expire without recomputing on Android`() {
        val projection = parseCompanion(companionFixture())
        val beforeExpiry = Instant.parse("2026-09-02T12:10:00Z")
        assertTrue(projection.isCurrentSnapshot(beforeExpiry))
        assertFalse(projection.isCurrentSnapshot(Instant.parse("2026-09-02T12:15:00Z")))
        assertFalse(projection.isCurrentSnapshot(Instant.parse("2026-09-01T12:00:00Z")))
        assertTrue(projection.containsSyntheticData)
        assertEquals("Server sample forecast", projection.estimate(beforeExpiry)?.label)
        assertEquals(Instant.parse("2026-09-03T03:00:00Z"), projection.estimate(beforeExpiry)?.predictedSleepWindow?.start)
        assertNull(projection.estimate(Instant.parse("2026-09-04T00:00:00Z")))
        assertEquals("theil-sen-sleep-start/v1", projection.algorithm)
    }

    @Test fun `server refusal cannot leave an old forecast in the replacement projection`() {
        val projection = parseCompanion(companionFixture("companion-refused.json"))
        assertEquals("refused", projection.status)
        assertEquals("withheld", projection.freshness)
        assertTrue(projection.forecasts.isEmpty())
        assertNull(projection.estimate(Instant.parse("2026-09-02T12:05:00Z")))
    }

    @Test fun `unsupported version missing provenance invalid confidence and inverted windows fail closed`() {
        val valid = companionFixture()
        val invalid = listOf(
            JsonObject(valid + ("unexpected_private_field" to JsonPrimitive("synthetic"))),
            JsonObject(valid + ("schema_version" to JsonPrimitive("v99"))),
            JsonObject(valid - "provenance"),
            JsonObject(valid + ("confidence" to buildJsonObject { put("level", "certain"); put("reasons", JsonArray(emptyList())) })),
            JsonObject(valid + ("valid_until" to JsonPrimitive("2026-09-02T11:00:00Z"))),
            JsonObject(valid + ("valid_until" to JsonPrimitive("2026-09-03T12:00:00Z"))),
            JsonObject(valid + ("confidence" to buildJsonObject {
                put("level", "low"); put("reasons", JsonArray(listOf(JsonPrimitive(12))))
            })),
        )
        invalid.forEach { assertTrue(runCatching { parseCompanion(it) }.isFailure) }
    }

    @Test fun `pull cursor cannot skip records move backward or accept unsupported kinds`() {
        val empty = buildJsonObject { put("schema_version", "v1"); put("cursor", 4); put("records", JsonArray(emptyList())) }
        assertEquals(4L, parsePullPage(empty, 4).cursor)
        assertTrue(runCatching { parsePullPage(empty, 3) }.isFailure)
        val row = buildJsonObject {
            put("seq", 5); put("recordId", "obs_synthetic"); put("kind", "unknown"); put("deviceId", "device_synthetic")
            put("createdAt", "2026-09-02T12:00:00Z"); put("payload", buildJsonObject {})
        }
        val invalid = buildJsonObject { put("schema_version", "v1"); put("cursor", 5); put("records", JsonArray(listOf(row))) }
        assertTrue(runCatching { parsePullPage(invalid, 4) }.isFailure)
    }

    @Test fun `task identity must match the revision in the immutable record id`() {
        val task = buildJsonObject {
            put("task_id", "task_synthetic"); put("revision", 2); put("title", "Synthetic task")
            put("duration_minutes", 30); put("status", "open"); put("created_at", "2026-09-02T12:00:00Z")
            put("updated_at", "2026-09-02T12:01:00Z"); put("minimum_confidence", "high")
            put("earliest_start_at", "2026-11-01T05:30:00Z"); put("latest_finish_at", "2026-11-01T06:30:00Z")
        }
        val parsed = parseSyncedTask("task_synthetic_r2", task)
        assertEquals(Instant.parse("2026-11-01T06:30:00Z"), parsed.deadline)
        assertEquals("high", parsed.minimumConfidence)
        assertTrue(runCatching { parseSyncedTask("task_synthetic_r2", JsonObject(task - "updated_at")) }.isFailure)
        assertTrue(runCatching { parseSyncedTask("task_synthetic_r1", task) }.isFailure)
    }

    private fun placement(id: String = "event_accepted_01", start: String = "2026-09-27T18:30:00Z", end: String = "2026-09-27T20:00:00Z") =
        buildJsonObject {
            put("placement_id", id); put("task_id", "task_synthetic"); put("start_at", start); put("end_at", end)
            put("zone_id", "America/New_York"); put("created_at", "2026-09-26T09:00:00Z")
        }

    @Test fun `an accepted time names its task and a real interval of at most a day`() {
        val parsed = parseSyncedPlacement("event_accepted_01", placement())
        assertEquals("task_synthetic", parsed.taskId)
        assertEquals(Instant.parse("2026-09-27T18:30:00Z"), parsed.start)
        validatePulledPayload("event_accepted_01", "placement", placement())
        // Another record's id, a reversed or day-long interval, or a title it must not carry.
        assertTrue(runCatching { parseSyncedPlacement("event_accepted_02", placement()) }.isFailure)
        assertTrue(runCatching { parseSyncedPlacement("event_accepted_01", placement(end = "2026-09-27T18:00:00Z")) }.isFailure)
        assertTrue(runCatching { parseSyncedPlacement("event_accepted_01", placement(end = "2026-09-28T19:00:00Z")) }.isFailure)
        assertTrue(runCatching { parseSyncedPlacement("event_accepted_01", JsonObject(placement() + ("title" to JsonPrimitive("private")))) }.isFailure)
        // Its erasure arrives as a tombstone naming the kind.
        validatePulledPayload("event_accepted_01", "tombstone", buildJsonObject { put("record_id", "event_accepted_01"); put("record_kind", "placement") })
    }

    @Test fun `plans are the accepted times of open tasks, titled from each task's latest revision`() {
        fun task(id: String, title: String, status: String) = SyncedTask(id, title, 30, status, 1, null, null, null)
        val tasks = listOf(task("task_open", "Paperwork (rescoped)", "open"), task("task_done", "Finished call", "done"))
        val placements = listOf(
            SyncedPlacement("event_late", "task_open", Instant.parse("2026-09-27T21:00:00Z"), Instant.parse("2026-09-27T21:30:00Z")),
            SyncedPlacement("event_early", "task_open", Instant.parse("2026-09-27T18:30:00Z"), Instant.parse("2026-09-27T20:00:00Z")),
            SyncedPlacement("event_done", "task_done", Instant.parse("2026-09-27T19:00:00Z"), Instant.parse("2026-09-27T19:30:00Z")),
            SyncedPlacement("event_orphan", "task_missing", Instant.parse("2026-09-27T19:00:00Z"), Instant.parse("2026-09-27T19:30:00Z")),
        )
        assertEquals(
            listOf("Paperwork (rescoped)" to Instant.parse("2026-09-27T18:30:00Z"), "Paperwork (rescoped)" to Instant.parse("2026-09-27T21:00:00Z")),
            plansFrom(placements, tasks).map { it.title to it.start },
        )
    }
}
