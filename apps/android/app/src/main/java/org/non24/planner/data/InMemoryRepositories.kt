package org.non24.planner.data

import java.time.Instant
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import org.non24.planner.domain.EstimateSnapshot
import org.non24.planner.domain.MedicationEvent
import org.non24.planner.domain.SleepCorrection
import org.non24.planner.domain.SleepCorrectionPolicy

internal abstract class CorrectableSleepRepository(
    protected val localUserDataRepository: LocalUserDataRepository =
        LocalUserDataRepository(InMemoryLocalUserDataStore()),
) : SleepRepository {
    final override val activeCorrections = localUserDataRepository.activeCorrections
    final override val correctionReviews = localUserDataRepository.correctionReviews

    final override suspend fun appendCorrection(correction: SleepCorrection): Result<Unit> {
        try {
            localUserDataRepository.initialize()
        } catch (exception: CancellationException) {
            throw exception
        } catch (exception: Exception) {
            return Result.failure(exception)
        }
        val source = sourceEpisodes.value.firstOrNull { it.id == correction.targetEpisodeId }
            ?: return Result.failure(IllegalArgumentException("The selected sleep episode no longer exists."))
        val validation = SleepCorrectionPolicy.validate(source, correction)
        if (validation.isFailure) return validation
        return try {
            localUserDataRepository.appendSleepCorrection(correction)
            Result.success(Unit)
        } catch (exception: CancellationException) {
            throw exception
        } catch (exception: Exception) {
            Result.failure(exception)
        }
    }
}

internal class LocalMedicationRepository(
    private val localUserDataRepository: LocalUserDataRepository =
        LocalUserDataRepository(InMemoryLocalUserDataStore()),
) : MedicationRepository {
    override val events: StateFlow<List<MedicationEvent>> = localUserDataRepository.medicationEvents

    override suspend fun append(event: MedicationEvent) {
        localUserDataRepository.appendMedicationEvent(event)
    }
}

class StaticEstimateRepository(
    snapshot: EstimateSnapshot?,
) : EstimateRepository {
    private val mutableEstimate = MutableStateFlow(snapshot)
    override val estimate: StateFlow<EstimateSnapshot?> = mutableEstimate.asStateFlow()
}

internal fun fixtureInstant(value: String): Instant = Instant.parse(value)
