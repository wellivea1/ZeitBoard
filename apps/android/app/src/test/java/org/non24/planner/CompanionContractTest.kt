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
}
