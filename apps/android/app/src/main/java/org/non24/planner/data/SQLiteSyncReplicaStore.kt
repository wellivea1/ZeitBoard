package org.non24.planner.data

import android.content.ContentValues
import android.database.sqlite.SQLiteDatabase
import java.time.Instant
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject

interface SyncReplicaStore {
    fun activateScope(scope: String)
    fun cursor(): Long
    fun apply(page: PullPage, receivedAt: Instant)
    fun cache(projection: JsonObject, receivedAt: Instant)
    fun state(): CompanionState
    fun knownSources(): Map<String, SourceSyncRevision>
    fun cachedReview(observationId: String): SleepReview?
    fun cacheReview(review: JsonObject)
    fun clear()
}

/** Raw downloaded records, erasure metadata, cursor and projection share one SQLite transaction boundary. */
class SQLiteSyncReplicaStore(private val database: () -> SQLiteDatabase) : SyncReplicaStore {
    private var scope: String? = null

    override fun activateScope(scope: String) {
        val db = database()
        if (this.scope != scope) {
            if (meta("scope") != scope) {
                transaction {
                    clearRows(db)
                    putMeta("scope", scope)
                }
            }
            this.scope = scope
        }
        compactIfNeeded()
    }

    override fun cursor(): Long = meta("cursor")?.toLong() ?: 0L

    override fun apply(page: PullPage, receivedAt: Instant) {
        require(page.records.size <= SYNC_PULL_LIMIT)
        val db = database()
        transaction {
            var previous = cursor()
            val ids = HashSet<String>()
            page.records.forEach { record ->
                require(record.seq > previous && ids.add(record.recordId))
                validatePulledPayload(record.recordId, record.kind, record.payload)
                previous = record.seq
            }
            require(previous == page.cursor)
            // Tombstones win within a page, even if an older version precedes them.
            page.records.filter { it.kind == "tombstone" }.forEach(::erase)
            page.records.filter { it.kind != "tombstone" }.forEach { record ->
                val target = when (record.kind) {
                    "correction" -> record.payload.string("target_observation_id")
                    "task" -> record.payload.string("task_id")
                    else -> record.recordId
                }
                val erased = exists("sync_erased_records", "record_id", record.recordId) ||
                    (record.kind == "correction" && exists("sync_erased_records", "record_id", target)) ||
                    (record.kind == "task" && exists("sync_erased_tasks", "task_id", target)) ||
                    exists("erased_health_sources", "observation_id", target)
                if (erased) return@forEach
                val existing = db.rawQuery("SELECT payload FROM sync_replica WHERE record_id = ?", arrayOf(record.recordId)).use {
                    if (it.moveToFirst()) Json.parseToJsonElement(it.getString(0)) else null
                }
                require(existing == null || existing == record.payload) { "A downloaded immutable record changed." }
                if (existing == null) db.insertOrThrow("sync_replica", null, ContentValues().apply {
                    put("record_id", record.recordId); put("kind", record.kind); put("target_id", target)
                    put("observation_kind", if (record.kind == "observation") record.payload.string("kind") else "")
                    put("task_status", if (record.kind == "task") record.payload.string("status") else "")
                    put("revision", if (record.kind == "task") record.payload.integer("revision") else 0L)
                    put("seq", record.seq); put("payload", record.payload.toString())
                })
            }
            if (page.records.isNotEmpty()) db.delete("sync_replica_meta", "key IN ('projection', 'projection_received_at') OR key LIKE 'review:%'", null)
            putMeta("cursor", page.cursor.toString())
            putMeta("downloaded_at", receivedAt.toString())
        }
        compactIfNeeded()
    }

    override fun cache(projection: JsonObject, receivedAt: Instant) {
        val parsed = parseCompanion(projection)
        transaction {
            require(parsed.cursor == cursor()) { "The server projection does not match downloaded records." }
            putMeta("projection", projection.toString())
            putMeta("projection_received_at", receivedAt.toString())
        }
    }

