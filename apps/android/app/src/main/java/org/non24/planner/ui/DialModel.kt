package org.non24.planner.ui

import java.time.Duration
import java.time.Instant
import java.time.ZoneId
import java.time.ZonedDateTime
import java.time.format.DateTimeFormatter
import java.time.temporal.ChronoUnit
import java.util.Locale
import org.non24.planner.domain.TimeWindow

// What the Now dial draws, worked out apart from the drawing so it can be
// tested: the next 24 hours as three states, the recorded nights inside them,
// and the words around the dial. The states are the ones the desktop draws —
// likely asleep, likely awake, and uncertain where the model cannot say which
// side of a boundary an instant is on.

internal enum class DialState { ASLEEP, UNCERTAIN, AWAKE }

internal data class DialSegment(val state: DialState, val start: Instant, val end: Instant)

/** A predicted sleep window and a predicted waking window, as the estimator gives them. */
internal data class ForecastPair(val sleep: TimeWindow, val waking: TimeWindow)

/**
 * The forecast between [from] and [to] as three states. Sleep and waking
 * windows overlap on purpose: an instant inside both is one the model cannot
 * place, so it is uncertain rather than whichever window was drawn last.
 * Instants in neither are left out.
 */
internal fun dialSegments(forecasts: List<ForecastPair>, from: Instant, to: Instant): List<DialSegment> {
    val sleeps = forecasts.map { it.sleep }
    val wakes = forecasts.map { it.waking }
    val bounds = (sleeps + wakes)
        .flatMap { listOf(it.start, it.end) }
        .plus(listOf(from, to))
        .filter { !it.isBefore(from) && !it.isAfter(to) }
        .distinct()
        .sorted()
    val segments = mutableListOf<DialSegment>()
    for (index in 0 until bounds.size - 1) {
        val start = bounds[index]
        val end = bounds[index + 1]
        val middle = start.plusMillis(Duration.between(start, end).toMillis() / 2)
        val asleep = sleeps.any { !middle.isBefore(it.start) && middle.isBefore(it.end) }
        val awake = wakes.any { !middle.isBefore(it.start) && middle.isBefore(it.end) }
        if (!asleep && !awake) continue
        val state = when {
            asleep && awake -> DialState.UNCERTAIN
            asleep -> DialState.ASLEEP
            else -> DialState.AWAKE
        }
        val last = segments.lastOrNull()
        if (last != null && last.state == state && last.end == start) {
            segments[segments.lastIndex] = last.copy(end = end)
        } else {
            segments += DialSegment(state, start, end)
        }
    }
    return segments
}

/**
 * A local snapshot names the window sleep is likely to begin in and the one
 * waking is likely in. As the presence windows the dial draws, that is asleep
 * from the first to the second and uncertain across each, with waking either
 * side reaching past the dial.
 */
internal fun snapshotPairs(onset: TimeWindow, wake: TimeWindow): List<ForecastPair> {
    if (!wake.start.isAfter(onset.end)) return emptyList()
    val sleep = TimeWindow(onset.start, wake.end)
    val before = TimeWindow(onset.start.minus(Duration.ofHours(24)), onset.end)
    val after = TimeWindow(wake.start, wake.end.plus(Duration.ofHours(24)))
    return listOf(ForecastPair(sleep, before), ForecastPair(sleep, after))
}

internal data class SleepAhead(val onset: DialSegment?, val wake: DialSegment?)

/**
 * The uncertain band in which the next sleep is likely to begin, and the one
 * in which it is likely to end. When the forecast already has now inside a
 * sleep there is no onset, only the end.
 */
internal fun sleepAhead(segments: List<DialSegment>): SleepAhead {
    val asleep = segments.indexOfFirst { it.state == DialState.ASLEEP }
    if (asleep < 0) return SleepAhead(null, null)
    val night = segments[asleep]
    val onset = segments.getOrNull(asleep - 1)
        ?.takeIf { it.state == DialState.UNCERTAIN && it.end == night.start }
    val wake = segments.getOrNull(asleep + 1)
        ?.takeIf { it.state == DialState.UNCERTAIN && it.start == night.end }
    return SleepAhead(onset, wake)
}

internal data class LeadPart(val text: String, val strong: Boolean = false)

private fun roundToFive(instant: Instant): Instant {
    val step = 5 * 60_000L
    return Instant.ofEpochMilli(Math.round(instant.toEpochMilli().toDouble() / step) * step)
}

private fun clock(time: ZonedDateTime, use24HourTime: Boolean, withPeriod: Boolean = true): String {
    val pattern = when {
        use24HourTime -> "HH:mm"
        withPeriod -> "h:mm a"
        else -> "h:mm"
    }
    return DateTimeFormatter.ofPattern(pattern, Locale.getDefault()).format(time)
}

