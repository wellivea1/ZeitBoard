package org.non24.planner.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import java.time.Clock
import java.time.DateTimeException
import java.time.Instant
import java.time.LocalDateTime
import java.time.ZoneId
import java.time.ZoneOffset
import java.time.format.DateTimeFormatter
import java.time.format.DateTimeFormatterBuilder
import java.time.format.ResolverStyle
import java.time.temporal.TemporalQueries
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicLong
import java.util.UUID
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import org.non24.planner.AppDependencies
import org.non24.planner.data.DurableLocalDataState
import org.non24.planner.data.SleepRepository
import org.non24.planner.domain.AcquisitionMethod
import org.non24.planner.domain.AmbiguousLocalTimeException
import org.non24.planner.domain.AppSettings
import org.non24.planner.domain.DataMode
import org.non24.planner.domain.EffectiveSleepEpisode
import org.non24.planner.domain.EstimateSnapshot
import org.non24.planner.domain.EvidenceStatus
import org.non24.planner.domain.HealthConnectAvailability
import org.non24.planner.domain.HealthPermissionState
import org.non24.planner.domain.InvalidLocalTimeOffsetException
import org.non24.planner.domain.MedicationEvent
import org.non24.planner.domain.NonexistentLocalTimeException
import org.non24.planner.domain.Provenance
import org.non24.planner.domain.ResolvedLocalDateTime
import org.non24.planner.domain.SleepCorrection
import org.non24.planner.domain.SleepCorrectionReview
import org.non24.planner.domain.SleepCorrectionPolicy
import org.non24.planner.domain.SleepEpisode
import org.non24.planner.domain.resolveLocalDateTimeWithOffset

data class AppUiState(
    val settings: AppSettings = AppSettings(),
    val sleepEpisodes: List<EffectiveSleepEpisode> = emptyList(),
    val estimate: EstimateSnapshot? = null,
    val medicationEvents: List<MedicationEvent> = emptyList(),
    val healthAvailability: HealthConnectAvailability = HealthConnectAvailability.UNAVAILABLE,
    val healthPermissionState: HealthPermissionState = HealthPermissionState.UNKNOWN,
    val healthRefreshError: String? = null,
    val localDataState: DurableLocalDataState = DurableLocalDataState.Loading,
    val correctionReviews: List<SleepCorrectionReview> = emptyList(),
) {
    val latestSleepEpisode: EffectiveSleepEpisode?
        get() = sleepEpisodes.maxByOrNull { it.start }
}

private data class SleepState(
    val episodes: List<EffectiveSleepEpisode>,
    val estimate: EstimateSnapshot?,
    val correctionReviews: List<SleepCorrectionReview>,
)

private data class HealthState(
    val episodes: List<EffectiveSleepEpisode>,
    val availability: HealthConnectAvailability,
    val permissionState: HealthPermissionState,
    val refreshError: String?,
    val correctionReviews: List<SleepCorrectionReview>,
)

sealed interface MedicationSaveState {
    data object Idle : MedicationSaveState

    data class Saving(val requestId: Long) : MedicationSaveState

    data class Succeeded(val requestId: Long) : MedicationSaveState

    data class Failed(val requestId: Long, val message: String) : MedicationSaveState
}

