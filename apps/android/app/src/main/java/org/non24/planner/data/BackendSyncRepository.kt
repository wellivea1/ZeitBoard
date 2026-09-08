package org.non24.planner.data

import java.time.Instant
import java.time.ZoneId
import java.util.UUID
import java.security.MessageDigest
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import org.non24.planner.domain.SleepEpisode

data class SyncConfig(
    val baseUrl: String,
    val token: String,
    val homeZoneId: String,
    val queueScope: String,
) {
    fun homeZone(): ZoneId = ZoneId.of(homeZoneId)
}

interface SyncConfigStore {
    fun load(): SyncConfig?
    fun save(config: SyncConfig)
    fun clear()
}

const val MAX_SYNC_BATCHES = 4

/** One process-wide repository serializes enrollment, durable mapping and bounded uploads. */
class BackendSyncRepository(
    private val outbox: SyncOutboxStore,
    private val configStore: SyncConfigStore,
    private val client: BackendSyncClient,
    private val now: () -> Instant = Instant::now,
    private val io: CoroutineDispatcher = Dispatchers.IO,
    private val replica: SyncReplicaStore,
    private val onRecordsChanged: suspend (Boolean) -> Unit = {},
) {
    private val mutex = Mutex()
    private val statusState = MutableStateFlow(SyncStatus())
    val status: StateFlow<SyncStatus> = statusState.asStateFlow()
    private val companionState = MutableStateFlow(CompanionState())
    val companion: StateFlow<CompanionState> = companionState.asStateFlow()
    private val reviewState = MutableStateFlow(SleepReviewState())
    val sleepReview: StateFlow<SleepReviewState> = reviewState.asStateFlow()
    private var reviewScope: String? = null

    private suspend fun <T> locked(block: suspend () -> T): T =
        withContext(io) { mutex.withLock { block() } }

    private fun configuration(): SyncConfig? = configStore.load()?.also {
        outbox.activateScope(it.queueScope)
        val tokenDigest = MessageDigest.getInstance("SHA-256").digest(it.token.toByteArray()).joinToString("") { byte -> "%02x".format(byte) }
        val scope = it.queueScope + ":" + tokenDigest
        if (reviewScope != scope) { reviewState.value = SleepReviewState(); reviewScope = scope }
        replica.activateScope(scope)
    }

    suspend fun isConfigured(): Boolean = locked { configuration() != null }

    suspend fun initialise() = locked {
        val config = configuration()
        if (config == null) {
            replica.clear()
            statusState.value = SyncStatus()
            companionState.value = CompanionState()
        } else {
            publishIdle(config)
            companionState.value = replica.state()
        }
    }

    suspend fun enroll(baseUrl: String, enrollmentSecret: String, homeZoneId: String, label: String): Result<Unit> =
        locked {
            syncResult {
                val address = normalizeSyncUrl(baseUrl)
                val zone = ZoneId.of(homeZoneId.trim())
                val existing = configStore.load()
                // No durable state changes until enrollment succeeds.
                val token = client.enroll(address, enrollmentSecret, label.trim()).getOrThrow()
                val sameServer = existing != null && normalizeSyncUrl(existing.baseUrl) == address
                val config = SyncConfig(
                    baseUrl = address,
                    token = token,
                    homeZoneId = zone.id,
                    queueScope = if (sameServer) existing.queueScope else UUID.randomUUID().toString(),
                )
                // The saved generation makes a crash between these steps safe:
                // every queue access reselects it and discards obsolete generations.
                configStore.save(config)
                companionState.value = CompanionState()
                outbox.activateScope(config.queueScope)
                configuration()
                companionState.value = replica.state()
                publishIdle(config)
            }
        }

    suspend fun disable() = locked {
        // Forget authorization first, so a killed process cannot resume uploads.
        configStore.clear()
        outbox.clear()
        replica.clear()
        companionState.value = CompanionState()
        statusState.value = SyncStatus()
        reviewState.value = SleepReviewState()
        reviewScope = null
    }

    suspend fun loadSleepReview(observationId: String): Result<Unit> = locked {
        val result = syncResult {
            val config = configuration() ?: error("Not enrolled")
            require(validSyncId(observationId))
            if (outbox.hasPendingManualCorrection(observationId)) {
                reviewState.value = SleepReviewState(selectedId = observationId, pending = true)
                return@syncResult
            }
            reviewState.value = SleepReviewState(selectedId = observationId, context = replica.cachedReview(observationId), loading = true)
            val raw = client.sleepReview(config.baseUrl, config.token, observationId).getOrThrow()
            require(parseSleepReview(raw).observationId == observationId)
            replica.cacheReview(raw)
            reviewState.value = SleepReviewState(selectedId = observationId, context = parseSleepReview(raw))
        }
        result.onFailure {
            val missing = it is BackendSyncException && it.status == 404
            reviewState.value = reviewState.value.copy(loading = false, context = if (missing) null else reviewState.value.context,
                error = if (missing) "This sleep record is unavailable. Sync to refresh the list." else "Review could not refresh. A saved review can be used offline; sync and retry if records have changed.")
        }
        result
    }

    suspend fun saveSleepReview(review: SleepReview, start: Instant, end: Instant, classification: String, excluded: Boolean): Result<Unit> = locked {
        val result = syncResult {
            val config = configuration() ?: error("Not enrolled")
            val state = reviewState.value
            check(state.context === review && !state.stale && !state.loading && !outbox.hasPendingManualCorrection(review.observationId))
            val record = manualCorrection(review, "cor-user-" + UUID.randomUUID().toString(), now(), start, end, classification, excluded)
            outbox.enqueue(listOf(record))
            check(outbox.contains(record.recordId)) { "This observation has been erased." }
            reviewState.value = SleepReviewState(selectedId = review.observationId, pending = true)
            publishIdle(config)
        }
        result.onFailure { reviewState.value = reviewState.value.copy(error = "Correction could not be saved. Keep your changes, sync and reload the review before trying again.") }
        result
    }

    suspend fun enqueue(episodes: List<SleepEpisode>): Int = locked {
        val config = configuration() ?: return@locked 0
        enqueueLocked(config, episodes)
    }

    private fun enqueueLocked(config: SyncConfig, episodes: List<SleepEpisode>): Int {
        val known = replica.knownSources().toMutableMap()
        outbox.knownSources().forEach { (id, revision) -> if (known[id]?.revision?.isAfter(revision.revision) != true) known[id] = revision }
        val mapping = SyncContract.map(episodes, config.homeZone(), known, now())
        val before = outbox.pendingCount()
        outbox.enqueue(mapping.records)
        publishIdle(config, mapping.held.size)
        return outbox.pendingCount() - before
    }

    /** Download erasures before uploading; only cache a projection matching the complete local cursor. */
    suspend fun synchronize(): Result<Int> = locked {
        val config = configuration()
            ?: return@locked Result.failure(IllegalStateException("Sync is not configured."))
        val cache = replica
        statusState.value = statusState.value.copy(state = SyncState.SYNCING, lastError = null)
        val result = try { syncResult {
            suspend fun pullUntilCaughtUp() {
                repeat(4) {
                    val page = client.pull(config.baseUrl, config.token, cache.cursor()).getOrThrow()
                    try { cache.apply(page, now()) } finally {
                        if (page.records.isNotEmpty()) {
                            val erased = page.records.any { it.kind == "tombstone" }
                            val selected = reviewState.value.selectedId
                            if (erased) reviewState.value = SleepReviewState(error = if (selected != null) "Downloaded erasures cleared the open review. Select a remaining source to continue." else null)
                            else if (reviewState.value.context != null && page.records.any { it.recordId == selected || (it.kind == "correction" && it.payload.string("target_observation_id") == selected) }) {
                                reviewState.value = reviewState.value.copy(stale = true)
                            }
                            onRecordsChanged(erased)
                        }
                    }
                    companionState.value = cache.state()
                    if (page.records.size < SYNC_PULL_LIMIT) return
                }
                throw SyncMorePagesException()
            }
            pullUntilCaughtUp()
            // Validate server generation before replaying locally accepted data.
            val beforeUpload = parseCompanion(client.companion(config.baseUrl, config.token).getOrThrow())
            if (beforeUpload.cursor < cache.cursor()) throw SyncServerResetException()
            if (beforeUpload.cursor > cache.cursor()) throw SyncMorePagesException()
            outbox.reconcileAccepted()
            val uploaded = pushLocked(config)
            if (outbox.pendingCount() > 0) throw SyncMorePagesException()
            pullUntilCaughtUp()
            val projection = client.companion(config.baseUrl, config.token).getOrThrow()
            val projectionCursor = parseCompanion(projection).cursor
            if (projectionCursor < cache.cursor()) throw SyncServerResetException()
            if (projectionCursor > cache.cursor()) throw SyncMorePagesException()
            cache.cache(projection, now())
            companionState.value = cache.state()
            publishIdle(config)
            reviewState.value.selectedId?.let { id -> reviewState.value = reviewState.value.copy(pending = outbox.hasPendingManualCorrection(id)) }
            uploaded
        } } catch (cancelled: CancellationException) {
            publishIdle(config)
            companionState.value = runCatching { cache.state() }.getOrDefault(CompanionState())
            throw cancelled
        }
        if (result.isFailure) {
            val error = result.exceptionOrNull()!!
            val message = if (error is SyncMorePagesException) "More records are waiting. Sync will continue on the next attempt." else safeSyncError(error)
            companionState.value = runCatching { cache.state() }.getOrDefault(CompanionState()).copy(error = message)
            statusState.value = statusState.value.copy(state = if (error is SyncMorePagesException) SyncState.QUEUED else SyncState.ERROR,
                queuedCount = outbox.pendingCount(), lastError = message)
        }
        result
    }

    private suspend fun pushLocked(config: SyncConfig): Int {
        var accepted = 0
        repeat(MAX_SYNC_BATCHES) {
            val batch = outbox.prepareBatch(SYNC_BATCH_LIMIT, replica.knownSources())
            if (batch.isEmpty()) {
                if (outbox.pendingCount() == 0) return accepted
            } else {
                val ids = client.push(config.baseUrl, config.token, batch).getOrThrow()
                require(ids.size == batch.size && ids.toSet() == batch.map { it.recordId }.toSet()) {
                    "The server did not acknowledge the complete batch."
                }
                outbox.markSynced(ids, now())
                accepted += ids.size
            }
        }
        return accepted
    }

    private fun publishIdle(config: SyncConfig, held: Int = statusState.value.heldCount) {
        val pending = outbox.pendingCount()
        val lastUpload = outbox.lastSyncedAt()
        statusState.value = SyncStatus(
            state = when {
                pending > 0 -> SyncState.QUEUED
                lastUpload != null -> SyncState.SYNCED
                else -> SyncState.READY
            },
            queuedCount = pending,
            heldCount = held,
            lastSyncedAt = lastUpload,
            serverUrl = config.baseUrl,
        )
    }


}

internal class SyncMorePagesException : Exception("More sync work remains.")
internal class SyncServerResetException : Exception("The server history moved backwards.")

/** Transport/parser exceptions may contain private input; UI messages are deliberately bounded. */
internal fun safeSyncError(error: Throwable): String = when (error) {
    is BackendSyncException -> if (error.status != 200) error.message.orEmpty() else "The server returned an invalid response."
    is SyncServerResetException -> "The server history changed after a restore. Re-enroll this phone in Settings to download it again; saved local uploads will be reconciled."
    else -> "Sync could not finish. Saved records are retained; check the server address and connection, then retry."
}
