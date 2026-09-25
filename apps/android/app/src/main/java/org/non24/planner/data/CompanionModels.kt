package org.non24.planner.data

import java.time.Instant
import java.time.ZoneId
import kotlinx.serialization.json.*
import org.non24.planner.domain.*

const val SYNC_PULL_LIMIT = 100
data class PulledRecord(val seq: Long, val recordId: String, val kind: String, val payload: JsonObject)
data class PullPage(val cursor: Long, val records: List<PulledRecord>)
data class SyncedTask(
    val id: String, val title: String, val durationMinutes: Int, val status: String, val revision: Long,
    val earliestStart: Instant?, val deadline: Instant?, val preferredAfterWakeMinutes: Int?,
    val minimumConfidence: String? = null,
)
data class SyncedSleep(val id: String, val start: Instant, val end: Instant, val zoneId: String, val corrected: Boolean)
data class ServerForecast(val sleep: TimeWindow, val waking: TimeWindow, val zoneId: String)
data class CompanionProjection(
    val cursor: Long, val generatedAt: Instant, val validUntil: Instant, val algorithm: String,
    val status: String, val refusal: String?, val freshness: String, val freshnessExplanation: String,
    val confidence: Confidence?, val confidenceReasons: List<String>,
    val forecasts: List<ServerForecast>, val sleep: List<SyncedSleep>,
    val containsSyntheticData: Boolean = false,
) {
    fun isCurrentSnapshot(now: Instant): Boolean = now.isBefore(validUntil) && !generatedAt.isAfter(now.plusSeconds(60))
    fun estimate(now: Instant): EstimateSnapshot? {
        val next = forecasts.firstOrNull { it.waking.end.isAfter(now) } ?: return null
        val level = confidence ?: return null
        return EstimateSnapshot(
            label = if (containsSyntheticData) "Server sample forecast" else if (isCurrentSnapshot(now)) "Server forecast" else "Cached server forecast",
            predictedSleepWindow = next.sleep, predictedWakingWindow = next.waking,
            confidence = level, confidenceReasons = confidenceReasons,
            createdAt = generatedAt, algorithmVersion = algorithm,
            provenance = Provenance(AcquisitionMethod.SERVER, if (containsSyntheticData) EvidenceStatus.SYNTHETIC else EvidenceStatus.ESTIMATED, "self-hosted-server"),
        )
    }
}
data class CompanionState(
    val projection: CompanionProjection? = null,
    val tasks: List<SyncedTask> = emptyList(),
    val downloadedAt: Instant? = null,
    val error: String? = null,
    val totalTaskCount: Int = tasks.size,
    val sleepSources: List<SyncedSleep> = emptyList(),
    val totalSleepSourceCount: Int = sleepSources.size,
)

internal fun JsonObject.integer(key: String): Long =
    (get(key) as? JsonPrimitive)?.takeUnless { it.isString }?.longOrNull ?: error("Invalid integer field.")
internal fun JsonObject.instant(key: String): Instant = Instant.parse(string(key))
internal fun JsonObject.optionalInstant(key: String): Instant? = if (containsKey(key)) instant(key) else null
internal fun validSyncId(id: String): Boolean = Regex("^[a-z][a-z0-9_-]{2,63}$").matches(id)
internal fun JsonObject.exactKeys(required: Set<String>, optional: Set<String> = emptySet()) {
    require(keys.containsAll(required) && (keys - required - optional).isEmpty())
}
internal fun JsonObject.boundedString(key: String, max: Int, min: Int = 1): String =
    string(key).also { require(it.length in min..max) }

internal fun parsePullPage(json: JsonObject, since: Long): PullPage {
    json.exactKeys(setOf("schema_version", "cursor", "records"))
    require(json.string("schema_version") == SyncContract.SCHEMA_VERSION)
    val rows = json.getValue("records").jsonArray
    require(rows.size <= SYNC_PULL_LIMIT)
    var previous = since
    val seen = HashSet<String>()
    val records = rows.map { element ->
        val row = element.jsonObject
        row.exactKeys(setOf("seq", "recordId", "kind", "deviceId", "createdAt", "payload"))
        val seq = row.integer("seq")
        val id = row.string("recordId")
        require(seq > previous && validSyncId(id) && seen.add(id))
        previous = seq
        row.instant("createdAt")
        require(validSyncId(row.string("deviceId")))
        val kind = row.string("kind")
        val payload = row.getValue("payload").jsonObject
        validatePulledPayload(id, kind, payload)
        PulledRecord(seq, id, kind, payload)
    }
    require(json.integer("cursor") == previous)
    return PullPage(previous, records)
}

internal fun validatePulledPayload(id: String, kind: String, payload: JsonObject) {
    when (kind) {
        "observation" -> {
            require(payload.string("observation_id") == id)
            require(payload.instant("end_at").isAfter(payload.instant("start_at")))
            ZoneId.of(payload.string("zone_id"))
            payload.getValue("provenance").jsonObject.instant("recorded_at")
        }
        "correction" -> {
            require(payload.string("correction_id") == id && validSyncId(payload.string("target_observation_id")))
            payload.instant("created_at")
            val changes = payload.getValue("changes").jsonObject
            require(changes.isNotEmpty())
            changes["start_at"]?.let { Instant.parse(it.jsonPrimitive.content) }
            changes["end_at"]?.let { Instant.parse(it.jsonPrimitive.content) }
        }
        "task" -> parseSyncedTask(id, payload)
        "tombstone" -> {
            payload.exactKeys(setOf("record_id"), setOf("record_kind"))
            require(payload.string("record_id") == id)
            if (payload.containsKey("record_kind")) require(payload.string("record_kind") in setOf("observation", "correction", "task"))
        }
        else -> error("Unsupported sync record kind.")
    }
}

