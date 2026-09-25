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
    }

    private companion object {
        const val IMPORT_WORK = "zeitboard-health-import"
        const val UPLOAD_WORK = "zeitboard-sleep-upload"
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
