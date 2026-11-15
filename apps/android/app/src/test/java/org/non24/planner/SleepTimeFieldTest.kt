package org.non24.planner

import java.time.LocalDate
import java.time.ZoneId
import java.time.ZoneOffset
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test
import org.non24.planner.ui.formTimeText
import org.non24.planner.ui.offsetChoices
import org.non24.planner.ui.parseFormTime
import org.non24.planner.ui.withClock
import org.non24.planner.ui.withDate
import org.non24.planner.ui.withOffset

/** The times the correction pickers write, in the text the forms already save. */
class SleepTimeFieldTest {
    private val newYork = ZoneId.of("America/New_York")

    @Test
    fun readsTheFormsOwnTextAndWritesItBackUnchanged() {
        val text = "2026-07-08 07:30:00 -04:00"
        val time = parseFormTime(text, newYork)
        assertNotNull(time)
        assertEquals(text, formTimeText(time!!))
        // The shorter shapes the local form accepts are read too.
        assertEquals(text, formTimeText(parseFormTime("2026-07-08 07:30", newYork)!!))
        assertEquals(text, formTimeText(parseFormTime("2026-07-08 07:30 -04:00", newYork)!!))
        assertNull(parseFormTime("half past seven", newYork))
    }

    @Test
    fun aPickedClockTimeKeepsTheDayAndCarriesItsOffset() {
        val time = parseFormTime("2026-07-08 07:30:00 -04:00", newYork)!!
        assertEquals("2026-07-08 06:45:00 -04:00", formTimeText(withClock(time, 6, 45)))
    }

    @Test
    fun aPickedDayKeepsTheClockTimeAndTakesThatDaysOffset() {
        val summer = parseFormTime("2026-07-08 07:30:00 -04:00", newYork)!!
        // In December the same clock time is an hour further from UTC.
        assertEquals("2026-12-08 07:30:00 -05:00", formTimeText(withDate(summer, LocalDate.of(2026, 12, 8))))
    }

    @Test
    fun theHourThatHappensTwiceOffersBothAndOnlyThen() {
        val ordinary = parseFormTime("2026-07-08 07:30:00 -04:00", newYork)!!
        assertEquals(1, offsetChoices(ordinary).size)

        // 1:30 AM on the night the clocks go back happens twice.
        val repeated = withClock(parseFormTime("2026-11-01 00:30:00 -04:00", newYork)!!, 1, 30)
        assertEquals(listOf(ZoneOffset.ofHours(-4), ZoneOffset.ofHours(-5)), offsetChoices(repeated))
        // The offset in force is kept until the owner says otherwise.
        assertEquals("2026-11-01 01:30:00 -04:00", formTimeText(repeated))
        assertEquals("2026-11-01 01:30:00 -05:00", formTimeText(withOffset(repeated, ZoneOffset.ofHours(-5))))
    }

    @Test
    fun aClockTimeTheClocksSkippedMovesToTheNextTimeThatExists() {
        // 2:30 AM does not happen on the night the clocks go forward.
        val before = parseFormTime("2026-03-08 01:30:00 -05:00", newYork)!!
        assertEquals("2026-03-08 03:30:00 -04:00", formTimeText(withClock(before, 2, 30)))
    }
}
