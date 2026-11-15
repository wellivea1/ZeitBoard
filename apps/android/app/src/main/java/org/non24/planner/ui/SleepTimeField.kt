package org.non24.planner.ui

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.DatePicker
import androidx.compose.material3.DatePickerDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TimePicker
import androidx.compose.material3.rememberDatePickerState
import androidx.compose.material3.rememberTimePickerState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import java.time.Instant
import java.time.LocalDate
import java.time.LocalDateTime
import java.time.OffsetDateTime
import java.time.ZoneId
import java.time.ZoneOffset
import java.time.ZonedDateTime
import java.time.format.DateTimeFormatter
import java.time.format.DateTimeParseException
import java.util.Locale
import org.non24.planner.data.formatReviewInput

// One time on a correction form, chosen with the phone's own date and clock
// pickers. The forms used to ask for "yyyy-MM-dd HH:mm:ss with UTC offset",
// typed on a phone keyboard, which is the hardest way to say "I woke at one".
//
// The form still keeps each time as the text its save path reads, so nothing
// about how a correction is stored or sent changes: the pickers write that
// text. Only the hour that happens twice when the clocks go back needs a
// further answer, and it is asked only then.

private val formTimePatterns = listOf(
    DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm[:ss][.SSSSSSSSS][.SSSSSS][.SSS] xxx"),
    DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm[:ss] xxx"),
)
private val localTimePattern = DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm[:ss]")

/** Reads a form's time text in its zone, or null when it is not a time. */
internal fun parseFormTime(text: String, zone: ZoneId): ZonedDateTime? {
    val value = text.trim()
    for (pattern in formTimePatterns) {
        try {
            return OffsetDateTime.parse(value, pattern).atZoneSameInstant(zone)
        } catch (_: DateTimeParseException) {
            // Try the next shape.
        }
    }
    return try {
        ZonedDateTime.ofLocal(LocalDateTime.parse(value, localTimePattern), zone, null)
    } catch (_: DateTimeParseException) {
        null
    }
}

/** The text the form keeps for a time: to the second, with its offset. */
internal fun formTimeText(time: ZonedDateTime): String = formatReviewInput(time.toInstant(), time.zone.id)

/**
 * The same clock time on another day. The offset in force is kept where the
 * new day allows it, so a time in the repeated hour stays the one it was.
 */
internal fun withDate(time: ZonedDateTime, date: LocalDate): ZonedDateTime =
    ZonedDateTime.ofLocal(LocalDateTime.of(date, time.toLocalTime()), time.zone, time.offset)

/** The same day at another clock time, to the minute. */
internal fun withClock(time: ZonedDateTime, hour: Int, minute: Int): ZonedDateTime =
    ZonedDateTime.ofLocal(time.toLocalDate().atTime(hour, minute), time.zone, time.offset)

/**
 * The offsets a clock time can mean: two in the hour that repeats when the
 * clocks go back, one otherwise.
 */
internal fun offsetChoices(time: ZonedDateTime): List<ZoneOffset> =
    time.zone.rules.getValidOffsets(time.toLocalDateTime())

/** The same clock time under the other of its two offsets. */
internal fun withOffset(time: ZonedDateTime, offset: ZoneOffset): ZonedDateTime =
    ZonedDateTime.ofLocal(time.toLocalDateTime(), time.zone, offset)

/** "Thu, Nov 12, 6:47 PM": a time as a person would say it. */
internal fun formTimeWords(time: ZonedDateTime, use24HourTime: Boolean): String =
    DateTimeFormatter.ofPattern("EEE, MMM d", Locale.getDefault()).format(time) + ", " + clock(time, use24HourTime)

/** A stored instant in words, in its zone. */
internal fun timeWords(instant: Instant, zoneId: String, use24HourTime: Boolean): String =
    formTimeWords(instant.atZone(ZoneId.of(zoneId)), use24HourTime)

/** "Thu, Nov 12, 11:17 AM to 6:47 PM": a night by its two ends, the day said once when it is the same. */
internal fun nightWords(start: Instant, end: Instant, zoneId: String, use24HourTime: Boolean): String {
    val zone = ZoneId.of(zoneId)
    val from = start.atZone(zone)
    val to = end.atZone(zone)
    val wake = if (from.toLocalDate() == to.toLocalDate()) clock(to, use24HourTime) else formTimeWords(to, use24HourTime)
    return formTimeWords(from, use24HourTime) + " to " + wake
}

