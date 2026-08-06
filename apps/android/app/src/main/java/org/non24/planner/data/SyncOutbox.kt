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

/**
 * What the user is told about sync. These are deliberately distinguishable:
 * "queued" and "error" mean different things to someone deciding whether to
 * trust what the desktop is showing, and collapsing them into a spinner would
 * hide a backend that has been unreachable for a week.
 */
enum class SyncState {
    /** Sync is not configured. Local-only use is a supported mode, not a fault. */
    OFF,

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
) {
    /**
     * True when the server's copy is known to be complete as of the last
     * successful push. The desktop uses this to decide whether its own
     * freshness claim can lean on Android at all.
     */
    val isCurrent: Boolean get() = state == SyncState.SYNCED && queuedCount == 0
}

/** Durable storage for the outbox and its bookkeeping. */
interface SyncOutboxStore {
    /** Records not yet accepted, oldest first, bounded by [limit]. */
    fun pending(limit: Int): List<OutboxRecord>

    /** Adds records, ignoring any whose record id is already pending. */
    fun enqueue(records: List<OutboxRecord>)

    /**
     * Marks records accepted and remembers the revision each represented, which
     * is what lets a later mapping pass skip unchanged episodes entirely.
     */
    fun markSynced(recordIds: List<String>, at: Instant)

    /** Record id to last accepted source revision. */
    fun syncedRevisions(): Map<String, Instant>

    fun pendingCount(): Int

    fun lastSyncedAt(): Instant?

    /**
     * Forgets everything. Used when the user disables sync or re-enrolls
     * against a different server, so records are never pushed to an instance
     * the user did not intend.
     */
    fun clear()
}
