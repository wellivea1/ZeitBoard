package org.non24.planner.data

import java.time.Instant
import java.time.LocalDate
import java.time.LocalTime
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

// The sample rhythm is drawn around the day the app is opened, so Now shows a
// night ahead rather than an empty dial: three recorded nights drifting about
// 45 minutes later each cycle, and the forecast for the next. It is sample
// data and is labelled as such wherever it appears.
private val fixtureZone: ZoneId = ZoneId.of("America/New_York")

/**
 * The day the sample's last recorded night ended on: the most recent 7:20 in
 * the morning. Before then the sample has you in the night it forecast.
 */
internal fun fixtureAnchor(now: Instant): LocalDate {
    val local = now.atZone(fixtureZone)
    val woke = local.toLocalTime() >= LocalTime.of(7, 20)
    return if (woke) local.toLocalDate() else local.toLocalDate().minusDays(1)
}

private fun fixtureAt(day: LocalDate, hour: Int, minute: Int): Instant =
    day.atTime(hour, minute).atZone(fixtureZone).toInstant()

fun fixtureEstimateRepository(now: Instant = Instant.now()): EstimateRepository {
    val day = fixtureAnchor(now)
    return StaticEstimateRepository(
        EstimateSnapshot(
            label = "Estimated sleep-wake phase",
            // The window sleep is likely to begin in, then the one waking is likely in.
            predictedSleepWindow = TimeWindow(
                start = fixtureAt(day, 23, 55),
                end = fixtureAt(day.plusDays(1), 1, 25),
            ),
            predictedWakingWindow = TimeWindow(
                start = fixtureAt(day.plusDays(1), 7, 35),
                end = fixtureAt(day.plusDays(1), 9, 25),
            ),
            confidence = Confidence.MODERATE,
            confidenceReasons = listOf(
                "Synthetic fixture has seven recent principal sleep episodes.",
                "Forecast windows include explicit temporal uncertainty.",
            ),
            createdAt = now.minusSeconds(3600),
            algorithmVersion = "fixture-contract-v1",
            provenance = Provenance(
                acquisitionMethod = AcquisitionMethod.FIXTURE,
                evidenceStatus = EvidenceStatus.SYNTHETIC,
                sourceId = "android-phase-one-fixture",
            ),
        ),
    )
}

/** Sample accepted times for the dial in sample mode, placed around the present. */
internal fun fixturePlans(now: Instant = Instant.now()): List<SyncedPlan> {
    val start = now.truncatedTo(java.time.temporal.ChronoUnit.HOURS).plus(java.time.Duration.ofHours(3))
    return listOf(
        SyncedPlan("Sample: paperwork", start, start.plus(java.time.Duration.ofMinutes(90))),
        SyncedPlan("Sample: call the pharmacy", start.plus(java.time.Duration.ofHours(19)), start.plus(java.time.Duration.ofHours(19).plusMinutes(30))),
    )
}

internal fun fixtureSleepEpisodes(now: Instant = Instant.now()): List<SleepEpisode> {
    val provenance = Provenance(
        acquisitionMethod = AcquisitionMethod.FIXTURE,
        evidenceStatus = EvidenceStatus.SYNTHETIC,
        sourceId = "android-phase-one-fixture",
    )
    val day = fixtureAnchor(now)
    // The identifiers keep their old dates so sample corrections saved
    // against them stay attached.
    return listOf(
        fixtureSleepEpisode(
            id = "fixture-sleep-2026-06-14",
            start = fixtureAt(day.minusDays(1), 23, 10),
            end = fixtureAt(day, 7, 20),
            zone = fixtureZone,
            provenance = provenance,
        ),
        fixtureSleepEpisode(
            id = "fixture-sleep-2026-06-13",
            start = fixtureAt(day.minusDays(2), 22, 25),
            end = fixtureAt(day.minusDays(1), 6, 35),
            zone = fixtureZone,
            provenance = provenance,
        ),
        fixtureSleepEpisode(
            id = "fixture-sleep-2026-06-12",
            start = fixtureAt(day.minusDays(3), 21, 40),
            end = fixtureAt(day.minusDays(2), 5, 50),
            zone = fixtureZone,
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
