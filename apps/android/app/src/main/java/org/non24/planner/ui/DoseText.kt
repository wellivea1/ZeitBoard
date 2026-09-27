package org.non24.planner.ui

import java.time.Instant
import java.time.LocalDate
import java.time.LocalTime
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.temporal.ChronoUnit
import java.util.Locale
import org.non24.planner.data.CompanionDose
import org.non24.planner.data.CompanionMedication

// The words beside each medication on the Doses screen, as the desktop's
// design sketches them: "Usually 10:00 PM · last taken yesterday at 10:05 PM".

/** "Usually 8:00 AM and 8:00 PM", "As needed", "On a cycle"; null without a schedule. */
internal fun usualTimes(medication: CompanionMedication, deviceZone: ZoneId, use24HourTime: Boolean): String? {
    val times = medication.civilTimes
    return when {
        medication.scheduleKind == null -> null
        medication.scheduleKind == "as_needed" -> "As needed"
        times.isEmpty() -> if (medication.scheduleKind == "cycling") "On a cycle" else null
        else -> {
            val zone = medication.scheduleZoneId?.let(ZoneId::of) ?: deviceZone
            val clocks = times.map { clock(LocalDate.now(zone).atTime(LocalTime.parse(it)).atZone(zone), use24HourTime) }
            // The schedule keeps its own zone; say so when the phone is elsewhere.
            val elsewhere = if (zone.rules != deviceZone.rules) " ${zone.id.substringAfterLast('/').replace('_', ' ')} time" else ""
            "Usually " + clocks.joinToString(" and ") + elsewhere
        }
    }
}

/** "Last taken yesterday at 10:05 PM", "Skipped today at 8:00 AM, uploading", "Nothing recorded yet". */
internal fun lastDoseText(dose: CompanionDose?, now: Instant, deviceZone: ZoneId, use24HourTime: Boolean): String {
    if (dose == null) return "Nothing recorded yet"
    val at = dose.doseAt.atZone(deviceZone)
    val days = ChronoUnit.DAYS.between(at.toLocalDate(), now.atZone(deviceZone).toLocalDate())
    val day = when {
        days == 0L -> "today"
        days == 1L -> "yesterday"
        days in 2..6 -> "on " + DateTimeFormatter.ofPattern("EEEE", Locale.getDefault()).format(at)
        else -> "on " + DateTimeFormatter.ofPattern("d MMM", Locale.getDefault()).format(at)
    }
    val verb = if (dose.status == "taken") "taken" else "skipped"
    val uploading = if (dose.pending) ", uploading" else ""
    return "Last $verb $day at ${clock(at, use24HourTime)}$uploading"
}
