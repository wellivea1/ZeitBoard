package org.non24.planner.domain

import java.time.Duration
import java.time.Instant
import java.time.LocalDateTime
import java.time.ZoneId
import java.time.ZoneOffset

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
    val sourceRecordId: String? = null,
    val sourceUpdatedAt: Instant? = null,
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
    val logicalSourceId: String,
    val start: Instant,
    val end: Instant,
    val ianaTimeZoneId: String?,
    val startZoneOffset: ZoneOffset?,
    val endZoneOffset: ZoneOffset?,
    val provenance: Provenance,
) {
    init {
        require(id.isNotBlank()) { "Sleep revision ID must not be blank." }
        require(logicalSourceId.isNotBlank()) { "Sleep logical source ID must not be blank." }
        require(end.isAfter(start)) { "Sleep end must be after sleep start." }
    }
}

data class SleepCorrection(
    val id: String,
    val targetEpisodeId: String,
    val targetLogicalSourceId: String,
    val correctedStart: Instant,
    val correctedEnd: Instant,
    val ianaTimeZoneId: String?,
    val startZoneOffset: ZoneOffset?,
    val endZoneOffset: ZoneOffset?,
    val createdAt: Instant,
    val provenance: Provenance,
) {
    init {
        require(id.isNotBlank()) { "Sleep correction ID must not be blank." }
        require(targetEpisodeId.isNotBlank()) { "Correction target revision ID must not be blank." }
        require(targetLogicalSourceId.isNotBlank()) { "Correction logical source ID must not be blank." }
    }
}

data class SleepCorrectionReview(
    val correction: SleepCorrection,
    val currentEpisodeId: String,
)

