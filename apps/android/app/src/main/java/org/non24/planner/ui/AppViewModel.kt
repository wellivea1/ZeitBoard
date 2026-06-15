package org.non24.planner.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import java.time.Clock
import java.time.Instant
import java.time.LocalDateTime
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.DateTimeParseException
import java.util.UUID
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import org.non24.planner.AppContainer
import org.non24.planner.data.SleepRepository
import org.non24.planner.domain.AcquisitionMethod
import org.non24.planner.domain.AppSettings
import org.non24.planner.domain.DataMode
import org.non24.planner.domain.EffectiveSleepEpisode
import org.non24.planner.domain.EstimateSnapshot
import org.non24.planner.domain.EvidenceStatus
import org.non24.planner.domain.HealthConnectAvailability
import org.non24.planner.domain.HealthPermissionState
import org.non24.planner.domain.MedicationEvent
import org.non24.planner.domain.Provenance
import org.non24.planner.domain.SleepCorrection
import org.non24.planner.domain.SleepCorrectionPolicy
import org.non24.planner.domain.SleepEpisode

data class AppUiState(
    val settings: AppSettings = AppSettings(),
    val sleepEpisodes: List<EffectiveSleepEpisode> = emptyList(),
    val estimate: EstimateSnapshot? = null,
    val medicationEvents: List<MedicationEvent> = emptyList(),
    val healthAvailability: HealthConnectAvailability = HealthConnectAvailability.UNAVAILABLE,
    val healthPermissionState: HealthPermissionState = HealthPermissionState.UNKNOWN,
) {
    val latestSleepEpisode: EffectiveSleepEpisode?
        get() = sleepEpisodes.maxByOrNull { it.start }
}

private data class SleepState(
    val episodes: List<EffectiveSleepEpisode>,
    val estimate: EstimateSnapshot?,
)

private data class HealthState(
    val episodes: List<EffectiveSleepEpisode>,
    val availability: HealthConnectAvailability,
    val permissionState: HealthPermissionState,
)

class AppViewModel(
    private val container: AppContainer,
    private val clock: Clock = Clock.systemUTC(),
) : ViewModel() {
    private val mutableMessage = MutableStateFlow<String?>(null)
    val message: StateFlow<String?> = mutableMessage.asStateFlow()

    private val fixtureState = combine(
        container.fixtureSleepRepository.sourceEpisodes,
        container.fixtureSleepRepository.corrections,
        container.fixtureEstimateRepository.estimate,
    ) { episodes, corrections, estimate ->
        SleepState(
            episodes = effectiveEpisodes(episodes, corrections),
            estimate = estimate,
        )
    }

    private val healthState = combine(
        container.healthConnectRepository.sourceEpisodes,
        container.healthConnectRepository.corrections,
        container.healthConnectRepository.availability,
        container.healthConnectRepository.permissionState,
    ) { episodes, corrections, availability, permissionState ->
        HealthState(
            episodes = effectiveEpisodes(episodes, corrections),
            availability = availability,
            permissionState = permissionState,
        )
    }

    val uiState: StateFlow<AppUiState> = combine(
        container.settingsRepository.settings,
        fixtureState,
        healthState,
        container.medicationRepository.events,
    ) { settings, fixture, health, medications ->
        val activeEpisodes = when (settings.dataMode) {
            DataMode.FIXTURE -> fixture.episodes
            DataMode.HEALTH_CONNECT -> health.episodes
        }
        AppUiState(
            settings = settings,
            sleepEpisodes = activeEpisodes,
            estimate = if (settings.dataMode == DataMode.FIXTURE) fixture.estimate else null,
            medicationEvents = medications,
            healthAvailability = health.availability,
            healthPermissionState = health.permissionState,
        )
    }.stateIn(
        scope = viewModelScope,
        started = SharingStarted.WhileSubscribed(5_000),
        initialValue = AppUiState(),
    )

    init {
        viewModelScope.launch {
            container.healthConnectRepository.refreshPermissionState()
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
            mutableMessage.value = when (container.healthConnectRepository.permissionState.value) {
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
        val zone = runCatching { ZoneId.of(latest.timeZoneId) }.getOrDefault(ZoneId.systemDefault())
        val correctedStart = parseLocalDateTime(startText, zone) ?: return
        val correctedEnd = parseLocalDateTime(endText, zone) ?: return
        val correction = SleepCorrection(
            id = UUID.randomUUID().toString(),
            targetEpisodeId = latest.source.id,
            correctedStart = correctedStart,
            correctedEnd = correctedEnd,
            timeZoneId = zone.id,
            createdAt = Instant.now(clock),
            provenance = Provenance(
                acquisitionMethod = AcquisitionMethod.MANUAL,
                evidenceStatus = EvidenceStatus.USER_CORRECTED,
                sourceId = "local-user",
            ),
        )
        viewModelScope.launch {
            selectedSleepRepository().appendCorrection(correction)
                .onSuccess { mutableMessage.value = "Manual sleep/wake correction saved." }
                .onFailure { mutableMessage.value = it.message ?: "Correction could not be saved." }
        }
    }

    fun addMedicationEvent(displayName: String, occurredAtText: String) {
        val zone = ZoneId.systemDefault()
        val occurredAt = parseLocalDateTime(occurredAtText, zone) ?: return
        val event = MedicationEvent(
            id = UUID.randomUUID().toString(),
            displayName = displayName.trim().ifBlank { "Medication" },
            occurredAt = occurredAt,
            timeZoneId = zone.id,
            createdAt = Instant.now(clock),
            provenance = Provenance(
                acquisitionMethod = AcquisitionMethod.MANUAL,
                evidenceStatus = EvidenceStatus.USER_CORRECTED,
                sourceId = "local-user",
            ),
        )
        viewModelScope.launch {
            container.medicationRepository.append(event)
            mutableMessage.value = "Medication event saved locally."
        }
    }

    fun clearMessage() {
        mutableMessage.value = null
    }

    private fun parseLocalDateTime(value: String, zoneId: ZoneId): Instant? =
        try {
            LocalDateTime.parse(value.trim(), INPUT_FORMATTER).atZone(zoneId).toInstant()
        } catch (_: DateTimeParseException) {
            mutableMessage.value = "Use date and time format yyyy-MM-dd HH:mm."
            null
        }

    private fun selectedSleepRepository(): SleepRepository =
        when (uiState.value.settings.dataMode) {
            DataMode.FIXTURE -> container.fixtureSleepRepository
            DataMode.HEALTH_CONNECT -> container.healthConnectRepository
        }

    companion object {
        val INPUT_FORMATTER: DateTimeFormatter = DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm")

        fun factory(container: AppContainer): ViewModelProvider.Factory =
            object : ViewModelProvider.Factory {
                @Suppress("UNCHECKED_CAST")
                override fun <T : ViewModel> create(modelClass: Class<T>): T =
                    AppViewModel(container) as T
            }

        private fun effectiveEpisodes(
            episodes: List<SleepEpisode>,
            corrections: List<SleepCorrection>,
        ): List<EffectiveSleepEpisode> =
            episodes
                .map { SleepCorrectionPolicy.effective(it, corrections) }
                .sortedByDescending { it.start }
    }
}