class AppViewModel(
    private val container: AppDependencies,
    private val clock: Clock = Clock.systemUTC(),
) : ViewModel() {
    private val mutableMessage = MutableStateFlow<String?>(null)
    val message: StateFlow<String?> = mutableMessage.asStateFlow()
    private val medicationSaveGuard = AtomicBoolean(false)
    private val medicationSaveSequence = AtomicLong(0)
    private val mutableMedicationSaveState = MutableStateFlow<MedicationSaveState>(MedicationSaveState.Idle)
    private var retryableMedicationEvent: MedicationEvent? = null
    val medicationSaveState: StateFlow<MedicationSaveState> = mutableMedicationSaveState.asStateFlow()

    private val fixtureState = combine(
        container.fixtureSleepRepository.sourceEpisodes,
        container.fixtureSleepRepository.activeCorrections,
        container.fixtureSleepRepository.correctionReviews,
        container.fixtureEstimateRepository.estimate,
    ) { episodes, activeCorrections, correctionReviews, estimate ->
        SleepState(
            episodes = effectiveEpisodes(episodes, activeCorrections),
            estimate = estimate,
            correctionReviews = reviewsForEpisodes(episodes, correctionReviews),
        )
    }

    private val healthSleepState = combine(
        container.healthConnectRepository.sourceEpisodes,
        container.healthConnectRepository.activeCorrections,
        container.healthConnectRepository.correctionReviews,
    ) { episodes, activeCorrections, correctionReviews ->
        SleepState(
            episodes = effectiveEpisodes(episodes, activeCorrections),
            estimate = null,
            correctionReviews = reviewsForEpisodes(episodes, correctionReviews),
        )
    }

    private val healthState = combine(
        healthSleepState,
        container.healthConnectRepository.availability,
        container.healthConnectRepository.permissionState,
        container.healthConnectRepository.lastRefreshError,
    ) { sleep, availability, permissionState, refreshError ->
        HealthState(
            episodes = sleep.episodes,
            availability = availability,
            permissionState = permissionState,
            refreshError = refreshError,
            correctionReviews = sleep.correctionReviews,
        )
    }

    val uiState: StateFlow<AppUiState> = combine(
        container.settingsRepository.settings,
        fixtureState,
        healthState,
        container.medicationRepository.events,
        container.localDataState,
    ) { settings, fixture, health, medications, localDataState ->
        val activeEpisodes = when (settings.dataMode) {
            DataMode.FIXTURE -> fixture.episodes
            DataMode.HEALTH_CONNECT -> health.episodes
        }
        val correctionReviews = when (settings.dataMode) {
            DataMode.FIXTURE -> fixture.correctionReviews
            DataMode.HEALTH_CONNECT -> health.correctionReviews
        }
        AppUiState(
            settings = settings,
            sleepEpisodes = activeEpisodes,
            estimate = if (settings.dataMode == DataMode.FIXTURE) fixture.estimate else null,
            medicationEvents = medications,
            healthAvailability = health.availability,
            healthPermissionState = health.permissionState,
            healthRefreshError = health.refreshError,
            localDataState = localDataState,
            correctionReviews = correctionReviews,
        )
    }.stateIn(
        scope = viewModelScope,
        started = SharingStarted.WhileSubscribed(5_000),
        initialValue = AppUiState(),
    )

    init {
        viewModelScope.launch {
            try {
                container.initializeLocalUserData()
            } catch (exception: CancellationException) {
                throw exception
            } catch (_: Exception) {
                // The repository publishes a durable Failed state for affected screens.
            }
        }
        viewModelScope.launch {
            container.healthConnectRepository.refreshPermissionState()
        }
    }

    fun retryLocalData() {
        viewModelScope.launch {
            try {
                container.initializeLocalUserData()
            } catch (exception: CancellationException) {
                throw exception
            } catch (_: Exception) {
                // State is already published by the durable repository.
            }
        }
    }

    fun setDataMode(mode: DataMode) {
        viewModelScope.launch {
            container.settingsRepository.update { it.copy(dataMode = mode) }
            if (mode == DataMode.HEALTH_CONNECT) {
                container.healthConnectRepository.refresh()
            }
        }
    }

    fun setUse24HourTime(enabled: Boolean) {
        viewModelScope.launch {
            container.settingsRepository.update { it.copy(use24HourTime = enabled) }
        }
    }

    fun refreshHealthConnect() {
        viewModelScope.launch {
            container.healthConnectRepository.refresh()
            mutableMessage.value = container.healthConnectRepository.lastRefreshError.value
                ?: when (container.healthConnectRepository.permissionState.value) {
                HealthPermissionState.GRANTED -> "Health Connect sleep import refreshed."
                HealthPermissionState.REQUIRED -> "Sleep permission is still required."
                HealthPermissionState.UNAVAILABLE -> "Health Connect is unavailable on this device."
                HealthPermissionState.UNKNOWN -> "Health Connect permission state is unknown."
            }
        }
    }

    fun onHealthPermissionResult(grantedPermissions: Set<String>) {
        viewModelScope.launch {
            container.healthConnectRepository.onPermissionResult(grantedPermissions)
            mutableMessage.value = if (
                grantedPermissions.containsAll(container.healthConnectRepository.requiredPermissions)
            ) {
                "Sleep read permission granted."
            } else {
                "Sleep read permission was not granted."
            }
        }
    }

    fun saveLatestSleepCorrection(startText: String, endText: String) {
        val latest = uiState.value.latestSleepEpisode ?: run {
            mutableMessage.value = "No sleep episode is available to correct."
            return
        }
        val correctedStart = parseLocalDateTime(
            startText,
            latest.ianaTimeZoneId,
            latest.startZoneOffset,
        ) ?: return
        val correctedEnd = parseLocalDateTime(
            endText,
            latest.ianaTimeZoneId,
            latest.endZoneOffset,
        ) ?: return
        val correction = SleepCorrection(
            id = UUID.randomUUID().toString(),
            targetEpisodeId = latest.source.id,
            targetLogicalSourceId = latest.source.logicalSourceId,
            correctedStart = correctedStart.instant,
            correctedEnd = correctedEnd.instant,
            ianaTimeZoneId = latest.ianaTimeZoneId,
            startZoneOffset = correctedStart.offset,
            endZoneOffset = correctedEnd.offset,
            createdAt = Instant.now(clock),
            provenance = Provenance(
                acquisitionMethod = AcquisitionMethod.MANUAL,
                evidenceStatus = EvidenceStatus.USER_CORRECTED,
                sourceId = "local-user",
            ),
        )
        val repository = selectedSleepRepository()
        viewModelScope.launch {
            repository.appendCorrection(correction)
                .onSuccess { mutableMessage.value = "Manual sleep/wake correction saved." }
                .onFailure { mutableMessage.value = it.message ?: "Correction could not be saved." }
        }
    }

    fun addMedicationEvent(displayName: String, occurredAtText: String) {
        if (!medicationSaveGuard.compareAndSet(false, true)) return
        val requestId = medicationSaveSequence.incrementAndGet()
        val zone = ZoneId.systemDefault()
        val occurredAt = parseLocalDateTime(occurredAtText, zone.id, null)
        if (occurredAt == null) {
            val failure = mutableMessage.value ?: "Medication event could not be saved."
            mutableMedicationSaveState.value = MedicationSaveState.Failed(requestId, failure)
            medicationSaveGuard.set(false)
            return
        }
        val normalizedDisplayName = displayName.trim().ifBlank { "Medication" }
        val retryable = retryableMedicationEvent?.takeIf {
            it.displayName == normalizedDisplayName && it.occurredAt == occurredAt.instant &&
                it.timeZoneId == zone.id
        }
        val event = retryable ?: MedicationEvent(
            id = UUID.randomUUID().toString(),
            displayName = normalizedDisplayName,
            occurredAt = occurredAt.instant,
            timeZoneId = zone.id,
            createdAt = Instant.now(clock),
            provenance = Provenance(
                acquisitionMethod = AcquisitionMethod.MANUAL,
                evidenceStatus = EvidenceStatus.USER_CORRECTED,
                sourceId = "local-user",
            ),
        )
        retryableMedicationEvent = event
        mutableMedicationSaveState.value = MedicationSaveState.Saving(requestId)
        viewModelScope.launch {
            try {
                container.medicationRepository.append(event)
                mutableMedicationSaveState.value = MedicationSaveState.Succeeded(requestId)
                retryableMedicationEvent = null
                mutableMessage.value = "Medication event saved on this device."
            } catch (exception: CancellationException) {
                mutableMedicationSaveState.value = MedicationSaveState.Idle
                throw exception
            } catch (_: Exception) {
                val failure = "Medication event could not be saved."
                mutableMedicationSaveState.value = MedicationSaveState.Failed(requestId, failure)
                mutableMessage.value = failure
            } finally {
                medicationSaveGuard.set(false)
            }
        }
    }

    fun consumeMedicationSaveResult(requestId: Long) {
        val current = mutableMedicationSaveState.value
        if (
            (current is MedicationSaveState.Succeeded && current.requestId == requestId) ||
            (current is MedicationSaveState.Failed && current.requestId == requestId)
        ) {
            mutableMedicationSaveState.value = MedicationSaveState.Idle
        }
    }

    fun clearMessage() {
        mutableMessage.value = null
    }

    private fun parseLocalDateTime(
        value: String,
        ianaTimeZoneId: String?,
        preferredOffset: ZoneOffset?,
    ): ResolvedLocalDateTime? = try {
        val parsed = CORRECTION_INPUT_FORMATTER.parse(value.trim())
        val localDateTime = LocalDateTime.from(parsed)
        val explicitOffset = parsed.query(TemporalQueries.offset())
        resolveLocalDateTimeWithOffset(
            localDateTime = localDateTime,
            ianaTimeZoneId = ianaTimeZoneId,
            preferredOffset = preferredOffset,
            explicitOffset = explicitOffset,
        )
    } catch (exception: AmbiguousLocalTimeException) {
        val choices = exception.validOffsets.joinToString(" or ")
        mutableMessage.value = "That time occurs twice. Add $choices after the time."
        null
    } catch (_: NonexistentLocalTimeException) {
        mutableMessage.value = "That local time does not exist because the clock moves forward."
        null
    } catch (_: InvalidLocalTimeOffsetException) {
        mutableMessage.value = "That UTC offset is not valid for the selected local time."
        null
    } catch (_: DateTimeException) {
        mutableMessage.value = "Use yyyy-MM-dd HH:mm, adding an offset such as -04:00 when required."
        null
    }

    private fun selectedSleepRepository(): SleepRepository =
        when (uiState.value.settings.dataMode) {
            DataMode.FIXTURE -> container.fixtureSleepRepository
            DataMode.HEALTH_CONNECT -> container.healthConnectRepository
        }

    companion object {
        val INPUT_FORMATTER: DateTimeFormatter = DateTimeFormatter.ofPattern("uuuu-MM-dd HH:mm")
        private val CORRECTION_INPUT_FORMATTER: DateTimeFormatter = DateTimeFormatterBuilder()
            .parseStrict()
            .append(INPUT_FORMATTER)
            .optionalStart()
            .appendLiteral(' ')
            .appendOffset("+HH:MM", "Z")
            .optionalEnd()
            .toFormatter()
            .withResolverStyle(ResolverStyle.STRICT)

        fun factory(container: AppDependencies): ViewModelProvider.Factory =
            object : ViewModelProvider.Factory {
                @Suppress("UNCHECKED_CAST")
                override fun <T : ViewModel> create(modelClass: Class<T>): T =
                    AppViewModel(container) as T
            }

        private fun effectiveEpisodes(
            episodes: List<SleepEpisode>,
            activeCorrections: Map<String, SleepCorrection>,
        ): List<EffectiveSleepEpisode> =
            SleepCorrectionPolicy.effectiveAll(episodes, activeCorrections)
                .sortedByDescending { it.start }

        private fun reviewsForEpisodes(
            episodes: List<SleepEpisode>,
            reviews: List<SleepCorrectionReview>,
        ): List<SleepCorrectionReview> {
            val currentIds = episodes.mapTo(HashSet(), SleepEpisode::id)
            return reviews.filter { it.currentEpisodeId in currentIds }
        }
    }
}
