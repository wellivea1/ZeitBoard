package org.non24.planner

import java.time.Instant
import java.time.LocalDateTime
import java.time.ZoneOffset
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test
import org.non24.planner.domain.AmbiguousLocalTimeException
import org.non24.planner.domain.InvalidLocalTimeOffsetException
import org.non24.planner.domain.NonexistentLocalTimeException
import org.non24.planner.domain.resolveLocalDateTimeWithOffset

class LocalTimeResolutionTest {
    @Test
    fun rejectsNamedZoneDstGap() {
        assertThrows(NonexistentLocalTimeException::class.java) {
            resolveLocalDateTimeWithOffset(
                localDateTime = LocalDateTime.parse("2026-03-08T02:30:00"),
                ianaTimeZoneId = "America/New_York",
                preferredOffset = null,
            )
        }
    }

    @Test
    fun overlapRequiresExplicitOffsetWhenSourceHasNoOffset() {
        val error = assertThrows(AmbiguousLocalTimeException::class.java) {
            resolveLocalDateTimeWithOffset(
                localDateTime = LocalDateTime.parse("2026-11-01T01:30:00"),
                ianaTimeZoneId = "America/New_York",
                preferredOffset = null,
            )
        }

        assertEquals(
            setOf(ZoneOffset.ofHours(-4), ZoneOffset.ofHours(-5)),
            error.validOffsets.toSet(),
        )
    }

    @Test
    fun explicitOffsetsSelectBothSidesOfOverlap() {
        val local = LocalDateTime.parse("2026-11-01T01:30:00")

        val earlier = resolveLocalDateTimeWithOffset(
            localDateTime = local,
            ianaTimeZoneId = "America/New_York",
            preferredOffset = null,
            explicitOffset = ZoneOffset.ofHours(-4),
        )
        val later = resolveLocalDateTimeWithOffset(
            localDateTime = local,
            ianaTimeZoneId = "America/New_York",
            preferredOffset = null,
            explicitOffset = ZoneOffset.ofHours(-5),
        )

        assertEquals(Instant.parse("2026-11-01T05:30:00Z"), earlier.instant)
        assertEquals(Instant.parse("2026-11-01T06:30:00Z"), later.instant)
    }

    @Test
    fun sourceOffsetDisambiguatesOverlapWithoutExtraInput() {
        val resolved = resolveLocalDateTimeWithOffset(
            localDateTime = LocalDateTime.parse("2026-11-01T01:30:00"),
            ianaTimeZoneId = "America/New_York",
            preferredOffset = ZoneOffset.ofHours(-5),
        )

        assertEquals(Instant.parse("2026-11-01T06:30:00Z"), resolved.instant)
        assertEquals(ZoneOffset.ofHours(-5), resolved.offset)
    }

    @Test
    fun rejectsOffsetThatDoesNotBelongToNamedZoneAtLocalTime() {
        assertThrows(InvalidLocalTimeOffsetException::class.java) {
            resolveLocalDateTimeWithOffset(
                localDateTime = LocalDateTime.parse("2026-11-01T01:30:00"),
                ianaTimeZoneId = "America/New_York",
                preferredOffset = null,
                explicitOffset = ZoneOffset.ofHours(-6),
            )
        }
    }

    @Test
    fun rejectsOffsetThatContradictsFixedSourceOffset() {
        assertThrows(InvalidLocalTimeOffsetException::class.java) {
            resolveLocalDateTimeWithOffset(
                localDateTime = LocalDateTime.parse("2026-11-01T01:30:00"),
                ianaTimeZoneId = null,
                preferredOffset = ZoneOffset.ofHours(-4),
                explicitOffset = ZoneOffset.ofHours(-5),
            )
        }
    }
}
