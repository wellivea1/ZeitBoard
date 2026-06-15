package org.non24.planner.data

import java.time.Instant
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import org.non24.planner.domain.EstimateSnapshot
import org.non24.planner.domain.MedicationEvent
import org.non24.planner.domain.SleepCorrection
import org.non24.planner.domain.SleepCorrectionPolicy
import org.non24.planner.domain.SleepEpisode

abstract class CorrectableSleepRepository(
    initialEpisodes: List<SleepEpisode> = emptyList(),
) : SleepRepository {
    protected val mutableSourceEpisodes = MutableStateFlow(initialEpisodes)
    private val mutableCorrections = MutableStateFlow<List<SleepCorrection>>(emptyList())

    final override val sourceEpisodes: StateFlow<List<SleepEpisode>> = mutableSourceEpisodes.asStateFlow()
    final override val corrections: StateFlow<List<SleepCorrection>> = mutableCorrections.asStateFlow()

    final override suspend fun appendCorrection(correction: SleepCorrection): Result<Unit> {
        val source = sourceEpisodes.value.firstOrNull { it.id == correction.targetEpisodeId }
            ?: return Result.failure(IllegalArgumentException("The selected sleep episode no longer exists."))
        return SleepCorrectionPolicy.validate(source, correction).onSuccess {
            mutableCorrections.update { it + correction }
        }
    }
}

class InMemoryMedicationRepository : MedicationRepository {
    private val mutableEvents = MutableStateFlow<List<MedicationEvent>>(emptyList())
    override val events: StateFlow<List<MedicationEvent>> = mutableEvents.asStateFlow()

    override suspend fun append(event: MedicationEvent) {
        mutableEvents.update { current ->
            (current + event).sortedByDescending { it.occurredAt }
        }
    }
}

class StaticEstimateRepository(
    snapshot: EstimateSnapshot?,
) : EstimateRepository {
    private val mutableEstimate = MutableStateFlow(snapshot)
    override val estimate: StateFlow<EstimateSnapshot?> = mutableEstimate.asStateFlow()
}

internal fun fixtureInstant(value: String): Instant = Instant.parse(value)
