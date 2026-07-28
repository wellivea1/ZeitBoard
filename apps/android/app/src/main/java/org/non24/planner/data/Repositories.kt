package org.non24.planner.data

import kotlinx.coroutines.flow.StateFlow
import org.non24.planner.domain.AppSettings
import org.non24.planner.domain.EstimateSnapshot
import org.non24.planner.domain.HealthConnectAvailability
import org.non24.planner.domain.HealthPermissionState
import org.non24.planner.domain.MedicationEvent
import org.non24.planner.domain.SleepCorrection
import org.non24.planner.domain.SleepCorrectionReview
import org.non24.planner.domain.SleepEpisode

interface SleepRepository {
    val sourceEpisodes: StateFlow<List<SleepEpisode>>
    val activeCorrections: StateFlow<Map<String, SleepCorrection>>
    val correctionReviews: StateFlow<List<SleepCorrectionReview>>

    suspend fun refresh()

    suspend fun appendCorrection(correction: SleepCorrection): Result<Unit>
}

interface EstimateRepository {
    val estimate: StateFlow<EstimateSnapshot?>
}

interface MedicationRepository {
    val events: StateFlow<List<MedicationEvent>>

    suspend fun append(event: MedicationEvent)
}

interface SettingsRepository {
    val settings: StateFlow<AppSettings>

    suspend fun update(transform: (AppSettings) -> AppSettings)
}

interface HealthConnectRepository : SleepRepository {
    val availability: StateFlow<HealthConnectAvailability>
    val permissionState: StateFlow<HealthPermissionState>
    val requiredPermissions: Set<String>
    val lastRefreshError: StateFlow<String?>

    suspend fun refreshPermissionState()

    suspend fun onPermissionResult(grantedPermissions: Set<String>)
}
