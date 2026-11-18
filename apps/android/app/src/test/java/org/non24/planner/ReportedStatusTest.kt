package org.non24.planner

import java.time.Instant
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test
import org.non24.planner.data.CompanionStatus
import org.non24.planner.data.StatusReporting
import org.non24.planner.data.newestStatus
import org.non24.planner.data.parseSyncedStatus
import org.non24.planner.data.statusRecord
import org.non24.planner.data.validatePulledPayload

/** A status the owner reports (ADR-0070), and the records the phone must accept to keep syncing. */
class ReportedStatusTest {
    private val noon = Instant.parse("2026-11-14T12:00:00Z")

    private fun payload(text: String) = Json.parseToJsonElement(text).jsonObject

    @Test
    fun aReportMadeHereIsTheRecordTheServerAccepts() {
        val record = statusRecord("status-0a1b2c3d", StatusReporting.TRYING_TO_SLEEP, noon, shared = true)
        assertEquals("status", record.kind)
        val sent = payload(record.payload)
        // The same fields, and no others, as contracts/v1 names them.
        assertEquals(setOf("status_id", "status", "reported_at", "expires_at", "shared"), sent.keys)
        val parsed = parseSyncedStatus(record.recordId, sent, pending = true)
        assertEquals(noon.plus(StatusReporting.defaultLifetime(StatusReporting.TRYING_TO_SLEEP)), parsed.expiresAt)
        assertTrue(parsed.shared && parsed.pending)
    }

    @Test
    fun aReportLastsAsLongAsTheOwnerChose() {
        val hour = statusRecord("status-0a1b2c3e", StatusReporting.TRYING_TO_SLEEP, noon, shared = false, minutes = 60)
        assertEquals(noon.plusSeconds(3600), parseSyncedStatus(hour.recordId, payload(hour.payload), pending = true).expiresAt)
        // Zero is the usual length, and the choices offered are all within a day.
        assertEquals(listOf("Usual", "1 h", "2 h", "4 h", "8 h"), StatusReporting.LIFETIME_CHOICES.map(StatusReporting::lifetimeWords))
        assertThrows(Exception::class.java) { statusRecord("status-0a1b2c3f", StatusReporting.AWAKE, noon, shared = false, minutes = 1441) }
        assertThrows(Exception::class.java) { statusRecord("status-0a1b2c40", StatusReporting.AWAKE, noon, shared = false, minutes = 3) }
        // Taking a status back lasts no time.
        assertThrows(Exception::class.java) { statusRecord("status-0a1b2c41", StatusReporting.CLEARED, noon, shared = false, minutes = 60) }
    }

    @Test
    fun takingAStatusBackSharesNothingAndHasNoExpiry() {
        val record = statusRecord("status-cleared1", StatusReporting.CLEARED, noon, shared = true)
        val sent = payload(record.payload)
        assertEquals(setOf("status_id", "status", "reported_at", "shared"), sent.keys)
        val parsed = parseSyncedStatus(record.recordId, sent, pending = true)
        assertFalse(parsed.shared)
        assertNull(parsed.expiresAt)
        assertFalse(parsed.standsAt(noon.plusSeconds(1)))
    }

    @Test
    fun theNewestReportStandsUntilItLapses() {
        fun report(id: String, status: String, at: Instant) =
            CompanionStatus(id, status, at, at.plusSeconds(3600), shared = false, pending = false)
        val older = report("status-a", StatusReporting.AWAKE, noon.minusSeconds(600))
        val newer = report("status-b", StatusReporting.DO_NOT_DISTURB, noon.minusSeconds(60))
        val newest = newestStatus(listOf(newer, older))
        assertEquals("status-b", newest?.statusId)
        assertTrue(newest!!.standsAt(noon))
        // At its expiry it no longer stands, and one dated ahead of the clock is not believed early.
        assertFalse(newest.standsAt(noon.minusSeconds(60).plusSeconds(3600)))
        assertFalse(report("status-c", StatusReporting.AWAKE, noon.plusSeconds(60)).standsAt(noon))
        assertNull(newestStatus(emptyList()))
    }

    @Test
    fun aStatusOutOfTheContractIsRefused() {
        val base = """{"status_id":"status-x1","status":"awake","reported_at":"2026-11-14T12:00:00Z","expires_at":"2026-11-14T16:00:00Z","shared":true}"""
        validatePulledPayload("status-x1", "status", payload(base))
        listOf(
            base.replace("\"awake\"", "\"asleep\""),
            base.replace(",\"expires_at\":\"2026-11-14T16:00:00Z\"", ""),
            base.replace("2026-11-14T16:00:00Z", "2026-11-16T12:00:00Z"),
            base.replace("\"shared\":true", "\"shared\":true,\"note\":\"x\""),
        ).forEach { bad ->
            assertThrows(Exception::class.java) { validatePulledPayload("status-x1", "status", payload(bad)) }
        }
    }

    /**
     * Events the owner adds on their computer sync too (ADR-0068). A phone
     * that refused them would stop downloading everything after the first one.
     */
    @Test
    fun anEventFromTheComputerIsAcceptedAndABrokenOneIsNot() {
        val event = """{"event_id":"event_0a1b","title":"Dentist (synthetic)","start_at":"2026-11-14T20:00:00Z","end_at":"2026-11-14T21:00:00Z","zone_id":"America/New_York","revision":2,"created_at":"2026-11-13T10:00:00Z","updated_at":"2026-11-13T11:00:00Z"}"""
        validatePulledPayload("event_0a1b_r2", "event", payload(event))
        // The record's id must name the revision it carries.
        assertThrows(Exception::class.java) { validatePulledPayload("event_0a1b_r1", "event", payload(event)) }
        assertThrows(Exception::class.java) {
            validatePulledPayload("event_0a1b_r2", "event", payload(event.replace("2026-11-14T21:00:00Z", "2026-11-14T19:00:00Z")))
        }
        // A tombstone may name either new kind.
        validatePulledPayload("event_0a1b_r2", "tombstone", payload("""{"record_id":"event_0a1b_r2","record_kind":"event"}"""))
        validatePulledPayload("status-x1", "tombstone", payload("""{"record_id":"status-x1","record_kind":"status"}"""))
    }
}
