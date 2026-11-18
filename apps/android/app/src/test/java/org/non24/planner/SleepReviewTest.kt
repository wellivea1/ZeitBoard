package org.non24.planner

import java.io.File
import java.time.Instant
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.*
import org.junit.Assert.*
import org.junit.Test
import org.non24.planner.data.*

internal fun sleepReviewFixture(): JsonObject {
    val file = generateSequence(File(requireNotNull(System.getProperty("user.dir"))).absoluteFile) { it.parentFile }
        .map { File(it, "testdata/v1/sleep-review.json") }.first { it.isFile }
    return Json.parseToJsonElement(file.readText()).jsonObject
}

class SleepReviewTest {
    @Test fun `resolution retains reviewed heads source revision exact instants and exclusion`() {
        val review = parseSleepReview(sleepReviewFixture())
        assertTrue(review.needsReview)
        assertFalse(review.removedAtSource)
        assertNull(review.sourceOffset)
        assertEquals(2, review.edits.size)
        assertEquals("2026-03-04 23:30:00.123456789 -05:00", formatReviewInput(review.source.start, review.source.zoneId))
        val record = manualCorrection(review, "cor_resolution", review.generatedAt, review.effective.start, review.effective.end, "nap", true)
        val payload = Json.parseToJsonElement(record.payload).jsonObject
        assertEquals(review.sourceRevision, payload.instant("based_on_source_revision"))
        assertEquals(review.edits.map { it.id }, payload.getValue("supersedes_correction_ids").jsonArray.map { it.jsonPrimitive.content })
        val changes = payload.getValue("changes").jsonObject
        assertEquals(review.effective.start, changes.instant("start_at"))
        assertEquals(review.effective.end, changes.instant("end_at"))
        assertTrue(changes.boolean("excluded"))
        assertEquals("nap", changes.string("sleep_classification"))
    }

    @Test fun `malformed review does not become editable context`() {
        val valid = sleepReviewFixture()
        listOf(
            JsonObject(valid - "source_revision"),
            JsonObject(valid + ("needs_review" to JsonPrimitive("false"))),
            JsonObject(valid + ("schema_version" to JsonPrimitive("v99"))),
            JsonObject(valid + ("manual_corrections" to JsonArray(List(2) { valid.getValue("manual_corrections").jsonArray.first() }))),
            JsonObject(valid + ("unexpected_notes" to JsonPrimitive("synthetic private note"))),
            JsonObject(valid - "removed_at_source"),
            JsonObject(valid + ("source_offset" to buildJsonObject { put("start_seconds", 7200) })),
            JsonObject(valid + ("source_offset" to buildJsonObject { put("start_seconds", 90000); put("end_seconds", 0) })),
        ).forEach { assertTrue(runCatching { parseSleepReview(it) }.isFailure) }
    }

    // ADR-0072 and ADR-0073: the review says a night is gone from Health
    // Connect, and how far from home a night away was recorded.
    @Test fun `a review says a night is gone at its source and how far from home it was recorded`() {
        val valid = sleepReviewFixture()
        val review = parseSleepReview(JsonObject(valid + mapOf(
            "removed_at_source" to JsonPrimitive(true),
            "source_offset" to buildJsonObject { put("start_seconds", 3600); put("end_seconds", 3600) },
        )))
        assertTrue(review.removedAtSource)
        assertEquals(SourceOffset(3600, 3600), review.sourceOffset)
        // March 5 2026 in New York is UTC-5, so UTC+1 is six hours ahead.
        val zone = java.time.ZoneId.of("America/New_York")
        assertEquals("Recorded 6 hours ahead of home", recordedAway(review.sourceOffset, review.source.start, review.source.end, zone))
        assertEquals("Recorded 5 hours 30 minutes behind home",
            recordedAway(SourceOffset(-18000, -37800), review.source.start, review.source.end, zone))
        assertNull(recordedAway(null, review.source.start, review.source.end, zone))
    }

    @Test fun `cached offline review saves once without hiding a provider revision`() = runTest {
        val outbox = FakeOutbox()
        val replica = FakeReplica().apply { review = parseSleepReview(sleepReviewFixture()) }
        val repository = BackendSyncRepository(outbox, FakeConfigStore(SyncConfig("https://synthetic.test", "token", "UTC", "synthetic-scope")), FakeClient(), replica = replica)
        assertTrue(repository.loadSleepReview("obs_review").isFailure)
        val review = requireNotNull(repository.sleepReview.value.context)
        assertTrue(repository.saveSleepReview(review, review.effective.start, review.effective.end, "principal", false).isSuccess)
        assertEquals(1, outbox.pendingCount())
        assertTrue(outbox.knownSources().isEmpty())
        assertTrue(repository.sleepReview.value.pending)
        assertTrue(repository.saveSleepReview(review, review.effective.start, review.effective.end, "principal", false).isFailure)
        assertEquals(1, outbox.pendingCount())
    }

    @Test fun `re-enrollment cannot save the old server review to the new queue`() = runTest {
        val client = FakeClient().apply { reviewResponse = Result.success(sleepReviewFixture()) }
        val outbox = FakeOutbox()
        val repository = BackendSyncRepository(outbox, FakeConfigStore(SyncConfig("https://first.test", "token", "UTC", "synthetic-scope")), client, replica = FakeReplica())
        repository.loadSleepReview("obs_review").getOrThrow()
        val review = requireNotNull(repository.sleepReview.value.context)
        repository.enroll("https://second.test", "synthetic-secret", "UTC", "Synthetic phone").getOrThrow()
        assertNull(repository.sleepReview.value.context)
        assertTrue(repository.saveSleepReview(review, review.effective.start, review.effective.end, "principal", false).isFailure)
        assertEquals(0, outbox.pendingCount())
    }
}