private fun utcMillis(date: LocalDate): Long = date.atStartOfDay(ZoneOffset.UTC).toInstant().toEpochMilli()

private fun utcDate(millis: Long): LocalDate = Instant.ofEpochMilli(millis).atZone(ZoneOffset.UTC).toLocalDate()

@Composable
private fun PickerButton(text: String, description: String, enabled: Boolean, onClick: () -> Unit) {
    OutlinedButton(
        onClick = onClick,
        enabled = enabled,
        modifier = Modifier.heightIn(min = 44.dp).semantics { contentDescription = description },
        shape = MaterialTheme.shapes.small,
        border = BorderStroke(1.dp, if (enabled) Ink else Line),
        colors = ButtonDefaults.outlinedButtonColors(contentColor = Ink),
        contentPadding = PaddingValues(horizontal = 12.dp, vertical = 0.dp),
    ) {
        Text(text, style = MaterialTheme.typography.labelLarge)
    }
}

/**
 * A time shown in words, with a button for its day and one for its clock time.
 * [text] is the form's own text for the time, and [onTextChange] receives the
 * text for whatever is picked.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun SleepTimeField(
    label: String,
    text: String,
    zone: ZoneId,
    use24HourTime: Boolean,
    onTextChange: (String) -> Unit,
    enabled: Boolean = true,
) {
    val time = parseFormTime(text, zone)
    var choosingDate by remember { mutableStateOf(false) }
    var choosingClock by remember { mutableStateOf(false) }

    Column(modifier = Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Text(label, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        if (time == null) {
            Text("This time could not be read. Reload the record to set it again.", style = MaterialTheme.typography.bodyMedium)
            return@Column
        }
        Text(formTimeWords(time, use24HourTime), style = MaterialTheme.typography.titleMedium)
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            PickerButton("Change day", "$label: change the day", enabled) { choosingDate = true }
            PickerButton("Change time", "$label: change the time", enabled) { choosingClock = true }
        }
        val offsets = offsetChoices(time)
        if (offsets.size > 1) {
            // The clocks went back, and this time happened twice that night.
            Text("${clock(time, use24HourTime)} happened twice that night. Which one?", style = MaterialTheme.typography.bodySmall)
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                offsets.forEachIndexed { index, offset ->
                    val chosen = offset == time.offset
                    PickerButton(
                        text = (if (chosen) "● " else "○ ") + (if (index == 0) "The first" else "The second"),
                        description = "$label: " + (if (index == 0) "the first" else "the second") + " of the two" + (if (chosen) ", chosen" else ""),
                        enabled = enabled,
                    ) { onTextChange(formTimeText(withOffset(time, offset))) }
                }
            }
        }
    }

    if (time != null && choosingDate) {
        val state = rememberDatePickerState(initialSelectedDateMillis = utcMillis(time.toLocalDate()))
        DatePickerDialog(
            onDismissRequest = { choosingDate = false },
            confirmButton = {
                TextButton(onClick = {
                    state.selectedDateMillis?.let { onTextChange(formTimeText(withDate(time, utcDate(it)))) }
                    choosingDate = false
                }) { Text("Use this day") }
            },
            dismissButton = { TextButton(onClick = { choosingDate = false }) { Text("Cancel") } },
        ) {
            DatePicker(state = state)
        }
    }
    if (time != null && choosingClock) {
        val state = rememberTimePickerState(initialHour = time.hour, initialMinute = time.minute, is24Hour = use24HourTime)
        AlertDialog(
            onDismissRequest = { choosingClock = false },
            title = { Text(label) },
            text = { TimePicker(state = state) },
            confirmButton = {
                TextButton(onClick = {
                    onTextChange(formTimeText(withClock(time, state.hour, state.minute)))
                    choosingClock = false
                }) { Text("Use this time") }
            },
            dismissButton = { TextButton(onClick = { choosingClock = false }) { Text("Cancel") } },
        )
    }
}
