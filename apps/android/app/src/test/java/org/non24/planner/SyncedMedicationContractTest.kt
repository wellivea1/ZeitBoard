package org.non24.planner

import java.io.File
import kotlinx.serialization.json.*
import org.junit.Assert.*
import org.junit.Test
import org.non24.planner.data.*

private fun contractFixture(path: String): JsonObject {
    val file = generateSequence(File(requireNotNull(System.getProperty("user.dir"))).absoluteFile) { it.parentFile }
        .map { File(it, "testdata/$path") }.first { it.isFile }
    return Json.parseToJsonElement(file.readText()).jsonObject
}

private fun JsonObject.with(key: String, value: JsonElement) = JsonObject(this + (key to value))
private fun JsonObject.text(key: String) = getValue(key).jsonPrimitive.content
private fun fails(block: () -> Unit) = assertTrue(runCatching(block).isFailure)

/**
 * The companion reads medication definitions, doses, dose corrections and
 * context markers exactly as the contracts define them (ADR-0048). The records
 * here are the contracts' own schema-validated fixtures, the shapes the
 * desktop uploads.
 */
class SyncedMedicationContractTest {
    private val medication = contractFixture("v2/medication-set.json").getValue("medications").jsonArray[0].jsonObject
    private val events = contractFixture("v2/medication-event-set.json")
    private val dose = events.getValue("events").jsonArray[0].jsonObject
    private val correction = events.getValue("corrections").jsonArray[0].jsonObject
    private val marker = contractFixture("v1/rhythm-marker-set.json").getValue("markers").jsonArray[0].jsonObject
    private val revisionId = "${medication.text("medication_id")}_r${medication.getValue("revision").jsonPrimitive.long}"

    @Test fun `the contracts' medication records parse as the desktop uploads them`() {
        val parsed = parseSyncedMedication(revisionId, medication)
        assertEquals(medication.text("medication_id"), parsed.medicationId)
        assertEquals(medication.text("label"), parsed.label)
        val parsedDose = parseSyncedDose(dose.text("event_id"), dose)
        assertEquals(dose.text("medication_id"), parsedDose.medicationId)
        assertEquals(dose.text("status"), parsedDose.status)
        validatePulledPayload(revisionId, "medication", medication)
        validatePulledPayload(dose.text("event_id"), "medication_event", dose)
        validatePulledPayload(correction.text("correction_id"), "medication_correction", correction)
        validatePulledPayload(marker.text("marker_id"), "context_marker", marker)
        for (kind in listOf("medication", "medication_event", "medication_correction", "context_marker")) {
            validatePulledPayload("record_synthetic", "tombstone", buildJsonObject { put("record_id", "record_synthetic"); put("record_kind", kind) })
        }
    }

    @Test fun `a page carrying every new kind parses in order`() {
        fun row(seq: Int, id: String, kind: String, payload: JsonObject) = buildJsonObject {
            put("seq", seq); put("recordId", id); put("kind", kind); put("deviceId", "device_synthetic")
            put("createdAt", "2026-09-02T12:00:00Z"); put("payload", payload)
        }
        val page = buildJsonObject {
            put("schema_version", "v1"); put("cursor", 5)
            put("records", JsonArray(listOf(
                row(1, revisionId, "medication", medication),
                row(2, dose.text("event_id"), "medication_event", dose),
                row(3, correction.text("correction_id"), "medication_correction", correction),
                row(4, marker.text("marker_id"), "context_marker", marker),
                row(5, dose.text("event_id") + "_x", "tombstone", buildJsonObject {
                    put("record_id", dose.text("event_id") + "_x"); put("record_kind", "medication_event")
                }),
            )))
        }
        assertEquals(listOf("medication", "medication_event", "medication_correction", "context_marker", "tombstone"),
            parsePullPage(page, 0).records.map { it.kind })
    }

    @Test fun `a record the companion cannot read fails instead of being kept`() {
        // Another revision's id, and a schedule kind the contract does not know.
        fails { parseSyncedMedication("${medication.text("medication_id")}_r9", medication) }
        fails { parseSyncedMedication(revisionId, medication.with("schedule", buildJsonObject { put("kind", "whenever"); put("reminder_enabled", false) })) }
        // A dose with an unknown status or a field the contract does not carry.
        fails { parseSyncedDose(dose.text("event_id"), dose.with("status", JsonPrimitive("maybe"))) }
        fails { parseSyncedDose(dose.text("event_id"), dose.with("title", JsonPrimitive("private"))) }
        // A correction that changes nothing; a marker of an unknown kind.
        fails { validateSyncedDoseCorrection(correction.text("correction_id"), correction.with("changes", buildJsonObject {})) }
        fails { validateSyncedMarker(marker.text("marker_id"), marker.with("kind", JsonPrimitive("holiday"))) }
        // A tombstone naming a kind that does not sync.
        fails { validatePulledPayload("record_synthetic", "tombstone", buildJsonObject { put("record_id", "record_synthetic"); put("record_kind", "calendar") }) }
    }
}
