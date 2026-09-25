package org.non24.planner.data

import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import kotlinx.serialization.json.*

data class SleepReviewWindow(val start: Instant, val end: Instant, val zoneId: String)
data class SleepReviewEdit(val id: String, val createdAt: Instant, val start: Instant?, val end: Instant?, val classification: String?, val excluded: Boolean?)
data class SleepReview(
    val observationId: String, val cursor: Long, val generatedAt: Instant, val sourceRevision: Instant,
    val source: SleepReviewWindow, val effective: SleepReviewWindow, val classification: String,
    val excluded: Boolean, val needsReview: Boolean, val edits: List<SleepReviewEdit>,
)
data class SleepReviewState(
    val selectedId: String? = null, val context: SleepReview? = null, val loading: Boolean = false,
    val stale: Boolean = false, val pending: Boolean = false, val error: String? = null,
)

fun formatReviewInput(instant: Instant, zoneId: String): String {
    val zoned = instant.atZone(ZoneId.of(zoneId))
    return DateTimeFormatter.ISO_LOCAL_DATE_TIME.format(zoned).replace('T', ' ') + " " + DateTimeFormatter.ofPattern("xxx").format(zoned)
}

internal fun JsonObject.boolean(key: String): Boolean =
    (get(key) as? JsonPrimitive)?.takeUnless { it.isString }?.booleanOrNull ?: error("Invalid boolean field.")

internal fun parseSleepReview(json: JsonObject): SleepReview {
    json.exactKeys(setOf("schema_version", "source_cursor", "generated_at", "observation_id", "source_revision", "source_window", "effective_window", "sleep_classification", "excluded", "needs_review", "manual_corrections"))
    require(json.string("schema_version") == "v1" && validSyncId(json.string("observation_id")))
    fun window(name: String): SleepReviewWindow {
        val value = json.getValue(name).jsonObject
        value.exactKeys(setOf("start_at", "end_at", "zone_id"))
        val start = value.instant("start_at"); val end = value.instant("end_at"); val zone = value.string("zone_id")
        require(end.isAfter(start)); ZoneId.of(zone)
        return SleepReviewWindow(start, end, zone)
    }
    val source = window("source_window"); val effective = window("effective_window")
    require(source.zoneId == effective.zoneId && json.integer("source_cursor") >= 0)
    val classification = json.string("sleep_classification")
    require(classification in setOf("principal", "nap", "unknown"))
    val heads = json.getValue("manual_corrections").jsonArray
    require(heads.size <= 256)
    val ids = HashSet<String>()
    val edits = heads.map { element ->
        val row = element.jsonObject
        row.exactKeys(setOf("correction_id", "created_at", "changes"))
        val id = row.string("correction_id")
        require(validSyncId(id) && ids.add(id))
        val changes = row.getValue("changes").jsonObject
        changes.exactKeys(emptySet(), setOf("start_at", "end_at", "sleep_classification", "excluded"))
        require(changes.isNotEmpty())
        val kind = if (changes.containsKey("sleep_classification")) changes.string("sleep_classification").also { require(it in setOf("principal", "nap", "unknown")) } else null
        SleepReviewEdit(id, row.instant("created_at"), changes.optionalInstant("start_at"), changes.optionalInstant("end_at"), kind,
            if (changes.containsKey("excluded")) changes.boolean("excluded") else null)
    }
    return SleepReview(json.string("observation_id"), json.integer("source_cursor"), json.instant("generated_at"), json.instant("source_revision"),
        source, effective, classification, json.boolean("excluded"), json.boolean("needs_review"), edits)
}

internal fun manualCorrection(review: SleepReview, id: String, at: Instant, start: Instant, end: Instant, classification: String, excluded: Boolean): OutboxRecord {
    return manualCorrection(review.observationId, review.sourceRevision, review.edits.map { it.id }, id, at, start, end, classification, excluded)
}

internal fun manualCorrection(observationId: String, sourceRevision: Instant, reviewedIds: List<String>, id: String, at: Instant, start: Instant, end: Instant, classification: String, excluded: Boolean): OutboxRecord {
    require(validSyncId(id) && end.isAfter(start) && classification in setOf("principal", "nap", "unknown"))
    require(validSyncId(observationId) && reviewedIds.size <= 256 && reviewedIds.distinct().size == reviewedIds.size && reviewedIds.all { validSyncId(it) && it != id })
    val payload = buildJsonObject {
        put("correction_id", id); put("target_observation_id", observationId)
        put("created_at", at.toString()); put("reason", "user_edit"); put("acquisition_method", "manual")
        put("based_on_source_revision", sourceRevision.toString())
        put("supersedes_correction_ids", buildJsonArray { reviewedIds.forEach { add(JsonPrimitive(it)) } })
        put("changes", buildJsonObject {
            put("start_at", start.toString()); put("end_at", end.toString())
            put("sleep_classification", classification); put("excluded", excluded)
        })
    }
    return OutboxRecord(id, "correction", at, sourceRevision, payload.toString())
}
