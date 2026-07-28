package org.non24.planner

import java.time.Instant
import java.time.LocalDateTime
import java.time.ZoneId
import java.time.ZoneOffset
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Test
import org.non24.planner.domain.AcquisitionMethod
import org.non24.planner.domain.EvidenceStatus
import org.non24.planner.domain.Provenance
import org.non24.planner.domain.SleepCorrection
import org.non24.planner.domain.SleepCorrectionPolicy
import org.non24.planner.domain.SleepEpisode
import org.non24.planner.domain.resolveLocalDateTime
import org.non24.planner.domain.resolveTemporalZone

class SleepCorrectionPolicyTest {
    private val source = SleepEpisode(
        id = "source-1",
        logicalSourceId = "logical-source-1",
        start = Instant.parse("2026-03-08T06:30:00Z"),
        end = Instant.parse("2026-03-08T14:30:00Z"),
        ianaTimeZoneId = "America/New_York",
        startZoneOffset = ZoneOffset.ofHours(-5),
        endZoneOffset = ZoneOffset.ofHours(-4),
        provenance = Provenance(
            acquisitionMethod = AcquisitionMethod.HEALTH_CONNECT,
            evidenceStatus = EvidenceStatus.IMPORTED,
            sourceId = "synthetic.test.source",
        ),
    )

    @Test
    fun effectiveAppliesLatestCorrectionWithoutChangingSourceObservation() {
        val first = correction(
            id = "correction-1",
            start = "2026-03-08T06:45:00Z",
            end = "2026-03-08T14:15:00Z",
            createdAt = "2026-03-08T15:00:00Z",
        )
        val latest = correction(
            id = "correction-2",
            start = "2026-03-08T07:00:00Z",
            end = "2026-03-08T14:00:00Z",
            createdAt = "2026-03-08T16:00:00Z",
        )

        val effective = SleepCorrectionPolicy.effective(source, listOf(latest, first))

        assertSame(source, effective.source)
        assertEquals(Instant.parse("2026-03-08T07:00:00Z"), effective.start)
        assertEquals(Instant.parse("2026-03-08T14:00:00Z"), effective.end)
        assertEquals(ZoneOffset.ofHours(-4), effective.startZoneOffset)
        assertEquals(ZoneOffset.ofHours(-4), effective.endZoneOffset)
        assertEquals("correction-2", effective.appliedCorrection?.id)
        assertEquals(Instant.parse("2026-03-08T06:30:00Z"), source.start)
        assertEquals(Instant.parse("2026-03-08T14:30:00Z"), source.end)
    }

    @Test
    fun effectiveReturnsSourceWhenNoCorrectionExists() {
        val effective = SleepCorrectionPolicy.effective(source, emptyList())

        assertEquals(source.start, effective.start)
        assertEquals(source.end, effective.end)
        assertNull(effective.appliedCorrection)
    }

    @Test
    fun effectiveUsesLaterAppendWhenCreationTimesMatch() {
        val first = correction(
            id = "correction-1",
            start = "2026-03-08T06:45:00Z",
            end = "2026-03-08T14:15:00Z",
            createdAt = "2026-03-08T15:00:00Z",
        )
        val second = correction(
            id = "correction-2",
            start = "2026-03-08T07:00:00Z",
            end = "2026-03-08T14:00:00Z",
            createdAt = "2026-03-08T15:00:00Z",
        )

        val effective = SleepCorrectionPolicy.effective(source, listOf(first, second))

        assertEquals("correction-2", effective.appliedCorrection?.id)
    }

    @Test
    fun effectiveAllSelectsLatestCorrectionPerSource() {
        val latest = correction(
            id = "latest",
            start = "2026-03-08T07:00:00Z",
            end = "2026-03-08T14:00:00Z",
            createdAt = "2026-03-08T16:00:00Z",
        )
        val secondSource = source.copy(id = "source-2")

        val effective = SleepCorrectionPolicy.effectiveAll(listOf(source, secondSource), listOf(latest))

        assertEquals("latest", effective.first().appliedCorrection?.id)
        assertNull(effective.last().appliedCorrection)
    }

    @Test
    fun temporalZonePrefersKnownIanaThenOffsetThenFallback() {
        val offset = ZoneOffset.ofHours(-4)
        val fallback = ZoneId.of("UTC")

        assertEquals(ZoneId.of("America/New_York"), resolveTemporalZone("America/New_York", offset, fallback))
        assertEquals(offset, resolveTemporalZone(null, offset, fallback))
        assertEquals(fallback, resolveTemporalZone(null, null, fallback))
        assertEquals(offset, resolveTemporalZone("not/a-zone", offset, fallback))
    }

    @Test
    fun localDateTimeUsesEndpointOffsetToDisambiguateDstOverlap() {
        val local = LocalDateTime.parse("2026-11-01T01:30:00")

        assertEquals(
            Instant.parse("2026-11-01T05:30:00Z"),
            resolveLocalDateTime(local, "America/New_York", ZoneOffset.ofHours(-4)),
        )
        assertEquals(
            Instant.parse("2026-11-01T06:30:00Z"),
            resolveLocalDateTime(local, "America/New_York", ZoneOffset.ofHours(-5)),
        )
    }

    @Test
    fun validationRejectsReversedAndOverlongEpisodes() {
        val reversed = correction(
            id = "reversed",
            start = "2026-03-08T14:00:00Z",
            end = "2026-03-08T07:00:00Z",
            createdAt = "2026-03-08T16:00:00Z",
        )
        val overlong = correction(
            id = "overlong",
            start = "2026-03-08T07:00:00Z",
            end = "2026-03-09T08:00:00Z",
            createdAt = "2026-03-08T16:00:00Z",
        )

        assertFalse(SleepCorrectionPolicy.validate(source, reversed).isSuccess)
        assertFalse(SleepCorrectionPolicy.validate(source, overlong).isSuccess)
        assertTrue(SleepCorrectionPolicy.validate(source, correction(
            id = "valid",
            start = "2026-03-08T07:00:00Z",
            end = "2026-03-08T14:00:00Z",
            createdAt = "2026-03-08T16:00:00Z",
        )).isSuccess)
    }

    private fun correction(
        id: String,
        start: String,
        end: String,
        createdAt: String,
    ) = SleepCorrection(
        id = id,
        targetEpisodeId = source.id,
        targetLogicalSourceId = source.logicalSourceId,
        correctedStart = Instant.parse(start),
        correctedEnd = Instant.parse(end),
        ianaTimeZoneId = source.ianaTimeZoneId,
        startZoneOffset = offsetAt(start),
        endZoneOffset = offsetAt(end),
        createdAt = Instant.parse(createdAt),
        provenance = Provenance(
            acquisitionMethod = AcquisitionMethod.MANUAL,
            evidenceStatus = EvidenceStatus.USER_CORRECTED,
            sourceId = "local-user",
        ),
    )

    private fun offsetAt(instant: String): ZoneOffset =
        ZoneId.of(requireNotNull(source.ianaTimeZoneId))
            .rules
            .getOffset(Instant.parse(instant))
}
