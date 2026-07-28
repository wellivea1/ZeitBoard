package org.non24.planner.data

import java.time.Instant
import java.time.ZoneId
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import org.non24.planner.domain.AcquisitionMethod
import org.non24.planner.domain.Confidence
import org.non24.planner.domain.EstimateSnapshot
import org.non24.planner.domain.EvidenceStatus
import org.non24.planner.domain.Provenance
import org.non24.planner.domain.SleepEpisode
import org.non24.planner.domain.TimeWindow

internal class FixtureSleepRepository(
    localUserDataRepository: LocalUserDataRepository =
        LocalUserDataRepository(InMemoryLocalUserDataStore()),
    sourceEpisodes: List<SleepEpisode> = fixtureSleepEpisodes(),
) : CorrectableSleepRepository(localUserDataRepository) {
    private val mutableSourceEpisodes = MutableStateFlow(sourceEpisodes)
    override val sourceEpisodes: StateFlow<List<SleepEpisode>> = mutableSourceEpisodes.asStateFlow()

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

internal fun fixtureSleepEpisodes(): List<SleepEpisode> {
    val provenance = Provenance(
        acquisitionMethod = AcquisitionMethod.FIXTURE,
        evidenceStatus = EvidenceStatus.SYNTHETIC,
        sourceId = "android-phase-one-fixture",
    )
    val zone = ZoneId.of("America/New_York")
    return listOf(
        fixtureSleepEpisode(
            id = "fixture-sleep-2026-06-14",
            start = fixtureInstant("2026-06-14T03:55:00Z"),
            end = fixtureInstant("2026-06-14T12:05:00Z"),
            zone = zone,
            provenance = provenance,
        ),
        fixtureSleepEpisode(
            id = "fixture-sleep-2026-06-13",
            start = fixtureInstant("2026-06-13T03:08:00Z"),
            end = fixtureInstant("2026-06-13T11:20:00Z"),
            zone = zone,
            provenance = provenance,
        ),
        fixtureSleepEpisode(
            id = "fixture-sleep-2026-06-12",
            start = fixtureInstant("2026-06-12T02:22:00Z"),
            end = fixtureInstant("2026-06-12T10:35:00Z"),
            zone = zone,
            provenance = provenance,
        ),
    )
}

private fun fixtureSleepEpisode(
    id: String,
    start: Instant,
    end: Instant,
    zone: ZoneId,
    provenance: Provenance,
): SleepEpisode = SleepEpisode(
    id = id,
    logicalSourceId = id,
    start = start,
    end = end,
    ianaTimeZoneId = zone.id,
    startZoneOffset = zone.rules.getOffset(start),
    endZoneOffset = zone.rules.getOffset(end),
    provenance = provenance,
)