data class EffectiveSleepEpisode(
    val source: SleepEpisode,
    val start: Instant,
    val end: Instant,
    val ianaTimeZoneId: String?,
    val startZoneOffset: ZoneOffset?,
    val endZoneOffset: ZoneOffset?,
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

fun resolveTemporalZone(
    ianaTimeZoneId: String?,
    offset: ZoneOffset?,
    fallback: ZoneId = ZoneId.systemDefault(),
): ZoneId {
    if (ianaTimeZoneId != null) {
        runCatching { ZoneId.of(ianaTimeZoneId) }.getOrNull()?.let { return it }
    }
    return offset ?: fallback
}

fun resolveLocalDateTime(
    localDateTime: LocalDateTime,
    ianaTimeZoneId: String?,
    preferredOffset: ZoneOffset?,
    fallback: ZoneId = ZoneId.systemDefault(),
    explicitOffset: ZoneOffset? = null,
): Instant = resolveLocalDateTimeWithOffset(
    localDateTime = localDateTime,
    ianaTimeZoneId = ianaTimeZoneId,
    preferredOffset = preferredOffset,
    fallback = fallback,
    explicitOffset = explicitOffset,
).instant

data class ResolvedLocalDateTime(
    val instant: Instant,
    val offset: ZoneOffset,
)

sealed class LocalTimeResolutionException(message: String) : IllegalArgumentException(message)

class NonexistentLocalTimeException(
    val localDateTime: LocalDateTime,
    val zoneId: ZoneId,
) : LocalTimeResolutionException("$localDateTime does not exist in ${zoneId.id}.")

class AmbiguousLocalTimeException(
    val localDateTime: LocalDateTime,
    val zoneId: ZoneId,
    val validOffsets: List<ZoneOffset>,
) : LocalTimeResolutionException("$localDateTime occurs twice in ${zoneId.id}.")

class InvalidLocalTimeOffsetException(
    val localDateTime: LocalDateTime,
    val zoneId: ZoneId,
    val suppliedOffset: ZoneOffset,
) : LocalTimeResolutionException(
    "$suppliedOffset is not valid for $localDateTime in ${zoneId.id}.",
)

fun resolveLocalDateTimeWithOffset(
    localDateTime: LocalDateTime,
    ianaTimeZoneId: String?,
    preferredOffset: ZoneOffset?,
    fallback: ZoneId = ZoneId.systemDefault(),
    explicitOffset: ZoneOffset? = null,
): ResolvedLocalDateTime {
    val namedZone = ianaTimeZoneId
        ?.let { runCatching { ZoneId.of(it) }.getOrNull() }
    if (namedZone == null && preferredOffset != null) {
        if (explicitOffset != null && explicitOffset != preferredOffset) {
            throw InvalidLocalTimeOffsetException(localDateTime, preferredOffset, explicitOffset)
        }
        return ResolvedLocalDateTime(
            localDateTime.toInstant(preferredOffset),
            preferredOffset,
        )
    }
    val zone = namedZone ?: fallback

    if (zone is ZoneOffset) {
        val selectedOffset = explicitOffset ?: zone
        return ResolvedLocalDateTime(localDateTime.toInstant(selectedOffset), selectedOffset)
    }

    val validOffsets = zone.rules.getValidOffsets(localDateTime)
    if (validOffsets.isEmpty()) {
        throw NonexistentLocalTimeException(localDateTime, zone)
    }

    val selectedOffset = when {
        explicitOffset != null && explicitOffset !in validOffsets ->
            throw InvalidLocalTimeOffsetException(localDateTime, zone, explicitOffset)
        explicitOffset != null -> explicitOffset
        preferredOffset != null && preferredOffset in validOffsets -> preferredOffset
        validOffsets.size == 1 -> validOffsets.single()
        else -> throw AmbiguousLocalTimeException(localDateTime, zone, validOffsets)
    }
    return ResolvedLocalDateTime(localDateTime.toInstant(selectedOffset), selectedOffset)
}

object SleepCorrectionPolicy {
    private val maximumEpisodeDuration: Duration = Duration.ofHours(24)

    fun validate(source: SleepEpisode, correction: SleepCorrection): Result<Unit> {
        if (correction.targetEpisodeId != source.id) {
            return Result.failure(IllegalArgumentException("Correction targets a different sleep episode."))
        }
        if (correction.targetLogicalSourceId != source.logicalSourceId) {
            return Result.failure(IllegalArgumentException("Correction targets a different logical sleep source."))
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
                    if (selected == null || isCorrectionNewer(candidate, selected)) {
                        candidate
                    } else {
                        selected
                    }
                }

        return effective(source, latest)
    }

    fun effectiveAll(
        sources: List<SleepEpisode>,
        corrections: List<SleepCorrection>,
    ): List<EffectiveSleepEpisode> {
        val latestByTarget = HashMap<String, SleepCorrection>()
        corrections.forEach { candidate ->
            val selected = latestByTarget[candidate.targetEpisodeId]
            if (selected == null || isCorrectionNewer(candidate, selected)) {
                latestByTarget[candidate.targetEpisodeId] = candidate
            }
        }
        return effectiveAll(sources, latestByTarget)
    }

    fun effectiveAll(
        sources: List<SleepEpisode>,
        latestByTarget: Map<String, SleepCorrection>,
    ): List<EffectiveSleepEpisode> {
        return sources.map { source -> effective(source, latestByTarget[source.id]) }
    }

    private fun effective(
        source: SleepEpisode,
        latest: SleepCorrection?,
    ): EffectiveSleepEpisode {
        return EffectiveSleepEpisode(
            source = source,
            start = latest?.correctedStart ?: source.start,
            end = latest?.correctedEnd ?: source.end,
            ianaTimeZoneId = latest?.ianaTimeZoneId ?: source.ianaTimeZoneId,
            startZoneOffset = latest?.startZoneOffset ?: source.startZoneOffset,
            endZoneOffset = latest?.endZoneOffset ?: source.endZoneOffset,
            appliedCorrection = latest,
        )
    }
}

fun isCorrectionNewer(candidate: SleepCorrection, selected: SleepCorrection): Boolean =
    candidate.createdAt.isAfter(selected.createdAt) ||
        (candidate.createdAt == selected.createdAt && candidate.id > selected.id)