internal fun parseSyncedTask(recordId: String, payload: JsonObject): SyncedTask {
    payload.exactKeys(setOf("task_id", "revision", "title", "duration_minutes", "status", "created_at", "updated_at"),
        setOf("earliest_start_at", "latest_finish_at", "preferred_after_wake_minutes", "minimum_confidence"))
    val id = payload.string("task_id")
    val revision = payload.integer("revision")
    require(validSyncId(id) && revision >= 1 && recordId == "${id}_r$revision")
    val title = payload.string("title")
    val duration = payload.integer("duration_minutes")
    val status = payload.string("status")
    require(title.isNotBlank() && title.length <= 120 && duration in 5..720 && status in setOf("open", "done"))
    payload.instant("created_at")
    payload.instant("updated_at")
    val earliest = payload.optionalInstant("earliest_start_at")
    val deadline = payload.optionalInstant("latest_finish_at")
    require(earliest == null || deadline == null || deadline.isAfter(earliest))
    val minimumConfidence = if (payload.containsKey("minimum_confidence")) payload.string("minimum_confidence").also {
        require(it in setOf("low", "medium", "high"))
    } else null
    val afterWake = if (payload.containsKey("preferred_after_wake_minutes")) payload.integer("preferred_after_wake_minutes").also { require(it in 0..1440) }.toInt() else null
    return SyncedTask(id, title, duration.toInt(), status, revision, earliest, deadline, afterWake, minimumConfidence)
}

internal fun parseCompanion(json: JsonObject): CompanionProjection {
    json.exactKeys(setOf("schema_version", "source_cursor", "generated_at", "valid_until", "algorithm_version", "provenance",
        "status", "freshness", "forecasts", "sleep", "contains_synthetic_data"), setOf("refusal", "confidence"))
    require(json.string("schema_version") == "v2" && json.string("provenance") == "self_hosted_synced_sleep")
    val generated = json.instant("generated_at")
    val expires = json.instant("valid_until")
    require(expires.isAfter(generated) && !expires.isAfter(generated.plusSeconds(15 * 60)))
    val status = json.string("status")
    require(status in setOf("estimated", "refused"))
    val fresh = json.getValue("freshness").jsonObject
    fresh.exactKeys(setOf("state", "reason", "explanation"))
    fresh.boundedString("reason", 64, 0)
    val freshness = fresh.string("state")
    require(freshness in setOf("current", "stale", "withheld"))
    fun window(value: JsonElement): Pair<TimeWindow, String> {
        val obj = value.jsonObject
        obj.exactKeys(setOf("start_at", "end_at", "zone_id"))
        val zone = ZoneId.of(obj.string("zone_id")).id
        return TimeWindow(obj.instant("start_at"), obj.instant("end_at")) to zone
    }
    val forecastRows = json.getValue("forecasts").jsonArray
    val sleepRows = json.getValue("sleep").jsonArray
    require(forecastRows.size <= 14 && sleepRows.size <= 256)
    val forecasts = forecastRows.map {
        val obj = it.jsonObject
        obj.exactKeys(setOf("predicted_sleep_window", "predicted_waking_window"))
        val (sleep, zone) = window(obj.getValue("predicted_sleep_window"))
        val (wake, wakeZone) = window(obj.getValue("predicted_waking_window"))
        require(zone == wakeZone)
        ServerForecast(sleep, wake, zone)
    }
    val confidence = json["confidence"]?.jsonObject
    confidence?.exactKeys(setOf("level", "reasons"))
    val reasons = confidence?.getValue("reasons")?.jsonArray?.also { require(it.size <= 32) }?.map {
        val value = it.jsonPrimitive
        require(value.isString && value.content.length in 1..240)
        value.content
    }.orEmpty()
    val refusal = json["refusal"]?.jsonObject?.let {
        it.exactKeys(setOf("code", "message"))
        require(it.string("code") in setOf("insufficient_data", "ambiguous_cycle_index", "conflicting_observations", "unsupported_input", "correction_review_required", "history_limit"))
        it.boundedString("message", 240)
    }
    val level = confidence?.string("level")?.let {
        when (it) { "low" -> Confidence.LOW; "medium" -> Confidence.MODERATE; "high" -> Confidence.HIGH; else -> error("Unsupported confidence.") }
    }
    require(if (status == "estimated") level != null && forecasts.isNotEmpty() && !json.containsKey("refusal")
        else level == null && forecasts.isEmpty() && json.containsKey("refusal"))
    return CompanionProjection(
        json.integer("source_cursor").also { require(it >= 0) }, generated, expires, json.boundedString("algorithm_version", 64),
        status, refusal, freshness, fresh.boundedString("explanation", 400),
        level, reasons, forecasts,
        sleepRows.map {
            val obj = it.jsonObject
            obj.exactKeys(setOf("row_id", "start_at", "end_at", "zone_id", "classification", "corrected"))
            require(obj.string("classification") in setOf("principal", "nap", "unknown"))
            val start = obj.instant("start_at"); val end = obj.instant("end_at")
            require(end.isAfter(start))
            SyncedSleep(obj.boundedString("row_id", 240), start, end, ZoneId.of(obj.string("zone_id")).id,
                obj.getValue("corrected").jsonPrimitive.boolean)
        },
        json.getValue("contains_synthetic_data").jsonPrimitive.boolean,
    )
}
