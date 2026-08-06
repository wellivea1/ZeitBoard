package org.non24.planner.data

import java.time.Instant
import java.time.ZoneId
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import org.non24.planner.domain.SleepEpisode

/** Where the device is enrolled, and with what. */
data class SyncConfig(
    val baseUrl: String,
    val token: String,
    val homeZoneId: String,
) {
    fun homeZone(): ZoneId = ZoneId.of(homeZoneId)
}

/** Persists enrollment. Kept separate so the token store can be hardened later. */
interface SyncConfigStore {
    fun load(): SyncConfig?

    fun save(config: SyncConfig)

    fun clear()
}

/**
 * Moves Health Connect sleep to the user's own server.
 *
 * The order matters and is the whole design: map, enqueue durably, then push.
 * Mapping straight into a request would lose evidence whenever the phone is
 * killed mid-flight, which on Android is routine rather than exceptional.
 *
 * This deliberately implements no estimator. Android observes and forwards;
 * the rhythm is computed once, in the shared Go core, so the two devices cannot
 * disagree about what the user's rhythm is.
 */
class BackendSyncRepository(
    private val outbox: SyncOutboxStore,
    private val configStore: SyncConfigStore,
    private val client: BackendSyncClient,
    private val now: () -> Instant = Instant::now,
) {
    private val mutex = Mutex()
    private val statusState = MutableStateFlow(SyncStatus())

    val status: StateFlow<SyncStatus> = statusState.asStateFlow()

    /** Reads persisted state so the UI opens with the truth, not with OFF. */
    suspend fun initialise() = mutex.withLock {
        val config = configStore.load()
        if (config == null) {
            statusState.value = SyncStatus(state = SyncState.OFF)
            return@withLock
        }
        val pending = outbox.pendingCount()
        statusState.value = SyncStatus(
            state = if (pending > 0) SyncState.QUEUED else SyncState.SYNCED,
            queuedCount = pending,
            lastSyncedAt = outbox.lastSyncedAt(),
        )
    }

    suspend fun enroll(baseUrl: String, enrollmentSecret: String, homeZoneId: String, label: String): Result<Unit> =
        mutex.withLock {
            // Re-enrolling against a different instance must not push records
            // captured for the previous one.
            val existing = configStore.load()
            if (existing != null && existing.baseUrl != baseUrl.trimEnd('/')) {
                outbox.clear()
            }
            client.enroll(baseUrl, enrollmentSecret, label).map { token ->
                configStore.save(
                    SyncConfig(
                        baseUrl = baseUrl.trimEnd('/'),
                        token = token,
                        homeZoneId = homeZoneId,
                    ),
                )
                statusState.value = SyncStatus(
                    state = SyncState.SYNCED,
                    queuedCount = outbox.pendingCount(),
                    lastSyncedAt = outbox.lastSyncedAt(),
                )
            }
        }

    /** Turns sync off and forgets the queue and the token. */
    suspend fun disable() = mutex.withLock {
        outbox.clear()
        configStore.clear()
        statusState.value = SyncStatus(state = SyncState.OFF)
    }

    /**
     * Maps the supplied episodes into the outbox. Safe to call on every Health
     * Connect refresh: unchanged episodes produce no records.
     */
    suspend fun enqueue(episodes: List<SleepEpisode>): Int = mutex.withLock {
        val config = configStore.load() ?: return@withLock 0
        val mapping = SyncContract.map(
            episodes = episodes,
            homeZone = config.homeZone(),
            alreadySynced = outbox.syncedRevisions(),
            now = now(),
        )
        outbox.enqueue(mapping.records)
        val pending = outbox.pendingCount()
        statusState.value = statusState.value.copy(
            state = if (pending > 0) SyncState.QUEUED else statusState.value.state,
            queuedCount = pending,
            heldCount = mapping.held.size,
        )
        mapping.records.size
    }

    /**
     * Pushes queued records in bounded batches.
     *
     * A failure leaves the queue intact and reports it. Retrying is always safe:
     * record ids are derived from source identity, so the server sees the same
     * id twice and treats the second as the duplicate it is.
     */
    suspend fun push(): Result<Int> = mutex.withLock {
        val config = configStore.load()
            ?: return@withLock Result.failure(IllegalStateException("Sync is not configured."))

        statusState.value = statusState.value.copy(state = SyncState.SYNCING)
        var accepted = 0

        while (true) {
            val batch = outbox.pending(SYNC_BATCH_LIMIT)
            if (batch.isEmpty()) break

            val result = client.push(config.baseUrl, config.token, batch)
            val ids = result.getOrElse { error ->
                // Keep the last successful time visible. "Nothing since
                // Tuesday" is what the user needs, not the transport message.
                statusState.value = statusState.value.copy(
                    state = SyncState.ERROR,
                    queuedCount = outbox.pendingCount(),
                    lastError = error.message ?: "Sync could not reach the server.",
                )
                return@withLock Result.failure(error)
            }
            outbox.markSynced(ids, now())
            accepted += ids.size
        }

        statusState.value = SyncStatus(
            state = SyncState.SYNCED,
            queuedCount = 0,
            heldCount = statusState.value.heldCount,
            lastSyncedAt = outbox.lastSyncedAt() ?: now(),
            lastError = null,
        )
        Result.success(accepted)
    }

    /** Convenience for the refresh path: enqueue then push. */
    suspend fun syncNow(episodes: List<SleepEpisode>): Result<Int> {
        enqueue(episodes)
        return push()
    }
}
