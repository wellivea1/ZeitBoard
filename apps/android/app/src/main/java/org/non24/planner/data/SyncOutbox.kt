package org.non24.planner.data

import java.time.Instant

/**
 * One record waiting to reach the user's own server.
 *
 * The outbox is durable because the alternative is losing evidence to a process
 * death on a phone, which happens constantly. A record enqueued here survives
 * until the server has accepted it, and is only then marked synced.
 */
data class OutboxRecord(
    val recordId: String,
    val kind: String,
    val createdAt: Instant,
    /** The source revision this record represents, used for idempotency. */
    val sourceRevision: Instant,
    val payload: String,
)

/** Includes queued revisions: a source can change again before its first upload. */
data class SourceSyncRevision(val revision: Instant, val correctionId: String? = null)

/**
 * What the user is told about sync. These are deliberately distinguishable:
 * "queued" and "error" mean different things to someone deciding whether to
 * trust what the desktop is showing, and collapsing them into a spinner would
 * hide a backend that has been unreachable for a week.
 */
enum class SyncState {
    /** Sync is not configured. Local-only use is a supported mode, not a fault. */
    OFF,

    /** Enrolled, but no successful upload is recorded. */
    READY,

    /** Records are waiting; nothing has failed. */
    QUEUED,

    /** A push is in flight. */
    SYNCING,

    /** Everything durable has been accepted by the server. */
    SYNCED,

    /** The last attempt failed. The queue is intact and will be retried. */
    ERROR,
}

/**
 * What the UI renders. It keeps the last successful sync time even while in
 * ERROR, because "nothing since Tuesday" is the useful fact — not the transport
 * error text.
 */
data class SyncStatus(
    val state: SyncState = SyncState.OFF,
    val queuedCount: Int = 0,
    val heldCount: Int = 0,
    val lastSyncedAt: Instant? = null,
    val lastError: String? = null,
    val serverUrl: String? = null,
) {
    /**
     * Queue bookkeeping only. This is never evidence of estimate freshness
     * or of whether Health Connect has supplied newer records.
     */
    val hasUploadedKnownRecords: Boolean
        get() = state == SyncState.SYNCED && queuedCount == 0 && heldCount == 0 && lastSyncedAt != null
}

/** Durable storage for the outbox and its bookkeeping. */
interface SyncOutboxStore {
    /** Select the persisted enrollment generation before accessing any queue data. */
    fun activateScope(scope: String)
    /** Records not yet accepted, oldest first, bounded by [limit]. */
    fun pending(limit: Int): List<OutboxRecord>
    fun prepareBatch(limit: Int, knownSources: Map<String, SourceSyncRevision>): List<OutboxRecord> = pending(limit)
    /** After a complete pull, recover accepted local records absent from the restored server. */
    fun reconcileAccepted() {}

    /** Adds records, ignoring any whose record id is already pending. */
    fun enqueue(records: List<OutboxRecord>)
    fun contains(recordId: String): Boolean
    fun hasPendingManualCorrection(observationId: String): Boolean

    /**
     * Marks records accepted and remembers the revision each represented, which
     * is what lets a later mapping pass skip unchanged episodes entirely.
     */
    fun markSynced(recordIds: List<String>, at: Instant)

    /** Observation id to latest durably queued or uploaded source revision. */
    fun knownSources(): Map<String, SourceSyncRevision>

    fun pendingCount(): Int

    fun lastSyncedAt(): Instant?

    /**
     * Forgets everything. Used when the user disables sync or re-enrolls
     * against a different server, so records are never pushed to an instance
     * the user did not intend.
     */
    fun clear()
}
