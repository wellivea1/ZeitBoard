package org.non24.planner

import java.time.Instant
import java.time.ZoneId
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.*
import org.junit.Assert.*
import org.junit.Test
import org.non24.planner.data.*
import org.non24.planner.ui.lastDoseText
import org.non24.planner.ui.usualTimes

class DoseLoggingTest {
    private val zone = ZoneId.of("America/New_York")
    private val now = Instant.parse("2026-09-27T14:00:00Z") // 10:00 AM in New York
    private val evening = CompanionMedication("med_synthetic", "Synthetic evening tablet", "fixed_clock", listOf("22:00"), "America/New_York", null)

    @Test fun `a tap records one immutable dose now, in the home zone, never marked scheduled`() {
        val record = doseRecord("dose-synthetic01", evening, "taken", Instant.parse("2026-09-27T14:00:07.123Z"), zone)
        assertEquals("medication_event", record.kind)
        assertEquals("dose-synthetic01", record.recordId)
        val payload = Json.parseToJsonElement(record.payload).jsonObject
        val dose = parseSyncedDose(record.recordId, payload)
        assertEquals(Instant.parse("2026-09-27T14:00:07Z"), dose.doseAt)
        assertEquals("America/New_York", dose.zoneId)
        // Even for a medication with a fixed schedule: only an explicit mark
        // makes a dose count in adherence (ADR-0027).
        assertFalse(dose.scheduled)
        assertEquals("user_reported", payload.getValue("provenance").jsonObject.getValue("evidence_status").jsonPrimitive.content)
        assertTrue(runCatching { doseRecord("dose-synthetic03", evening, "maybe", now, zone) }.isFailure)
    }

    @Test fun `corrections apply in the order they were made, and an exclusion hides the dose`() {
        val dose = SyncedDose("dose_synthetic", "med_synthetic", now, "UTC", "taken", true)
        fun correction(id: String, at: String, changes: JsonObject) = buildJsonObject {
            put("correction_id", id); put("target_event_id", "dose_synthetic"); put("created_at", at)
            put("reason", "user_edit"); put("changes", changes)
        }
        val later = correction("medcor_b", "2026-09-27T15:00:00Z", buildJsonObject { put("status", "taken") })
        val earlier = correction("medcor_a", "2026-09-27T14:30:00Z", buildJsonObject { put("status", "skipped"); put("dose_at", "2026-09-27T13:00:00Z") })
        val effective = effectiveDose(dose, listOf(later, earlier))!!
        assertEquals("taken", effective.status)
        assertEquals(Instant.parse("2026-09-27T13:00:00Z"), effective.doseAt)
        val excluded = correction("medcor_c", "2026-09-27T16:00:00Z", buildJsonObject { put("excluded", true) })
        assertNull(effectiveDose(dose, listOf(later, earlier, excluded)))
    }

    @Test fun `the words say the usual time and the latest dose as the desktop sketches them`() {
        assertEquals("Usually 10:00 PM", usualTimes(evening, zone, use24HourTime = false))
        assertEquals("Usually 22:00", usualTimes(evening, zone, use24HourTime = true))
        assertEquals("Usually 10:00 PM New York time", usualTimes(evening, ZoneId.of("Europe/Lisbon"), use24HourTime = false))
        assertEquals("Usually 21:00 UTC", usualTimes(evening.copy(civilTimes = listOf("21:00"), scheduleZoneId = "UTC"), zone, use24HourTime = true))
        assertEquals("As needed", usualTimes(evening.copy(scheduleKind = "as_needed", civilTimes = emptyList()), zone, false))
        assertNull(usualTimes(evening.copy(scheduleKind = null), zone, false))

        val yesterday = CompanionDose("dose_a", "taken", Instant.parse("2026-09-27T02:05:00Z"), "America/New_York", pending = false)
        assertEquals("Last taken yesterday at 10:05 PM", lastDoseText(yesterday, now, zone, false))
        val justNow = CompanionDose("dose_b", "skipped", Instant.parse("2026-09-27T13:55:00Z"), "America/New_York", pending = true)
        assertEquals("Last skipped today at 9:55 AM, uploading", lastDoseText(justNow, now, zone, false))
        assertEquals("Nothing recorded yet", lastDoseText(null, now, zone, false))
    }

    @Test fun `a tap queues one dose for a medication on the computer's list`() = runTest {
        val outbox = FakeOutbox()
        val replica = FakeReplica().apply { medications = listOf(evening) }
        val repository = BackendSyncRepository(outbox, FakeConfigStore(SyncConfig("https://host.test", "t", "America/New_York", "synthetic-scope")),
            FakeClient(), now = { now }, replica = replica)
        repository.logDose("med_synthetic", "taken").getOrThrow()
        val queued = outbox.pending(10).single()
        val dose = parseSyncedDose(queued.recordId, Json.parseToJsonElement(queued.payload).jsonObject)
        assertEquals("med_synthetic", dose.medicationId)
        assertEquals(now, dose.doseAt)
        assertEquals("taken", dose.status)
        // A medication no longer on the list records nothing.
        assertTrue(repository.logDose("med_deleted", "taken").isFailure)
        assertEquals(1, outbox.pending(10).size)
    }
}
