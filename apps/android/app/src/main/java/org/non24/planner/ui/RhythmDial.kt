package org.non24.planner.ui

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.widthIn
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.DrawScope
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.drawscope.clipPath
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.drawText
import androidx.compose.ui.text.rememberTextMeasurer
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import java.time.Duration
import java.time.Instant
import java.time.ZoneId
import kotlin.math.cos
import kotlin.math.sin
import org.non24.planner.domain.TimeWindow

// Now on Android (ui-refactor-plan.md §14): the day as a 24-hour clock face,
// midnight at the top. The outer ring is the next 24 hours — likely asleep in
// the sleep blue, the edges the model cannot place hatched, likely awake left
// as the track. Inside it the last seven recorded nights step inwards, newest
// outermost, so a drift later each cycle reads as the arcs turning. The hand
// is now, in the accent.

private const val DAY_MILLIS = 86_400_000f

private fun clockDegrees(instant: Instant, zone: ZoneId): Float {
    val time = instant.atZone(zone)
    val minutes = time.hour * 60 + time.minute + time.second / 60f
    return minutes / 1440f * 360f
}

private fun sweepDegrees(start: Instant, end: Instant): Float =
    (Duration.between(start, end).toMillis() / DAY_MILLIS * 360f).coerceIn(0f, 360f)

private fun DrawScope.pointAt(radius: Float, degrees: Float): Offset {
    val radians = Math.toRadians(degrees.toDouble())
    return Offset(center.x + radius * sin(radians).toFloat(), center.y - radius * cos(radians).toFloat())
}

/** An arc of a ring, angles in clock degrees (0 at the top, clockwise). */
private fun DrawScope.ringArc(outer: Float, inner: Float, start: Float, sweep: Float, color: Color, alpha: Float = 1f) {
    val middle = (outer + inner) / 2f
    drawArc(
        color = color,
        startAngle = start - 90f,
        sweepAngle = sweep,
        useCenter = false,
        topLeft = Offset(center.x - middle, center.y - middle),
        size = Size(middle * 2f, middle * 2f),
        style = Stroke(width = outer - inner),
        alpha = alpha,
    )
}

/** Hatched, as on the desktop: stripes read as "not solid knowledge" in any theme. */
private fun DrawScope.hatchedArc(outer: Float, inner: Float, start: Float, sweep: Float, color: Color) {
    val path = Path().apply {
        arcTo(Rect(center, outer), start - 90f, sweep, forceMoveTo = true)
        arcTo(Rect(center, inner), start - 90f + sweep, -sweep, forceMoveTo = false)
        close()
    }
    clipPath(path) {
        val step = 5.dp.toPx()
        val stroke = 1.6.dp.toPx()
        var x = -size.height
        while (x < size.width + size.height) {
            drawLine(color, Offset(x, 0f), Offset(x + size.height, size.height), stroke)
            x += step
        }
    }
}

@Composable
internal fun RhythmDial(
    segments: List<DialSegment>,
    nights: List<TimeWindow>,
    now: Instant,
    zone: ZoneId,
    use24HourTime: Boolean,
    reading: DialCenter,
    description: String,
    modifier: Modifier = Modifier,
) {
    val measurer = rememberTextMeasurer()
    // Drawing is not composable: take the theme's colours in with it.
    val palette = LocalAlmanacPalette.current
    val labelStyle = MaterialTheme.typography.labelSmall.copy(color = palette.muted)
    val labels = if (use24HourTime) listOf("00", "06", "12", "18") else listOf("12a", "6a", "12p", "6p")

    Box(
        modifier = modifier
            .widthIn(max = 360.dp)
            .fillMaxWidth()
            .aspectRatio(1f)
            .semantics { contentDescription = description },
        contentAlignment = Alignment.Center,
    ) {
        Canvas(modifier = Modifier.fillMaxSize()) {
            val radius = size.minDimension / 2f
            val forecastOuter = radius * 0.80f
            val forecastInner = radius * 0.69f

            // The next 24 hours.
            ringArc(forecastOuter, forecastInner, 0f, 360f, palette.divider, alpha = 0.55f)
            segments.forEach { segment ->
                val start = clockDegrees(segment.start, zone)
                val sweep = sweepDegrees(segment.start, segment.end)
                when (segment.state) {
                    DialState.ASLEEP -> ringArc(forecastOuter, forecastInner, start, sweep, palette.sleepBlue)
                    DialState.UNCERTAIN -> hatchedArc(forecastOuter, forecastInner, start, sweep, palette.uncertainFill)
                    DialState.AWAKE -> Unit
                }
            }

            // The last seven nights, newest outermost, fading inwards.
            val nightWidth = radius * 0.03f
            val pitch = radius * 0.042f
            nights.take(7).forEachIndexed { index, night ->
                val outer = radius * 0.64f - index * pitch
                val inner = outer - nightWidth
                ringArc(outer, inner, 0f, 360f, palette.panelAlt)
                ringArc(
                    outer,
                    inner,
                    clockDegrees(night.start, zone),
                    sweepDegrees(night.start, night.end),
                    palette.sleepBlue,
                    alpha = 1f - index * 0.1f,
                )
            }

            // Hours: a tick for each, longer at midnight, six, noon and eighteen.
            val tickOuter = radius * 0.885f
            for (hour in 0 until 24) {
                val major = hour % 6 == 0
                val tickInner = tickOuter - radius * (if (major) 0.065f else 0.03f)
                drawLine(
                    color = if (major) palette.muted else palette.divider,
                    start = pointAt(tickInner, hour * 15f),
                    end = pointAt(tickOuter, hour * 15f),
                    strokeWidth = (if (major) 1.4f else 1f).dp.toPx(),
                )
            }
            labels.forEachIndexed { index, label ->
                val layout = measurer.measure(label, labelStyle)
                val at = pointAt(radius * 0.955f, index * 90f)
                drawText(layout, topLeft = at - Offset(layout.size.width / 2f, layout.size.height / 2f))
            }

            // Now.
            val nowDegrees = clockDegrees(now, zone)
            val handEnd = pointAt(radius * 0.84f, nowDegrees)
            drawLine(
                color = palette.accent,
                start = pointAt(radius * 0.38f, nowDegrees),
                end = handEnd,
                strokeWidth = 2.dp.toPx(),
                cap = StrokeCap.Round,
            )
            drawCircle(palette.accent, radius = 3.5.dp.toPx(), center = handEnd)
        }
        Column(
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(2.dp),
        ) {
            Text(reading.kicker, style = MaterialTheme.typography.labelSmall, color = Muted, textAlign = TextAlign.Center)
            Text(
                reading.figure,
                style = MaterialTheme.typography.headlineMedium.copy(fontSize = 26.sp, lineHeight = 30.sp),
                color = Ink,
                textAlign = TextAlign.Center,
            )
            Text(reading.detail, style = MaterialTheme.typography.bodySmall, color = Muted, textAlign = TextAlign.Center)
        }
    }
}