    override fun state(): CompanionState {
        val totalSleepSources = database().rawQuery("SELECT COUNT(*) FROM sync_replica WHERE observation_kind = 'sleep_episode'", null).use { it.moveToFirst(); it.getInt(0) }
        val sources = database().rawQuery("SELECT record_id, payload FROM sync_replica WHERE observation_kind = 'sleep_episode' ORDER BY seq DESC LIMIT 500", null).use { rows ->
            buildList {
                while (rows.moveToNext()) {
                    val payload = Json.parseToJsonElement(rows.getString(1)).jsonObject
                    add(SyncedSleep(rows.getString(0), payload.instant("start_at"), payload.instant("end_at"), payload.string("zone_id"), false))
                }
            }
        }
        val tasks = database().rawQuery(
            """SELECT r.record_id, r.payload FROM sync_replica r WHERE r.kind = 'task'
                AND NOT EXISTS (SELECT 1 FROM sync_replica newer WHERE newer.kind = 'task'
                    AND newer.target_id = r.target_id AND newer.revision > r.revision)
                ORDER BY CASE r.task_status WHEN 'open' THEN 0 ELSE 1 END, r.seq DESC LIMIT 500""", null,
        ).use { cursor -> buildList { while (cursor.moveToNext()) add(parseSyncedTask(cursor.getString(0), Json.parseToJsonElement(cursor.getString(1)).jsonObject)) } }
        return CompanionState(
            projection = meta("projection")?.let { parseCompanion(Json.parseToJsonElement(it).jsonObject) },
            tasks = tasks,
            downloadedAt = meta("downloaded_at")?.let(Instant::parse),
            totalTaskCount = database().rawQuery("SELECT COUNT(DISTINCT target_id) FROM sync_replica WHERE kind = 'task'", null).use { it.moveToFirst(); it.getInt(0) },
            sleepSources = sources, totalSleepSourceCount = totalSleepSources,
        )
    }

    override fun cachedReview(observationId: String): SleepReview? = meta("review:$observationId")?.let { parseSleepReview(Json.parseToJsonElement(it).jsonObject) }

    override fun cacheReview(review: JsonObject) {
        val parsed = parseSleepReview(review)
        transaction {
            require(parsed.cursor == cursor() && exists("sync_replica", "record_id", parsed.observationId))
            putMeta("review:${parsed.observationId}", review.toString())
        }
    }

    override fun knownSources(): Map<String, SourceSyncRevision> = buildMap {
        database().rawQuery("SELECT record_id, kind, payload FROM sync_replica WHERE kind IN ('observation', 'correction') ORDER BY seq", null).use { cursor ->
            while (cursor.moveToNext()) {
                val id = cursor.getString(0); val kind = cursor.getString(1)
                val payload = Json.parseToJsonElement(cursor.getString(2)).jsonObject
                val target = outboxObservationId(kind, cursor.getString(2))
                if (!Regex("^hc-[a-f0-9]{24}$").matches(target)) continue
                if (kind == "correction" && payload["acquisition_method"]?.toString() != "\"health_connect\"") continue
                if (kind == "correction" && payload.string("reason") != "source_conflict") continue
                val candidate = SourceSyncRevision(sourceRevisionFromPayload(kind, cursor.getString(2)), if (kind == "correction") id else null)
                if (get(target)?.revision?.isAfter(candidate.revision) != true) put(target, candidate)
            }
        }
    }

    override fun clear() {
        if (meta("scope") != null) transaction { clearRows(database()) }
        scope = null
        compactIfNeeded()
    }

    private fun clearRows(db: SQLiteDatabase) {
        db.delete("sync_replica", null, null)
        db.delete("sync_erased_records", null, null)
        db.delete("sync_erased_tasks", null, null)
        db.delete("sync_replica_meta", null, null)
        // Health Connect suppression survives disconnect so deleted evidence is not re-imported.
        putMeta("compact_pending", "1")
    }

