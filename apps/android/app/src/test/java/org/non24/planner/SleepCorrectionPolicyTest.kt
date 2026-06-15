package org.non24.planner

import java.time.Instant
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

class SleepCorrectionPolicyTest {
    private val source = SleepEpisode(
        id = "source-1",
        start = Instant.parse("2026-03-08T06:30:00Z"),
        end = Instant.parse("2026-03-08T14:30:00Z"),
        timeZoneId = "America/New_York",
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
        correctedStart = Instant.parse(start),
        correctedEnd = Instant.parse(end),
        timeZoneId = source.timeZoneId,
        createdAt = Instant.parse(createdAt),
        provenance = Provenance(
            acquisitionMethod = AcquisitionMethod.MANUAL,
            evidenceStatus = EvidenceStatus.USER_CORRECTED,
            sourceId = "local-user",
        ),
    )
}
