package org.non24.planner.data

import android.content.ContentValues
import android.database.sqlite.SQLiteDatabase
import java.time.Instant
import java.time.ZoneId
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject

/**
 * SQLite-backed outbox.
 *
 * It shares the app database rather than opening its own, so a record and the
 * episode it came from are removed together when the user erases data. A
 * separate database would be one more place health-adjacent bytes could
 * outlive an erasure.
 */
class SQLiteSyncOutboxStore(
    private val readable: () -> SQLiteDatabase,
    private val writable: () -> SQLiteDatabase,
) : SyncOutboxStore {

    private var scope: String? = null

    override fun queueLocalCorrections(homeZone: ZoneId, knownSources: Map<String, SourceSyncRevision>, now: Instant, limit: Int): LocalCorrectionQueueResult {
        require(limit in 1..100)
        val db = writable()
        db.beginTransaction()
        try {
            // Fixture corrections have no Health Connect source row and never enter this page.
            val candidates = """FROM sleep_corrections c
                JOIN health_sleep_episodes e ON e.id = c.target_episode_id
                WHERE e.acquisition_method = 'HEALTH_CONNECT' AND e.evidence_status = 'IMPORTED'
                AND ((NOT EXISTS (SELECT 1 FROM sync_outbox o WHERE o.record_id = c.id)
                    AND NOT EXISTS (SELECT 1 FROM sync_replica r WHERE r.record_id = c.id))
                    OR (NOT EXISTS (SELECT 1 FROM sync_replica r WHERE r.record_id = e.sync_observation_id)
                        AND NOT EXISTS (SELECT 1 FROM sync_outbox o WHERE o.record_id = e.sync_observation_id)))
                AND NOT EXISTS (SELECT 1 FROM sync_erased_records d WHERE d.record_id IN (c.id, e.sync_observation_id))
                AND NOT EXISTS (SELECT 1 FROM erased_health_sources d WHERE d.observation_id = e.sync_observation_id)
                AND (c.sync_hold_zone IS NULL OR c.sync_hold_zone != ?)"""
            val corrections = db.rawQuery("SELECT c.* $candidates ORDER BY c.sequence LIMIT ?", arrayOf(homeZone.id, limit.toString())).use { rows ->
                buildList { while (rows.moveToNext()) add(readSleepCorrection(rows)) }
            }
            val known = knownSources.toMutableMap()
            this.knownSources().forEach { (id, revision) -> if (known[id]?.revision?.isAfter(revision.revision) != true) known[id] = revision }
            corrections.forEach { correction ->
                val source = db.rawQuery("SELECT * FROM health_sleep_episodes WHERE id = ?", arrayOf(correction.targetEpisodeId)).use { check(it.moveToFirst()); readSleepEpisode(it) }
                if (SyncContract.holdReason(source, homeZone) != null) {
                    db.update("sleep_corrections", ContentValues().apply { put("sync_hold_zone", homeZone.id) }, "id = ?", arrayOf(correction.id))
                } else {
                    val sourceId = SyncContract.observationId(source.logicalSourceId)
                    val hasSource = db.rawQuery("SELECT 1 FROM sync_replica WHERE record_id = ? UNION ALL SELECT 1 FROM sync_outbox WHERE record_id = ?", arrayOf(sourceId, sourceId)).use { it.moveToFirst() }
                    val sourceRecords = SyncContract.map(listOf(source), homeZone, if (hasSource) known else known - sourceId, now).records
                    val revision = source.provenance.sourceUpdatedAt ?: source.end
                    val record = manualCorrection(sourceId, revision, correction.supersedesCorrectionIds, correction.id, correction.createdAt,
                        correction.correctedStart, correction.correctedEnd, "principal", false)
                    enqueue(sourceRecords + record)
                    if (known[sourceId]?.revision?.isAfter(revision) != true) known[sourceId] = SourceSyncRevision(revision)
                    db.update("sleep_corrections", ContentValues().apply { putNull("sync_hold_zone") }, "id = ?", arrayOf(correction.id))
                }
            }
            val held = db.rawQuery("SELECT COUNT(*) FROM sleep_corrections WHERE sync_hold_zone IS NOT NULL", null).use { it.moveToFirst(); it.getInt(0) }
            db.setTransactionSuccessful()
            val hasMore = db.rawQuery("SELECT 1 $candidates LIMIT 1", arrayOf(homeZone.id)).use { it.moveToFirst() }
            return LocalCorrectionQueueResult(hasMore, held)
        } finally { db.endTransaction() }
    }

    override fun activateScope(scope: String) {
        if (scope == this.scope) return
        // Config commits first. If the process dies before this cleanup, the
        // next repository initialization performs it before reading the queue.
        writable().delete(OUTBOX_TABLE, "queue_scope != ?", arrayOf(scope))
        this.scope = scope
    }

    override fun pending(limit: Int): List<OutboxRecord> {
        require(limit > 0) { "Outbox page limit must be positive." }
        val records = mutableListOf<OutboxRecord>()
        readable().query(
            OUTBOX_TABLE,
            arrayOf(COLUMN_RECORD_ID, COLUMN_KIND, COLUMN_CREATED_AT, COLUMN_REVISION, COLUMN_PAYLOAD),
            "$COLUMN_SYNCED_AT IS NULL",
            null,
            null,
            null,
            "rowid ASC",
            limit.toString(),
        ).use { cursor ->
            while (cursor.moveToNext()) {
                records += OutboxRecord(
                    recordId = cursor.getString(0),
                    kind = cursor.getString(1),
                    createdAt = Instant.ofEpochMilli(cursor.getLong(2)),
                    sourceRevision = sourceRevisionFromPayload(cursor.getString(1), cursor.getString(4)),
                    payload = cursor.getString(4),
                )
            }
        }
        return records
    }

    override fun prepareBatch(limit: Int, knownSources: Map<String, SourceSyncRevision>): List<OutboxRecord> {
        val batch = pending(limit)
        val db = writable()
        db.beginTransaction()
        try {
            val prepared = batch.mapNotNull { record ->
                val remote = db.rawQuery("SELECT payload FROM sync_replica WHERE record_id = ?", arrayOf(record.recordId)).use {
                    if (it.moveToFirst()) Json.parseToJsonElement(it.getString(0)).jsonObject else null
                } ?: return@mapNotNull record
                val local = Json.parseToJsonElement(record.payload).jsonObject
                if (local == remote) return@mapNotNull record
                val replacement = if (record.kind == "observation") {
                    val latest = knownSources[record.recordId] ?: error("Source revision metadata is unavailable.")
                    if (record.sourceRevision == remote.getValue("provenance").jsonObject.instant("recorded_at")) {
                        require(local.instant("start_at") == remote.instant("start_at") && local.instant("end_at") == remote.instant("end_at")) {
                            "The same source revision contains conflicting timestamps."
                        }
                    }
                    if (record.sourceRevision == latest.revision && latest.correctionId != null) {
                        val latestPayload = db.rawQuery("SELECT payload FROM sync_replica WHERE record_id = ?", arrayOf(latest.correctionId)).use {
                            check(it.moveToFirst()) { "Source revision metadata is unavailable." }
                            Json.parseToJsonElement(it.getString(0)).jsonObject
                        }.getValue("changes").jsonObject
                        require(local.instant("start_at") == latestPayload.instant("start_at") && local.instant("end_at") == latestPayload.instant("end_at")) {
                            "The same source revision contains conflicting timestamps."
                        }
                    }
                    if (!record.sourceRevision.isAfter(latest.revision)) null else SyncContract.sourceCorrection(
                        record.recordId, local.instant("start_at"), local.instant("end_at"), record.sourceRevision, record.createdAt,
                    )
                } else {
                    error("A correction changed under the same immutable record ID.")
                }
                db.delete(OUTBOX_TABLE, "$COLUMN_RECORD_ID = ? AND $COLUMN_SYNCED_AT IS NULL", arrayOf(record.recordId))
                if (replacement == null) null else {
                    enqueue(listOf(replacement))
                    // Push the durable row, including when already accepted;
                    // never acknowledge a different payload under the same ID.
                    db.rawQuery("SELECT $COLUMN_PAYLOAD FROM $OUTBOX_TABLE WHERE $COLUMN_RECORD_ID = ? AND $COLUMN_SYNCED_AT IS NULL", arrayOf(replacement.recordId)).use {
                        if (it.moveToFirst()) replacement.copy(payload = it.getString(0)) else null
                    }
                }
            }
            db.setTransactionSuccessful()
            return prepared.distinctBy { it.recordId }
        } finally { db.endTransaction() }
    }

    override fun reconcileAccepted() {
        writable().execSQL("""UPDATE sync_outbox SET synced_at = NULL WHERE synced_at IS NOT NULL
            AND NOT EXISTS (SELECT 1 FROM sync_replica r WHERE r.record_id = sync_outbox.record_id)
            AND NOT EXISTS (SELECT 1 FROM sync_erased_records e WHERE e.record_id IN (sync_outbox.record_id, sync_outbox.observation_id))
            AND NOT EXISTS (SELECT 1 FROM erased_health_sources e WHERE e.observation_id = sync_outbox.observation_id)""")
    }

    override fun enqueue(records: List<OutboxRecord>) {
        if (records.isEmpty()) return
        val database = writable()
        database.beginTransaction()
        try {
            for (record in records) {
                val observationId = outboxObservationId(record.kind, record.payload)
                val erased = database.rawQuery(
                    "SELECT 1 FROM erased_health_sources WHERE observation_id = ? UNION ALL SELECT 1 FROM sync_erased_records WHERE record_id IN (?, ?)",
                    arrayOf(observationId, record.recordId, observationId),
                ).use { it.moveToFirst() }
                if (erased) continue
                val values = ContentValues().apply {
                    put("observation_id", observationId)
                    put("queue_scope", requireNotNull(scope))
                    put(COLUMN_RECORD_ID, record.recordId)
                    put(COLUMN_KIND, record.kind)
                    put(COLUMN_CREATED_AT, record.createdAt.toEpochMilli())
                    put(COLUMN_REVISION, record.sourceRevision.toEpochMilli())
                    put(COLUMN_PAYLOAD, record.payload)
                }
                // IGNORE rather than REPLACE: a record already queued under the
                // same id is the same record, and replacing it would reset a
                // row that a push may be reading right now.
                val inserted = database.insertWithOnConflict(
                    OUTBOX_TABLE,
                    null,
                    values,
                    SQLiteDatabase.CONFLICT_IGNORE,
                )
                if (inserted == -1L) database.rawQuery("SELECT payload FROM $OUTBOX_TABLE WHERE record_id = ?", arrayOf(record.recordId)).use {
                    check(it.moveToFirst() && Json.parseToJsonElement(it.getString(0)) == Json.parseToJsonElement(record.payload)) {
                        "An immutable queued record changed."
                    }
                }
            }
            database.setTransactionSuccessful()
        } finally {
            database.endTransaction()
        }
    }

    override fun markSynced(recordIds: List<String>, at: Instant) {
        if (recordIds.isEmpty()) return
        val database = writable()
        database.beginTransaction()
        try {
            for (recordId in recordIds) {
                val values = ContentValues().apply { put(COLUMN_SYNCED_AT, at.toEpochMilli()) }
                database.update(
                    OUTBOX_TABLE,
                    values,
                    "$COLUMN_RECORD_ID = ?",
                    arrayOf(recordId),
                )
            }
            database.setTransactionSuccessful()
        } finally {
            database.endTransaction()
        }
    }

    override fun knownSources(): Map<String, SourceSyncRevision> {
        val revisions = LinkedHashMap<String, SourceSyncRevision>()
        readable().query(
            OUTBOX_TABLE,
            arrayOf(COLUMN_RECORD_ID, COLUMN_KIND, COLUMN_PAYLOAD),
            null,
            null,
            null,
            null,
            null,
        ).use { cursor ->
            while (cursor.moveToNext()) {
                val kind = cursor.getString(1)
                val payload = Json.parseToJsonElement(cursor.getString(2)).jsonObject
                if (kind == "correction" && payload["acquisition_method"]?.toString() != "\"health_connect\"") continue
                val observationId = payload.string(if (kind == "correction") "target_observation_id" else "observation_id")
                val revision = SourceSyncRevision(
                    sourceRevisionFromPayload(kind, cursor.getString(2)),
                    if (kind == "correction") cursor.getString(0) else null,
                )
                val existing = revisions[observationId]
                if (existing == null || revision.revision.isAfter(existing.revision)) {
                    revisions[observationId] = revision
                }
            }
        }
        return revisions
    }

    override fun pendingCount(): Int {
        readable().rawQuery(
            "SELECT COUNT(*) FROM $OUTBOX_TABLE WHERE $COLUMN_SYNCED_AT IS NULL",
            null,
        ).use { cursor ->
            return if (cursor.moveToFirst()) cursor.getInt(0) else 0
        }
    }

    override fun contains(recordId: String): Boolean = readable().rawQuery(
        "SELECT 1 FROM sync_outbox WHERE record_id = ?", arrayOf(recordId),
    ).use { it.moveToFirst() }

    override fun hasPendingManualCorrection(observationId: String): Boolean = readable().rawQuery(
        "SELECT payload FROM sync_outbox WHERE observation_id = ? AND kind = 'correction' AND synced_at IS NULL", arrayOf(observationId),
    ).use { cursor ->
        var found = false
        while (cursor.moveToNext()) if (Json.parseToJsonElement(cursor.getString(0)).jsonObject["acquisition_method"]?.toString() != "\"health_connect\"") found = true
        found
    }

    override fun lastSyncedAt(): Instant? {
        readable().rawQuery(
            "SELECT MAX($COLUMN_SYNCED_AT) FROM $OUTBOX_TABLE WHERE $COLUMN_SYNCED_AT IS NOT NULL",
            null,
        ).use { cursor ->
            if (!cursor.moveToFirst() || cursor.isNull(0)) return null
            return Instant.ofEpochMilli(cursor.getLong(0))
        }
    }

    override fun clear() {
        writable().delete(OUTBOX_TABLE, null, null)
    }

    companion object {
        const val OUTBOX_TABLE = "sync_outbox"
        const val COLUMN_RECORD_ID = "record_id"
        const val COLUMN_KIND = "kind"
        const val COLUMN_CREATED_AT = "created_at"
        const val COLUMN_REVISION = "source_revision"
        const val COLUMN_PAYLOAD = "payload"
        const val COLUMN_SYNCED_AT = "synced_at"

        val CREATE_OUTBOX_TABLE = """
            CREATE TABLE $OUTBOX_TABLE (
                observation_id TEXT NOT NULL,
                queue_scope TEXT NOT NULL,
                $COLUMN_RECORD_ID TEXT PRIMARY KEY NOT NULL,
                $COLUMN_KIND TEXT NOT NULL,
                $COLUMN_CREATED_AT INTEGER NOT NULL,
                $COLUMN_REVISION INTEGER NOT NULL,
                $COLUMN_PAYLOAD TEXT NOT NULL,
                $COLUMN_SYNCED_AT INTEGER
            )
        """.trimIndent()

        val CREATE_OUTBOX_PENDING_INDEX = """
            CREATE INDEX idx_${OUTBOX_TABLE}_pending
                ON $OUTBOX_TABLE($COLUMN_SYNCED_AT, $COLUMN_CREATED_AT)
        """.trimIndent()
    }
}

/** Source revision identity retains the full precision of the immutable payload. */
internal fun sourceRevisionFromPayload(kind: String, payload: String): Instant {
    val json = Json.parseToJsonElement(payload).jsonObject
    return Instant.parse(
        if (kind == "correction") json.string("created_at")
        else json.getValue("provenance").jsonObject.string("recorded_at"),
    )
}

internal fun outboxObservationId(kind: String, payload: String): String =
    Json.parseToJsonElement(payload).jsonObject.string(if (kind == "correction") "target_observation_id" else "observation_id")