    private fun erase(record: PulledRecord) {
        val db = database()
        val id = record.recordId
        val erasedKind = if (record.payload.containsKey("record_kind")) record.payload.string("record_kind") else
            db.rawQuery("SELECT kind FROM sync_replica WHERE record_id = ?", arrayOf(id)).use {
                if (it.moveToFirst()) it.getString(0) else null
            }
        db.insertWithOnConflict("sync_erased_records", null, ContentValues().apply { put("record_id", id) }, SQLiteDatabase.CONFLICT_IGNORE)
        val taskId = if (erasedKind == "task") Regex("^(.+)_r[1-9][0-9]*$").matchEntire(id)?.groupValues?.get(1) else null
        if (taskId != null) {
            db.insertWithOnConflict("sync_erased_tasks", null, ContentValues().apply { put("task_id", taskId) }, SQLiteDatabase.CONFLICT_IGNORE)
            db.delete("sync_replica", "kind = 'task' AND target_id = ?", arrayOf(taskId))
        }
        db.delete("sync_replica", "record_id = ? OR (kind = 'correction' AND target_id = ?)", arrayOf(id, id))
        db.delete("sync_outbox", "record_id = ? OR observation_id = ?", arrayOf(id, id))
        if ((erasedKind == null || erasedKind == "observation") && Regex("^hc-[a-f0-9]{24}$").matches(id)) {
            db.insertWithOnConflict("erased_health_sources", null, ContentValues().apply { put("observation_id", id) }, SQLiteDatabase.CONFLICT_IGNORE)
            db.delete("sleep_corrections", "target_logical_source_id IN (SELECT logical_source_id FROM health_sleep_episodes WHERE sync_observation_id = ?)", arrayOf(id))
            db.delete("health_sleep_episodes", "sync_observation_id = ?", arrayOf(id))
        }
        putMeta("compact_pending", "1")
    }

    private fun compactIfNeeded() {
        if (meta("compact_pending") != "1") return
        val db = database()
        db.rawQuery("PRAGMA wal_checkpoint(TRUNCATE)", null).use { if (it.moveToFirst()) check(it.getInt(0) == 0) }
        db.execSQL("VACUUM")
        db.rawQuery("PRAGMA wal_checkpoint(TRUNCATE)", null).use { if (it.moveToFirst()) check(it.getInt(0) == 0) }
        db.delete("sync_replica_meta", "key = 'compact_pending'", null)
    }

    private fun meta(key: String): String? = database().rawQuery("SELECT value FROM sync_replica_meta WHERE key = ?", arrayOf(key)).use {
        if (it.moveToFirst()) it.getString(0) else null
    }
    private fun putMeta(key: String, value: String) {
        database().insertWithOnConflict("sync_replica_meta", null, ContentValues().apply { put("key", key); put("value", value) }, SQLiteDatabase.CONFLICT_REPLACE)
    }
    private fun exists(table: String, column: String, value: String): Boolean =
        database().rawQuery("SELECT 1 FROM $table WHERE $column = ? LIMIT 1", arrayOf(value)).use { it.moveToFirst() }
    private fun transaction(action: () -> Unit) {
        val db = database(); db.beginTransaction()
        try { action(); db.setTransactionSuccessful() } finally { db.endTransaction() }
    }

    companion object {
        internal fun createSchema(db: SQLiteDatabase) {
            db.execSQL("CREATE TABLE sync_replica(record_id TEXT PRIMARY KEY NOT NULL, kind TEXT NOT NULL, observation_kind TEXT NOT NULL, target_id TEXT NOT NULL, task_status TEXT NOT NULL, revision INTEGER NOT NULL, seq INTEGER NOT NULL, payload TEXT NOT NULL)")
            db.execSQL("CREATE INDEX sync_replica_target_revision ON sync_replica(kind, target_id, revision)")
            db.execSQL("CREATE INDEX sync_replica_sleep ON sync_replica(observation_kind, seq)")
            db.execSQL("CREATE TABLE sync_replica_meta(key TEXT PRIMARY KEY NOT NULL, value TEXT NOT NULL)")
            db.execSQL("CREATE TABLE sync_erased_records(record_id TEXT PRIMARY KEY NOT NULL)")
            db.execSQL("CREATE TABLE sync_erased_tasks(task_id TEXT PRIMARY KEY NOT NULL)")
            db.execSQL("CREATE TABLE erased_health_sources(observation_id TEXT PRIMARY KEY NOT NULL)")
        }
    }
}
