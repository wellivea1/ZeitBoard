package org.non24.planner.data

import android.content.Context
import androidx.health.connect.client.HealthConnectClient
import androidx.health.connect.client.records.SleepSessionRecord
import androidx.health.connect.client.request.ReadRecordsRequest
import androidx.health.connect.client.time.TimeRangeFilter
import java.time.Clock
import java.time.Duration
import java.time.Instant
import java.time.ZoneId
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import org.non24.planner.domain.AcquisitionMethod
import org.non24.planner.domain.EvidenceStatus
import org.non24.planner.domain.HealthConnectAvailability
import org.non24.planner.domain.HealthPermissionState
import org.non24.planner.domain.Provenance
import org.non24.planner.domain.SleepEpisode

object HealthConnectPermissions {
    const val READ_SLEEP = "android.permission.health.READ_SLEEP"
    val required: Set<String> = setOf(READ_SLEEP)
}

interface HealthConnectClientAdapter {
    fun availability(): HealthConnectAvailability

    suspend fun grantedPermissions(): Set<String>

    suspend fun readRecentSleep(): List<SleepEpisode>
}

class HealthConnectSleepRepository(
    private val client: HealthConnectClientAdapter,
) : CorrectableSleepRepository(), HealthConnectRepository {
    private val mutableAvailability = MutableStateFlow(client.availability())
    private val mutablePermissionState = MutableStateFlow(initialPermissionState(mutableAvailability.value))

    override val availability: StateFlow<HealthConnectAvailability> = mutableAvailability.asStateFlow()
    override val permissionState: StateFlow<HealthPermissionState> = mutablePermissionState.asStateFlow()
    override val requiredPermissions: Set<String> = HealthConnectPermissions.required

    override suspend fun refreshPermissionState() {
        mutableAvailability.value = client.availability()
        if (mutableAvailability.value != HealthConnectAvailability.AVAILABLE) {
            mutablePermissionState.value = HealthPermissionState.UNAVAILABLE
            mutableSourceEpisodes.value = emptyList()
            return
        }
        val granted = client.grantedPermissions()
        mutablePermissionState.value = if (granted.containsAll(requiredPermissions)) {
            HealthPermissionState.GRANTED
        } else {
            HealthPermissionState.REQUIRED
        }
    }

    override suspend fun onPermissionResult(grantedPermissions: Set<String>) {
        mutablePermissionState.value = if (grantedPermissions.containsAll(requiredPermissions)) {
            HealthPermissionState.GRANTED
        } else {
            HealthPermissionState.REQUIRED
        }
        if (mutablePermissionState.value == HealthPermissionState.GRANTED) {
            refresh()
        }
    }

    override suspend fun refresh() {
        refreshPermissionState()
        if (permissionState.value != HealthPermissionState.GRANTED) {
            return
        }
        try {
            mutableSourceEpisodes.value = client.readRecentSleep().sortedByDescending { it.start }
        } catch (_: SecurityException) {
            mutablePermissionState.value = HealthPermissionState.REQUIRED
            mutableSourceEpisodes.value = emptyList()
        }
    }

    private companion object {
        fun initialPermissionState(availability: HealthConnectAvailability): HealthPermissionState =
            if (availability == HealthConnectAvailability.AVAILABLE) {
                HealthPermissionState.UNKNOWN
            } else {
                HealthPermissionState.UNAVAILABLE
            }
    }
}

class AndroidHealthConnectClientAdapter(
    private val context: Context,
    private val clock: Clock = Clock.systemUTC(),
) : HealthConnectClientAdapter {
    override fun availability(): HealthConnectAvailability =
        when (HealthConnectClient.getSdkStatus(context, HEALTH_CONNECT_PACKAGE)) {
            HealthConnectClient.SDK_AVAILABLE -> HealthConnectAvailability.AVAILABLE
            HealthConnectClient.SDK_UNAVAILABLE_PROVIDER_UPDATE_REQUIRED -> HealthConnectAvailability.UPDATE_REQUIRED
            else -> HealthConnectAvailability.UNAVAILABLE
        }

    override suspend fun grantedPermissions(): Set<String> {
        val healthClient = availableClient() ?: return emptySet()
        return healthClient.permissionController.getGrantedPermissions()
    }

    override suspend fun readRecentSleep(): List<SleepEpisode> {
        val healthClient = availableClient() ?: return emptyList()
        val now = Instant.now(clock)
        val response = healthClient.readRecords(
            ReadRecordsRequest(
                recordType = SleepSessionRecord::class,
                timeRangeFilter = TimeRangeFilter.between(now.minus(Duration.ofDays(30)), now),
            ),
        )
        val localZoneId = ZoneId.systemDefault().id
        return response.records.map { record ->
            SleepEpisode(
                id = record.metadata.id.ifBlank {
                    "health-connect-${record.startTime}-${record.endTime}"
                },
                start = record.startTime,
                end = record.endTime,
                timeZoneId = localZoneId,
                provenance = Provenance(
                    acquisitionMethod = AcquisitionMethod.HEALTH_CONNECT,
                    evidenceStatus = EvidenceStatus.IMPORTED,
                    sourceId = record.metadata.dataOrigin.packageName,
                ),
            )
        }
    }

    private fun availableClient(): HealthConnectClient? =
        if (availability() == HealthConnectAvailability.AVAILABLE) {
            HealthConnectClient.getOrCreate(context)
        } else {
            null
        }

    private companion object {
        const val HEALTH_CONNECT_PACKAGE = "com.google.android.apps.healthdata"
    }
}
