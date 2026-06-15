package org.non24.planner.domain

import java.time.Duration
import java.time.Instant

enum class DataMode {
    FIXTURE,
    HEALTH_CONNECT,
}

enum class AcquisitionMethod {
    FIXTURE,
    HEALTH_CONNECT,
    MANUAL,
}

enum class EvidenceStatus {
    SYNTHETIC,
    IMPORTED,
    USER_CORRECTED,
}

enum class Confidence {
    LOW,
    MODERATE,
    HIGH,
}

data class Provenance(
    val acquisitionMethod: AcquisitionMethod,
    val evidenceStatus: EvidenceStatus,
    val sourceId: String,
)

data class TimeWindow(
    val start: Instant,
    val end: Instant,
) {
    init {
        require(end.isAfter(start)) { "Window end must be after its start." }
    }
}

data class SleepEpisode(
    val id: String,
    val start: Instant,
    val end: Instant,
    val timeZoneId: String,
    val provenance: Provenance,
) {
    init {
        require(end.isAfter(start)) { "Sleep end must be after sleep start." }
    }
}

data class SleepCorrection(
    val id: String,
    val targetEpisodeId: String,
    val correctedStart: Instant,
    val correctedEnd: Instant,
    val timeZoneId: String,
    val createdAt: Instant,
    val provenance: Provenance,
)

data class EffectiveSleepEpisode(
    val source: SleepEpisode,
    val start: Instant,
    val end: Instant,
    val timeZoneId: String,
    val appliedCorrection: SleepCorrection?,
)

data class EstimateSnapshot(
    val label: String,
    val predictedSleepWindow: TimeWindow,
    val predictedWakingWindow: TimeWindow,
    val confidence: Confidence,
    val confidenceReasons: List<String>,
    val createdAt: Instant,
    val algorithmVersion: String,
    val provenance: Provenance,
)

data class MedicationEvent(
    val id: String,
    val displayName: String,
    val occurredAt: Instant,
    val timeZoneId: String,
    val createdAt: Instant,
    val provenance: Provenance,
)

data class AppSettings(
    val dataMode: DataMode = DataMode.FIXTURE,
    val use24HourTime: Boolean = true,
)

enum class HealthConnectAvailability {
    AVAILABLE,
    UPDATE_REQUIRED,
    UNAVAILABLE,
}

enum class HealthPermissionState {
    UNKNOWN,
    REQUIRED,
    GRANTED,
    UNAVAILABLE,
}

object SleepCorrectionPolicy {
    private val maximumEpisodeDuration: Duration = Duration.ofHours(24)

    fun validate(source: SleepEpisode, correction: SleepCorrection): Result<Unit> {
        if (correction.targetEpisodeId != source.id) {
            return Result.failure(IllegalArgumentException("Correction targets a different sleep episode."))
        }
        if (!correction.correctedEnd.isAfter(correction.correctedStart)) {
            return Result.failure(IllegalArgumentException("Wake time must be after sleep time."))
        }
        if (Duration.between(correction.correctedStart, correction.correctedEnd) > maximumEpisodeDuration) {
            return Result.failure(IllegalArgumentException("A sleep episode cannot exceed 24 hours."))
        }
        return Result.success(Unit)
    }

    fun effective(source: SleepEpisode, corrections: List<SleepCorrection>): EffectiveSleepEpisode {
        val latest =
            corrections
                .asSequence()
                .filter { it.targetEpisodeId == source.id }
                .fold<SleepCorrection, SleepCorrection?>(null) { selected, candidate ->
                    if (selected == null || !candidate.createdAt.isBefore(selected.createdAt)) {
                        candidate
                    } else {
                        selected
                    }
                }

        return EffectiveSleepEpisode(
            source = source,
            start = latest?.correctedStart ?: source.start,
            end = latest?.correctedEnd ?: source.end,
            timeZoneId = latest?.timeZoneId ?: source.timeZoneId,
            appliedCorrection = latest,
        )
    }
}
