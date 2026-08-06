package org.non24.planner.data

import android.content.ContentValues
import android.database.sqlite.SQLiteDatabase
import java.time.Instant

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
            "$COLUMN_CREATED_AT ASC, $COLUMN_RECORD_ID ASC",
            limit.toString(),
        ).use { cursor ->
            while (cursor.moveToNext()) {
                records += OutboxRecord(
                    recordId = cursor.getString(0),
                    kind = cursor.getString(1),
                    createdAt = Instant.ofEpochMilli(cursor.getLong(2)),
                    sourceRevision = Instant.ofEpochMilli(cursor.getLong(3)),
                    payload = cursor.getString(4),
                )
            }
        }
        return records
    }

    override fun enqueue(records: List<OutboxRecord>) {
        if (records.isEmpty()) return
        val database = writable()
        database.beginTransaction()
        try {
            for (record in records) {
                val values = ContentValues().apply {
                    put(COLUMN_RECORD_ID, record.recordId)
                    put(COLUMN_KIND, record.kind)
                    put(COLUMN_CREATED_AT, record.createdAt.toEpochMilli())
                    put(COLUMN_REVISION, record.sourceRevision.toEpochMilli())
                    put(COLUMN_PAYLOAD, record.payload)
                }
                // IGNORE rather than REPLACE: a record already queued under the
                // same id is the same record, and replacing it would reset a
                // row that a push may be reading right now.
                database.insertWithOnConflict(
                    OUTBOX_TABLE,
                    null,
                    values,
                    SQLiteDatabase.CONFLICT_IGNORE,
                )
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

    override fun syncedRevisions(): Map<String, Instant> {
        val revisions = LinkedHashMap<String, Instant>()
        readable().query(
            OUTBOX_TABLE,
            arrayOf(COLUMN_RECORD_ID, COLUMN_REVISION),
            "$COLUMN_SYNCED_AT IS NOT NULL",
            null,
            null,
            null,
            null,
        ).use { cursor ->
            while (cursor.moveToNext()) {
                val recordId = cursor.getString(0)
                val revision = Instant.ofEpochMilli(cursor.getLong(1))
                val existing = revisions[recordId]
                // A correction row carries a later revision than the
                // observation it supersedes, so the newest wins.
                if (existing == null || revision.isAfter(existing)) {
                    revisions[recordId] = revision
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
