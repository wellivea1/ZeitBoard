package org.non24.planner.data

import android.content.Context
import androidx.work.BackoffPolicy
import androidx.work.Constraints
import androidx.work.CoroutineWorker
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.ExistingWorkPolicy
import androidx.work.NetworkType
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.withTimeoutOrNull
import org.non24.planner.ZeitBoardApplication

class AndroidBackgroundSyncScheduler(context: Context) : BackgroundSyncScheduler {
    private val manager = WorkManager.getInstance(context.applicationContext)

    override fun reconcile(enabled: Boolean) {
        if (!enabled) {
            manager.cancelUniqueWork(IMPORT_WORK)
            return
        }
        manager.enqueueUniquePeriodicWork(
            IMPORT_WORK,
            ExistingPeriodicWorkPolicy.KEEP,
            PeriodicWorkRequestBuilder<HealthConnectImportWorker>(1, TimeUnit.HOURS)
                .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 1, TimeUnit.MINUTES)
                .build(),
        )
    }

    override fun requestUpload() {
        manager.enqueueUniqueWork(
            UPLOAD_WORK,
            ExistingWorkPolicy.KEEP,
            OneTimeWorkRequestBuilder<BackendUploadWorker>()
                .setConstraints(Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build())
                .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 1, TimeUnit.MINUTES)
                .build(),
        )
    }

    override fun cancelAll() {
        manager.cancelUniqueWork(IMPORT_WORK)
        manager.cancelUniqueWork(UPLOAD_WORK)
        manager.cancelUniqueWork(NOTICE_WORK)
    }

    // Android runs periodic work at most every 15 minutes, which is how soon a
    // notice can follow a request; battery settings can stretch it further.
    override fun reconcileNotices(enabled: Boolean) {
        if (!enabled) {
            manager.cancelUniqueWork(NOTICE_WORK)
            return
        }
        manager.enqueueUniquePeriodicWork(
            NOTICE_WORK,
            ExistingPeriodicWorkPolicy.KEEP,
            PeriodicWorkRequestBuilder<TimeRequestNoticeWorker>(15, TimeUnit.MINUTES)
                .setConstraints(Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build())
                .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 1, TimeUnit.MINUTES)
                .build(),
        )
    }

    private companion object {
        const val IMPORT_WORK = "zeitboard-health-import"
        const val UPLOAD_WORK = "zeitboard-sleep-upload"
        const val NOTICE_WORK = "zeitboard-time-request-notices"
    }
}

class TimeRequestNoticeWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result = try {
        val container = (applicationContext as ZeitBoardApplication).container
        if (!container.settingsRepository.settings.value.timeRequestNotices) {
            Result.success()
        } else {
            val checked = withTimeoutOrNull(60_000) { container.timeRequestNotifier.check() }
            val error = checked?.exceptionOrNull()
            if ((checked == null || error != null) && shouldRetrySync(error, runAttemptCount)) Result.retry() else Result.success()
        }
    } catch (cancelled: CancellationException) {
        throw cancelled
    } catch (_: Exception) {
        // The next period tries again; nothing about a request is logged.
        Result.success()
    }
}

class HealthConnectImportWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result = try {
        val container = (applicationContext as ZeitBoardApplication).container
        val complete = withTimeoutOrNull(240_000) { container.evidenceSync.refreshBackground() } ?: false
        if (!complete && shouldRetrySync(null, runAttemptCount)) Result.retry() else Result.success()
    } catch (cancelled: CancellationException) {
        throw cancelled
    } catch (_: Exception) {
        // Never pass exception text or health-derived values to WorkManager logs/output.
        if (shouldRetrySync(null, runAttemptCount)) Result.retry() else Result.failure()
    }
}

class BackendUploadWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result = try {
        val container = (applicationContext as ZeitBoardApplication).container
        val sync = container.backendSyncRepository
        if (!sync.isConfigured()) {
            Result.success()
        } else {
            val uploaded = withTimeoutOrNull(240_000) { sync.synchronize() }
            val pending = sync.status.value.queuedCount > 0
            if ((uploaded == null || uploaded.isFailure || pending) &&
                shouldRetrySync(uploaded?.exceptionOrNull(), runAttemptCount)
            ) Result.retry() else if (uploaded?.isFailure == true) Result.failure() else Result.success()
        }
    } catch (cancelled: CancellationException) {
        throw cancelled
    } catch (_: Exception) {
        if (shouldRetrySync(null, runAttemptCount)) Result.retry() else Result.failure()
    }
}
