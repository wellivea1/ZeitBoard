package org.non24.planner.data

import java.time.Duration
import java.time.Instant
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.Json

/**
 * What one check of Health Connect found (ADR-0054). The desktop words it
 * where the owner judges their data: why the newest record is old, and what
 * would bring the next one in.
 */
enum class SourceOutcome(val wire: String) {
    /** Read in full over [SourceCheck.covered]. */
    READ("read"),

    /** The phone may not read sleep. */
    PERMISSION_MISSING("permission_missing"),

    /** Health Connect is not installed, or needs an update. */
    UNAVAILABLE("unavailable"),

    /** A read was attempted and failed. */
    READ_FAILED("read_failed"),
}

/** A half-open window of instants, [start, end). */
data class ReadWindow(val start: Instant, val end: Instant) {
    init {
        require(end.isAfter(start)) { "A read window must run forward." }
    }
}

data class SourceCheck(
    val outcome: SourceOutcome,
    val checkedAt: Instant,
    /** Whether the phone reads Health Connect without being opened. */
    val backgroundReads: Boolean,
    /** The window read in full; only with a read. */
    val covered: ReadWindow? = null,
) {
    init {
        require((outcome == SourceOutcome.READ) == (covered != null)) { "Only a read states its window." }
        require(covered == null || !covered.end.isAfter(checkedAt)) { "A read cannot cover time after it." }
    }
}

object SourceReporting {
    const val KIND = "source_report"

    /** How far back each Health Connect read reaches. */
    val LOOKBACK: Duration = Duration.ofDays(30)

    /**
     * A source that keeps saying the same thing is reported again after this,
     * so the desktop can tell a phone that reads from one that has gone quiet.
     */
    val REPEAT_AFTER: Duration = Duration.ofHours(6)

    /** Whether [next] says something [last] did not, or [last] is old enough to repeat. */
    fun due(last: OutboxRecord?, next: OutboxRecord): Boolean {
        if (last == null) return true
        val before = Json.parseToJsonElement(last.payload).jsonObject
        val now = Json.parseToJsonElement(next.payload).jsonObject
        val same = listOf("outcome", "background_reads", "held").all { before[it] == now[it] }
        return !same || !next.createdAt.isBefore(last.createdAt.plus(REPEAT_AFTER))
    }
}