/** "7:15" and "10:10 AM" when both halves of the day agree, as a sentence says it. */
internal fun betweenClocks(start: Instant, end: Instant, zone: ZoneId, use24HourTime: Boolean): Pair<String, String> {
    val a = start.atZone(zone)
    val b = end.atZone(zone)
    val samePeriod = !use24HourTime && (a.hour < 12) == (b.hour < 12)
    return clock(a, use24HourTime, withPeriod = !samePeriod) to clock(b, use24HourTime)
}

/** "tonight", "tomorrow", "on Saturday": the day a moment falls on, for a sentence. */
internal fun dayInSentence(instant: Instant, now: Instant, zone: ZoneId): String {
    val day = instant.atZone(zone)
    val offset = ChronoUnit.DAYS.between(now.atZone(zone).toLocalDate(), day.toLocalDate())
    return when (offset) {
        0L -> if (day.hour >= 17) "tonight" else "today"
        1L -> "tomorrow"
        -1L -> "yesterday"
        else -> "on " + DateTimeFormatter.ofPattern("EEEE", Locale.getDefault()).format(day)
    }
}

/**
 * The sentence under the dial, the same one the desktop leads with: "Sleep is
 * likely to begin between 11:30 PM and 1:35 AM tonight, and you will probably
 * wake between 7:15 and 10:10 AM tomorrow." Each time is a range because the
 * estimate is one. Empty when there is nothing ahead to say.
 */
internal fun leadParts(ahead: SleepAhead, now: Instant, zone: ZoneId, use24HourTime: Boolean): List<LeadPart> {
    val parts = mutableListOf<LeadPart>()
    val onset = ahead.onset
    val wake = ahead.wake
    fun wakeClause(prefix: String) {
        val band = wake ?: return
        val (c, d) = betweenClocks(roundToFive(band.start), roundToFive(band.end), zone, use24HourTime)
        parts += LeadPart(prefix)
        parts += LeadPart(c, strong = true)
        parts += LeadPart(" and ")
        parts += LeadPart(d, strong = true)
        parts += LeadPart(" ${dayInSentence(band.start, now, zone)}.")
    }
    if (onset != null) {
        val start = roundToFive(onset.start)
        val end = roundToFive(onset.end)
        val (a, b) = betweenClocks(start, end, zone, use24HourTime)
        if (!onset.start.isAfter(now.plusSeconds(300))) {
            parts += LeadPart("Sleep is likely any time before ")
            parts += LeadPart(clock(end.atZone(zone), use24HourTime), strong = true)
        } else {
            parts += LeadPart("Sleep is likely to begin between ")
            parts += LeadPart(a, strong = true)
            parts += LeadPart(" and ")
            parts += LeadPart(b, strong = true)
        }
        parts += LeadPart(" ${dayInSentence(onset.start, now, zone)}")
        if (wake != null) wakeClause(", and you will probably wake between ") else parts += LeadPart(".")
    } else if (wake != null) {
        wakeClause("This sleep is likely to end between ")
    }
    return parts
}

internal data class DialCenter(val kicker: String, val figure: String, val detail: String)

/** "8 h 2 m": long enough to read at a glance, exact enough to trust. */
internal fun shortDuration(duration: Duration): String {
    val minutes = duration.toMinutes().coerceAtLeast(0)
    val hours = minutes / 60
    return when {
        hours >= 48 -> "${hours / 24} days"
        hours > 0 -> "$hours h ${minutes % 60} m"
        else -> "${minutes % 60} m"
    }
}

/**
 * The words in the middle of the dial. Awake for how long, when the records
 * are current; otherwise only when the last recorded wake was, because an
 * unrecorded sleep since then would make "awake" untrue.
 */
internal fun dialCenter(
    lastWake: Instant?,
    now: Instant,
    zone: ZoneId,
    use24HourTime: Boolean,
    current: Boolean,
): DialCenter {
    if (lastWake == null) return DialCenter("NO SLEEP RECORDED", "—", "Nothing to measure from yet")
    val since = Duration.between(lastWake, now)
    val wake = lastWake.atZone(zone)
    return if (current && !since.isNegative && since < Duration.ofHours(24)) {
        DialCenter("AWAKE", shortDuration(since), "since " + clock(wake, use24HourTime))
    } else {
        val day = DateTimeFormatter.ofPattern("EEE", Locale.getDefault()).format(wake)
        DialCenter("LAST WAKE RECORDED", shortDuration(since) + " ago", "$day " + clock(wake, use24HourTime))
    }
}
