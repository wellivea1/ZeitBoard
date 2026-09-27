package org.non24.planner.data

import java.time.Instant
import java.time.ZoneId
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

// Medication definitions, doses, dose corrections and context markers synced
// from the owner's computers (ADR-0048). Each record is checked exactly as its
// contract defines it before it is kept: a record the companion cannot read
// fails the page rather than being stored half-understood.

/** One revision of a medication definition; the highest revision is current. */
data class SyncedMedication(
    val medicationId: String,
    val revision: Long,
    val label: String,
    val active: Boolean,
    val scheduleKind: String?,
    val civilTimes: List<String>,
    val scheduleZoneId: String?,
)

/** One recorded dose, taken or skipped. */
data class SyncedDose(
    val eventId: String,
    val medicationId: String,
    val doseAt: Instant,
    val zoneId: String,
    val status: String,
    val scheduled: Boolean,
)

internal val MEDICATION_SCHEDULE_KINDS = setOf("as_needed", "fixed_clock", "cycling")
internal val DOSE_STATUSES = setOf("taken", "skipped")
internal val MARKER_KINDS = setOf("travel", "illness", "disruption", "forced_schedule")
private val CIVIL_TIME = Regex("^(?:[01][0-9]|2[0-3]):[0-5][0-9]$")
private val CIVIL_DATE = Regex("^[0-9]{4}-[0-9]{2}-[0-9]{2}$")

private fun JsonObject.privateText(key: String, max: Int) {
    if (containsKey(key)) string(key).also { require(it.length <= max && it == it.trim()) }
}

private fun JsonObject.provenance() {
    getValue("provenance").jsonObject.also {
        it.exactKeys(setOf("acquisition_method", "evidence_status", "recorded_at"), setOf("source_record_id"))
        it.instant("recorded_at")
    }
}

internal fun parseSyncedMedication(recordId: String, payload: JsonObject): SyncedMedication {
    payload.exactKeys(
        setOf("medication_id", "label", "active", "created_at", "revision", "updated_at"),
        setOf("form", "strength_label", "clinician_rule", "started_at", "started_zone_id", "schedule"),
    )
    val id = payload.string("medication_id")
    val revision = payload.integer("revision")
    require(validSyncId(id) && revision >= 1 && recordId == "${id}_r$revision")
    val label = payload.string("label")
    require(label.isNotBlank() && label.length <= 120 && label == label.trim())
    payload.privateText("form", 80)
    payload.privateText("strength_label", 80)
    payload.privateText("clinician_rule", 500)
    val active = payload.boolean("active")
    require(!payload.instant("updated_at").isBefore(payload.instant("created_at")))
    require(payload.containsKey("started_at") == payload.containsKey("started_zone_id"))
    if (payload.containsKey("started_at")) {
        payload.instant("started_at")
        ZoneId.of(payload.string("started_zone_id"))
    }
    val schedule = payload["schedule"]?.jsonObject
        ?: return SyncedMedication(id, revision, label, active, null, emptyList(), null)
    schedule.exactKeys(setOf("kind", "reminder_enabled"), setOf("zone_id", "civil_times", "days_on", "days_off", "cycle_started_on"))
    val kind = schedule.string("kind")
    require(kind in MEDICATION_SCHEDULE_KINDS)
    schedule.boolean("reminder_enabled")
    val zone = if (schedule.containsKey("zone_id")) schedule.string("zone_id").also { ZoneId.of(it) } else null
    val times = schedule["civil_times"]?.jsonArray?.map { it.jsonPrimitive.content.also { time -> require(CIVIL_TIME.matches(time)) } }
        ?: emptyList()
    if (schedule.containsKey("cycle_started_on")) require(CIVIL_DATE.matches(schedule.string("cycle_started_on")))
    return SyncedMedication(id, revision, label, active, kind, times, zone)
}

internal fun parseSyncedDose(recordId: String, payload: JsonObject): SyncedDose {
    payload.exactKeys(setOf("event_id", "medication_id", "dose_at", "zone_id", "status", "scheduled", "provenance"), setOf("note"))
    require(validSyncId(recordId) && payload.string("event_id") == recordId)
    val medicationId = payload.string("medication_id")
    require(validSyncId(medicationId))
    val status = payload.string("status")
    require(status in DOSE_STATUSES)
    val zone = payload.string("zone_id").also { ZoneId.of(it) }
    payload.privateText("note", 500)
    payload.provenance()
    return SyncedDose(recordId, medicationId, payload.instant("dose_at"), zone, status, payload.boolean("scheduled"))
}

internal fun validateSyncedDoseCorrection(recordId: String, payload: JsonObject) {
    payload.exactKeys(setOf("correction_id", "target_event_id", "created_at", "reason", "changes"), setOf("supersedes_correction_id"))
    require(payload.string("correction_id") == recordId && validSyncId(payload.string("target_event_id")))
    if (payload.containsKey("supersedes_correction_id")) require(validSyncId(payload.string("supersedes_correction_id")))
    payload.instant("created_at")
    require(payload.string("reason") in setOf("user_edit", "duplicate", "invalid_time"))
    val changes = payload.getValue("changes").jsonObject
    changes.exactKeys(emptySet(), setOf("dose_at", "zone_id", "status", "scheduled", "note", "excluded"))
    require(changes.isNotEmpty())
    if (changes.containsKey("dose_at")) changes.instant("dose_at")
    if (changes.containsKey("zone_id")) ZoneId.of(changes.string("zone_id"))
    if (changes.containsKey("status")) require(changes.string("status") in DOSE_STATUSES)
    if (changes.containsKey("scheduled")) changes.boolean("scheduled")
    if (changes.containsKey("excluded")) changes.boolean("excluded")
    changes.privateText("note", 500)
}

internal fun validateSyncedMarker(recordId: String, payload: JsonObject) {
    payload.exactKeys(setOf("marker_id", "kind", "start_at", "zone_id", "provenance"), setOf("end_at", "note"))
    require(validSyncId(recordId) && payload.string("marker_id") == recordId)
    require(payload.string("kind") in MARKER_KINDS)
    val start = payload.instant("start_at")
    if (payload.containsKey("end_at")) require(payload.instant("end_at").isAfter(start))
    ZoneId.of(payload.string("zone_id"))
    payload.privateText("note", 500)
    payload.provenance()
}
