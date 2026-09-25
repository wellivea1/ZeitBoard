package org.non24.planner.data

import org.non24.planner.domain.BackgroundReadState
import org.non24.planner.domain.DataMode
import org.non24.planner.domain.HealthPermissionState

interface BackgroundSyncScheduler {
    fun reconcile(enabled: Boolean)
    fun requestUpload()
    fun cancelAll()
}

/** Testable orchestration shared by the foreground and durable Android jobs. */
class EvidenceSyncCoordinator(
    private val settings: SettingsRepository,
    private val health: HealthConnectRepository,
    private val sync: BackendSyncRepository,
    private val scheduler: BackgroundSyncScheduler,
    private val initialize: suspend () -> Unit,
) {
    suspend fun reconcileSchedule() {
        scheduler.reconcile(settings.settings.value.backgroundSyncEnabled &&
            settings.settings.value.dataMode == DataMode.HEALTH_CONNECT && sync.isConfigured())
    }

    suspend fun refreshForeground() {
        initialize()
        if (settings.settings.value.dataMode != DataMode.HEALTH_CONNECT) return
        health.refresh()
        queueSavedSleep()
        if (sync.isConfigured()) scheduler.requestUpload()
        reconcileSchedule()
    }

    /** Returns false for a transient import failure; denied permissions never cause a retry storm. */
    suspend fun refreshBackground(): Boolean {
        initialize()
        if (!settings.settings.value.backgroundSyncEnabled ||
            settings.settings.value.dataMode != DataMode.HEALTH_CONNECT || !sync.isConfigured()
        ) return true
        health.refreshPermissionState()
        if (health.permissionState.value == HealthPermissionState.GRANTED &&
            health.backgroundReadState.value == BackgroundReadState.GRANTED
        ) {
            health.refresh()
        }
        // Saved uploads remain authorized by enrollment even if further collection
        // is denied. No Health Connect read occurs without background permission.
        queueSavedSleep()
        scheduler.requestUpload()
        return health.permissionState.value != HealthPermissionState.UNKNOWN &&
            health.backgroundReadState.value != BackgroundReadState.UNKNOWN &&
            (health.permissionState.value != HealthPermissionState.GRANTED ||
                health.backgroundReadState.value != BackgroundReadState.GRANTED || health.lastRefreshError.value == null)
    }

    suspend fun uploadNow(): Result<Int> {
        initialize()
        if (settings.settings.value.dataMode != DataMode.HEALTH_CONNECT) {
            return Result.failure(IllegalStateException("Select My data to sync."))
        }
        health.refresh()
        queueSavedSleep()
        return sync.synchronize()
    }

    private suspend fun queueSavedSleep() {
        if (settings.settings.value.dataMode == DataMode.HEALTH_CONNECT &&
            health.permissionState.value == HealthPermissionState.GRANTED && sync.isConfigured()
        ) {
            sync.enqueue(health.sourceEpisodes.value)
            if (sync.status.value.queuedCount > 0) scheduler.requestUpload()
        }
    }
}

/** At most five backoff attempts per run; the next periodic import can retry later. */
internal fun shouldRetrySync(error: Throwable?, runAttemptCount: Int): Boolean =
    runAttemptCount < 4 && when (error) {
        is SyncServerResetException -> false
        is BackendSyncException -> error.status == 408 || error.status == 429 || error.status in 500..599
        else -> true
    }
