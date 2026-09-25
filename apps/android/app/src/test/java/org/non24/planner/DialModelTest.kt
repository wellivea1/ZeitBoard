package org.non24.planner

import java.time.Duration
import java.time.Instant
import java.time.ZoneId
import java.util.Locale
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Before
import org.junit.Test
import org.non24.planner.domain.TimeWindow
import org.non24.planner.ui.DialState
import org.non24.planner.ui.ForecastPair
import org.non24.planner.ui.dialCenter
import org.non24.planner.ui.dialSegments
import org.non24.planner.ui.leadParts
import org.non24.planner.ui.sleepAhead
import org.non24.planner.ui.snapshotPairs

class DialModelTest {
    private val zone = ZoneId.of("America/New_York")
    // Thursday 24 September, 4:10 PM in New York: the moment the design sketches use.
    private val now = Instant.parse("2026-09-24T20:10:00Z")
    private lateinit var locale: Locale

    @Before
    fun englishWords() {
        locale = Locale.getDefault()
        Locale.setDefault(Locale.US)
    }

    @After
    fun restoreLocale() = Locale.setDefault(locale)

    private fun at(hours: Double): Instant = now.plusSeconds((hours * 3600).toLong())

    // Awake until the evening's uncertain onset, asleep, then an uncertain
    // morning edge: the sleep window and the waking windows overlap at both.
    private val forecast = listOf(
        ForecastPair(sleep = TimeWindow(at(7.333), at(18.0)), waking = TimeWindow(at(-10.0), at(9.417))),
        ForecastPair(sleep = TimeWindow(at(31.0), at(40.0)), waking = TimeWindow(at(15.083), at(33.0))),
    )

    @Test
    fun overlappingWindowsAreUncertain() {
        val segments = dialSegments(forecast, now, now.plus(Duration.ofHours(24)))
        assertEquals(
            listOf(DialState.AWAKE, DialState.UNCERTAIN, DialState.ASLEEP, DialState.UNCERTAIN, DialState.AWAKE),
            segments.map { it.state },
        )
        assertEquals(at(9.417), segments[1].end)
        assertEquals(now.plus(Duration.ofHours(24)), segments.last().end)
    }

    @Test
    fun theSentenceSaysBothRangesTheWayDesktopDoes() {
        val segments = dialSegments(forecast, now, now.plus(Duration.ofHours(24)))
        val sentence = leadParts(sleepAhead(segments), now, zone, use24HourTime = false).joinToString("") { it.text }
        assertEquals(
            "Sleep is likely to begin between 11:30 PM and 1:35 AM tonight, " +
                "and you will probably wake between 7:15 and 10:10 AM tomorrow.",
            sentence,
        )
    }

    @Test
    fun insideASleepOnlyItsEndIsGiven() {
        val asleepNow = listOf(
            ForecastPair(sleep = TimeWindow(at(-4.0), at(6.0)), waking = TimeWindow(at(3.0), at(20.0))),
        )
        val ahead = sleepAhead(dialSegments(asleepNow, now, now.plus(Duration.ofHours(24))))
        assertNull(ahead.onset)
        val sentence = leadParts(ahead, now, zone, use24HourTime = true).joinToString("") { it.text }
        assertEquals("This sleep is likely to end between 19:10 and 22:10 tonight.", sentence)
    }

    @Test
    fun aLocalSnapshotIsReadAsAnOnsetWindowAndAWakeWindow() {
        val pairs = snapshotPairs(onset = TimeWindow(at(7.75), at(9.25)), wake = TimeWindow(at(15.417), at(17.25)))
        val segments = dialSegments(pairs, now, now.plus(Duration.ofHours(24)))
        assertEquals(
            listOf(DialState.AWAKE, DialState.UNCERTAIN, DialState.ASLEEP, DialState.UNCERTAIN, DialState.AWAKE),
            segments.map { it.state },
        )
        val sentence = leadParts(sleepAhead(segments), now, zone, use24HourTime = false).joinToString("") { it.text }
        assertEquals(
            "Sleep is likely to begin between 11:55 PM and 1:25 AM tonight, " +
                "and you will probably wake between 7:35 and 9:25 AM tomorrow.",
            sentence,
        )
    }

    @Test
    fun awakeIsOnlySaidWhileTheRecordsAreCurrent() {
        val wake = now.minus(Duration.ofMinutes(8 * 60 + 2))
        assertEquals("AWAKE", dialCenter(wake, now, zone, use24HourTime = false, current = true).kicker)
        assertEquals("8 h 2 m", dialCenter(wake, now, zone, use24HourTime = false, current = true).figure)
        val stale = dialCenter(wake, now, zone, use24HourTime = false, current = false)
        assertEquals("LAST WAKE RECORDED", stale.kicker)
        assertEquals("8 h 2 m ago", stale.figure)
    }
}
