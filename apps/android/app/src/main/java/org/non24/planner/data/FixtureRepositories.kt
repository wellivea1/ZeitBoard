package org.non24.planner.data

import org.non24.planner.domain.AcquisitionMethod
import org.non24.planner.domain.Confidence
import org.non24.planner.domain.EstimateSnapshot
import org.non24.planner.domain.EvidenceStatus
import org.non24.planner.domain.Provenance
import org.non24.planner.domain.SleepEpisode
import org.non24.planner.domain.TimeWindow

class FixtureSleepRepository : CorrectableSleepRepository(
    initialEpisodes = fixtureSleepEpisodes(),
) {
    override suspend fun refresh() = Unit
}

fun fixtureEstimateRepository(): EstimateRepository =
    StaticEstimateRepository(
        EstimateSnapshot(
            label = "Estimated sleep-wake phase",
            predictedSleepWindow = TimeWindow(
                start = fixtureInstant("2026-06-16T04:40:00Z"),
                end = fixtureInstant("2026-06-16T06:10:00Z"),
            ),
            predictedWakingWindow = TimeWindow(
                start = fixtureInstant("2026-06-16T12:45:00Z"),
                end = fixtureInstant("2026-06-16T14:35:00Z"),
            ),
            confidence = Confidence.MODERATE,
            confidenceReasons = listOf(
                "Synthetic fixture has seven recent principal sleep episodes.",
                "Forecast windows include explicit temporal uncertainty.",
            ),
            createdAt = fixtureInstant("2026-06-15T12:00:00Z"),
            algorithmVersion = "fixture-contract-v1",
            provenance = Provenance(
                acquisitionMethod = AcquisitionMethod.FIXTURE,
                evidenceStatus = EvidenceStatus.SYNTHETIC,
                sourceId = "android-phase-one-fixture",
            ),
        ),
    )

private fun fixtureSleepEpisodes(): List<SleepEpisode> {
    val provenance = Provenance(
        acquisitionMethod = AcquisitionMethod.FIXTURE,
        evidenceStatus = EvidenceStatus.SYNTHETIC,
        sourceId = "android-phase-one-fixture",
    )
    return listOf(
        SleepEpisode(
            id = "fixture-sleep-2026-06-14",
            start = fixtureInstant("2026-06-14T03:55:00Z"),
            end = fixtureInstant("2026-06-14T12:05:00Z"),
            timeZoneId = "America/New_York",
            provenance = provenance,
        ),
        SleepEpisode(
            id = "fixture-sleep-2026-06-13",
            start = fixtureInstant("2026-06-13T03:08:00Z"),
            end = fixtureInstant("2026-06-13T11:20:00Z"),
            timeZoneId = "America/New_York",
            provenance = provenance,
        ),
        SleepEpisode(
            id = "fixture-sleep-2026-06-12",
            start = fixtureInstant("2026-06-12T02:22:00Z"),
            end = fixtureInstant("2026-06-12T10:35:00Z"),
            timeZoneId = "America/New_York",
            provenance = provenance,
        ),
    )
}
