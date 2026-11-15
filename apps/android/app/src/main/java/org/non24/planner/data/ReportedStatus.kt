package org.non24.planner.data

import java.time.Duration
import java.time.Instant
import java.time.ZoneId
import java.time.temporal.ChronoUnit
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

/**
 * A status the owner reported (ADR-0070): awake, trying to sleep, or do not
 * disturb, with when it was said and when it stops standing. It is the
 * owner's word. It is never a sleep record, and nothing here estimates
 * anything from it.
 */
object StatusReporting {
    const val KIND = "status"

    const val AWAKE = "awake"
    const val TRYING_TO_SLEEP = "trying_to_sleep"
    const val DO_NOT_DISTURB = "do_not_disturb"

    /** Takes back whatever was reported before it. */
    const val CLEARED = "cleared"

    val STATUSES = setOf(AWAKE, TRYING_TO_SLEEP, DO_NOT_DISTURB, CLEARED)

    /** The longest a report may stand. */
    val MAX_LIFETIME: Duration = Duration.ofHours(24)

    /** How long a report stands when the owner does not say, as the desktop has it. */
    fun defaultLifetime(status: String): Duration = when (status) {
        TRYING_TO_SLEEP -> Duration.ofHours(10)
        DO_NOT_DISTURB -> Duration.ofHours(2)
        else -> Duration.ofHours(4)
    }

    fun words(status: String): String = when (status) {
        AWAKE -> "Awake"
        TRYING_TO_SLEEP -> "Trying to sleep"
        DO_NOT_DISTURB -> "Do not disturb"
        else -> ""
    }
}

/** The newest report this phone knows of, downloaded or still waiting to upload. */
data class CompanionStatus(
    val statusId: String,
    val status: String,
    val reportedAt: Instant,
    /** Absent only for a cleared status. */
    val expiresAt: Instant?,
    val shared: Boolean,
    /** True until the owner's server has the report. */
    val pending: Boolean,
) {
    /** Whether the report stands at [now]: not cleared, not lapsed, and not dated ahead of the clock. */
    fun standsAt(now: Instant): Boolean =
        status != StatusReporting.CLEARED && expiresAt != null && now.isBefore(expiresAt) && !reportedAt.isAfter(now)
}

internal fun parseSyncedStatus(recordId: String, payload: JsonObject, pending: Boolean): CompanionStatus {
    payload.exactKeys(setOf("status_id", "status", "reported_at", "shared"), setOf("expires_at"))
    require(validSyncId(recordId) && payload.string("status_id") == recordId)
    val status = payload.string("status")
    require(status in StatusReporting.STATUSES)
    val reportedAt = payload.instant("reported_at")
    val expiresAt = payload.optionalInstant("expires_at")
    val shared = payload.boolean("shared")
    if (status == StatusReporting.CLEARED) {
        require(expiresAt == null && !shared)
    } else {
        requireNotNull(expiresAt)
        require(expiresAt.isAfter(reportedAt) && !Duration.between(reportedAt, expiresAt).minus(StatusReporting.MAX_LIFETIME).isPositive)
    }
    return CompanionStatus(recordId, status, reportedAt, expiresAt, shared, pending)
}

internal fun validateSyncedStatus(recordId: String, payload: JsonObject) {
    parseSyncedStatus(recordId, payload, pending = false)
}

/**
 * An event the owner added on their computer (ADR-0068). The phone keeps it
 * as sent and does not show it yet; refusing it would stop every later record
 * from arriving.
 */
internal fun validateSyncedEvent(recordId: String, payload: JsonObject) {
    payload.exactKeys(setOf("event_id", "title", "start_at", "end_at", "zone_id", "revision", "created_at", "updated_at"))
    val eventId = payload.string("event_id")
    val revision = payload.integer("revision")
    require(validSyncId(eventId) && revision >= 1 && recordId == "${eventId}_r$revision")
    payload.boundedString("title", 120)
    val start = payload.instant("start_at")
    val end = payload.instant("end_at")
    require(end.isAfter(start) && !Duration.between(start, end).minus(Duration.ofHours(24)).isPositive)
    ZoneId.of(payload.string("zone_id"))
    payload.instant("created_at")
    payload.instant("updated_at")
}

/** The report that is newest: by when it was said, then by id, as every device orders them. */
internal fun newestStatus(reports: List<CompanionStatus>): CompanionStatus? =
    reports.maxWithOrNull(compareBy<CompanionStatus>({ it.reportedAt }, { it.statusId }))

/** A report made on this phone, ready to queue for the owner's server. */
internal fun statusRecord(statusId: String, status: String, at: Instant, shared: Boolean): OutboxRecord {
    require(validSyncId(statusId) && status in StatusReporting.STATUSES)
    val reported = at.truncatedTo(ChronoUnit.MILLIS)
    val payload = buildJsonObject {
        put("status_id", statusId)
        put("status", status)
        put("reported_at", reported.toString())
        if (status != StatusReporting.CLEARED) {
            put("expires_at", reported.plus(StatusReporting.defaultLifetime(status)).toString())
        }
        put("shared", status != StatusReporting.CLEARED && shared)
    }
    parseSyncedStatus(statusId, payload, pending = true)
    return OutboxRecord(recordId = statusId, kind = StatusReporting.KIND, createdAt = reported, sourceRevision = reported, payload = payload.toString())
}
