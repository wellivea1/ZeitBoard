package org.non24.planner.data

import android.content.ContentValues
import android.content.Context
import android.database.Cursor
import android.database.sqlite.SQLiteDatabase
import android.database.sqlite.SQLiteOpenHelper
import java.time.Instant
import java.time.ZoneOffset
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import org.non24.planner.domain.AcquisitionMethod
import org.non24.planner.domain.EvidenceStatus
import org.non24.planner.domain.MedicationEvent
import org.non24.planner.domain.Provenance
import org.non24.planner.domain.SleepCorrection
import org.non24.planner.domain.SleepEpisode

internal class SQLiteLocalUserDataStore(
    context: Context,
    databaseName: String = DATABASE_NAME,
) : SQLiteOpenHelper(context.applicationContext, databaseName, null, DATABASE_VERSION), LocalUserDataStore {
    override fun onConfigure(db: SQLiteDatabase) {
        super.onConfigure(db)
        db.setForeignKeyConstraintsEnabled(true)
    }

    override fun onCreate(db: SQLiteDatabase) {
        db.execSQL(HEALTH_EPISODES_SCHEMA)
        db.execSQL(HEALTH_SNAPSHOT_SCHEMA)
        db.execSQL(CORRECTIONS_SCHEMA)
        db.execSQL(MEDICATION_EVENTS_SCHEMA)
        db.execSQL(HEALTH_LOGICAL_SOURCE_INDEX)
        db.execSQL(CORRECTIONS_TARGET_INDEX)
        db.execSQL(CORRECTIONS_LOGICAL_SOURCE_INDEX)
        db.execSQL(MEDICATION_OCCURRED_INDEX)
        db.execSQL(SQLiteSyncOutboxStore.CREATE_OUTBOX_TABLE)
        db.execSQL(SQLiteSyncOutboxStore.CREATE_OUTBOX_PENDING_INDEX)
    }

    override fun onUpgrade(db: SQLiteDatabase, oldVersion: Int, newVersion: Int) {
        var version = oldVersion
        if (version == 1 && newVersion >= 2) {
            db.execSQL(
                "ALTER TABLE $HEALTH_EPISODES_TABLE ADD COLUMN " +
                    "logical_source_id TEXT NOT NULL DEFAULT ''",
            )
            db.execSQL(BACKFILL_HEALTH_LOGICAL_SOURCE)
            db.execSQL(
                "ALTER TABLE $CORRECTIONS_TABLE ADD COLUMN " +
                    "target_logical_source_id TEXT NOT NULL DEFAULT ''",
            )
            db.execSQL(BACKFILL_CORRECTION_LOGICAL_SOURCE)
            db.execSQL(HEALTH_LOGICAL_SOURCE_INDEX)
            db.execSQL("DROP INDEX IF EXISTS idx_sleep_corrections_target_created")
            db.execSQL(CORRECTIONS_TARGET_INDEX)
            db.execSQL(CORRECTIONS_LOGICAL_SOURCE_INDEX)
            version = 2
        }
        if (version == 2 && newVersion >= 3) {
            // The sync outbox arrives empty. Nothing is backfilled: a record
            // that was never queued was never owed to the server, and
            // inventing queue entries from existing episodes would push a
            // year of history the moment the user enrols.
            db.execSQL(SQLiteSyncOutboxStore.CREATE_OUTBOX_TABLE)
            db.execSQL(SQLiteSyncOutboxStore.CREATE_OUTBOX_PENDING_INDEX)
            version = 3
        }
        check(version == newVersion) {
            "No ZeitBoard Android database migration exists from $oldVersion to $newVersion."
        }
    }

    override fun onDowngrade(db: SQLiteDatabase, oldVersion: Int, newVersion: Int) {
        error("Downgrading the ZeitBoard Android database is unsupported ($oldVersion to $newVersion).")
    }

    override suspend fun loadHealthConnectSleepSnapshot(limit: Int): List<SleepEpisode> =
        withContext(Dispatchers.IO) {
            require(limit > 0) { "Sleep snapshot limit must be positive." }
            readableDatabase.rawQuery(
                """
                SELECT e.*
                FROM health_sleep_snapshot AS s
                JOIN health_sleep_episodes AS e ON e.id = s.episode_id
                ORDER BY e.start_epoch_second DESC, e.start_nano DESC, e.id DESC
                LIMIT ?
                """.trimIndent(),
                arrayOf(limit.toString()),
            ).use { cursor -> cursor.readAll(::readSleepEpisode) }
        }

    override suspend fun replaceHealthConnectSleepSnapshot(episodes: List<SleepEpisode>) {
        withContext(Dispatchers.IO) {
            val db = writableDatabase
            db.beginTransaction()
            try {
                episodes.forEach { episode ->
                    val inserted = db.insertWithOnConflict(
                        HEALTH_EPISODES_TABLE,
                        null,
                        episode.toContentValues(),
                        SQLiteDatabase.CONFLICT_IGNORE,
                    )
                    if (inserted == -1L) {
                        require(db.sleepEpisodeById(episode.id) == episode) {
                            "Health Connect observation ${episode.id} changed without a source revision."
                        }
                    }
                }

                db.delete(HEALTH_SNAPSHOT_TABLE, null, null)
                episodes.forEach { episode ->
                    val values = ContentValues().apply {
                        put("episode_id", episode.id)
                    }
                    check(db.insertOrThrow(HEALTH_SNAPSHOT_TABLE, null, values) != -1L)
                }
                db.setTransactionSuccessful()
            } finally {
                db.endTransaction()
            }
        }
    }

    override suspend fun loadSleepCorrections(limit: Int): List<SleepCorrection> =
        withContext(Dispatchers.IO) {
            require(limit > 0) { "Correction limit must be positive." }
            readableDatabase.query(
                CORRECTIONS_TABLE,
                null,
                null,
                null,
                null,
                null,
                "created_epoch_second DESC, created_nano DESC, id DESC",
                limit.toString(),
            ).use { cursor ->
                cursor.readAll(::readSleepCorrection)
                    .sortedWith(compareBy<SleepCorrection> { it.createdAt }.thenBy { it.id })
            }
        }

    override suspend fun loadLatestSleepCorrectionsForTargets(
        targetEpisodeIds: Set<String>,
    ): List<SleepCorrection> = loadLatestCorrections(
        keyColumn = "target_episode_id",
        keys = targetEpisodeIds,
    )

    override suspend fun loadLatestSleepCorrectionsForLogicalSources(
        logicalSourceIds: Set<String>,
    ): List<SleepCorrection> = loadLatestCorrections(
        keyColumn = "target_logical_source_id",
        keys = logicalSourceIds,
    )

    private suspend fun loadLatestCorrections(
        keyColumn: String,
        keys: Set<String>,
    ): List<SleepCorrection> = withContext(Dispatchers.IO) {
        require(keyColumn == "target_episode_id" || keyColumn == "target_logical_source_id")
        if (keys.isEmpty()) return@withContext emptyList()

        keys.sorted().chunked(SQL_KEY_CHUNK_SIZE).flatMap { chunk ->
            val placeholders = List(chunk.size) { "?" }.joinToString(",")
            readableDatabase.rawQuery(
                """
                SELECT c.*
                FROM $CORRECTIONS_TABLE AS c
                WHERE c.$keyColumn IN ($placeholders)
                  AND NOT EXISTS (
                    SELECT 1
                    FROM $CORRECTIONS_TABLE AS newer
                    WHERE newer.$keyColumn = c.$keyColumn
                      AND (
                        newer.created_epoch_second > c.created_epoch_second OR
                        (newer.created_epoch_second = c.created_epoch_second AND
                         newer.created_nano > c.created_nano) OR
                        (newer.created_epoch_second = c.created_epoch_second AND
                         newer.created_nano = c.created_nano AND newer.id > c.id)
                      )
                  )
                ORDER BY c.created_epoch_second, c.created_nano, c.id
                """.trimIndent(),
                chunk.toTypedArray(),
            ).use { cursor -> cursor.readAll(::readSleepCorrection) }
        }
    }

    override suspend fun appendSleepCorrection(correction: SleepCorrection) {
        withContext(Dispatchers.IO) {
            val db = writableDatabase
            val inserted = db.insertWithOnConflict(
                CORRECTIONS_TABLE,
                null,
                correction.toContentValues(),
                SQLiteDatabase.CONFLICT_IGNORE,
            )
            if (inserted == -1L) {
                require(db.sleepCorrectionById(correction.id) == correction) {
                    "Sleep correction ${correction.id} already exists with different content."
                }
            }
        }
    }

    override suspend fun loadMedicationEvents(limit: Int): List<MedicationEvent> =
        withContext(Dispatchers.IO) {
            require(limit > 0) { "Medication event limit must be positive." }
            readableDatabase.query(
                MEDICATION_EVENTS_TABLE,
                null,
                null,
                null,
                null,
                null,
                "occurred_epoch_second DESC, occurred_nano DESC, id DESC",
                limit.toString(),
            ).use { cursor -> cursor.readAll(::readMedicationEvent) }
        }

    override suspend fun appendMedicationEvent(event: MedicationEvent) {
        withContext(Dispatchers.IO) {
            val db = writableDatabase
            val inserted = db.insertWithOnConflict(
                MEDICATION_EVENTS_TABLE,
                null,
                event.toContentValues(),
                SQLiteDatabase.CONFLICT_IGNORE,
            )
            if (inserted == -1L) {
                require(db.medicationEventById(event.id) == event) {
                    "Medication event ${event.id} already exists with different content."
                }
            }
        }
    }

    private fun SQLiteDatabase.sleepEpisodeById(id: String): SleepEpisode? =
        query(HEALTH_EPISODES_TABLE, null, "id = ?", arrayOf(id), null, null, null, "1")
            .use { cursor -> if (cursor.moveToFirst()) readSleepEpisode(cursor) else null }

    private fun SQLiteDatabase.sleepCorrectionById(id: String): SleepCorrection? =
        query(CORRECTIONS_TABLE, null, "id = ?", arrayOf(id), null, null, null, "1")
            .use { cursor -> if (cursor.moveToFirst()) readSleepCorrection(cursor) else null }

    private fun SQLiteDatabase.medicationEventById(id: String): MedicationEvent? =
        query(MEDICATION_EVENTS_TABLE, null, "id = ?", arrayOf(id), null, null, null, "1")
            .use { cursor -> if (cursor.moveToFirst()) readMedicationEvent(cursor) else null }

    private fun SleepEpisode.toContentValues(): ContentValues = ContentValues().apply {
        put("id", id)
        put("logical_source_id", logicalSourceId)
        putInstant("start", start)
        putInstant("end", end)
        putNullableString("iana_zone_id", ianaTimeZoneId)
        putNullableInt("start_offset_seconds", startZoneOffset?.totalSeconds)
        putNullableInt("end_offset_seconds", endZoneOffset?.totalSeconds)
        putProvenance(provenance)
    }

    private fun SleepCorrection.toContentValues(): ContentValues = ContentValues().apply {
        put("id", id)
        put("target_episode_id", targetEpisodeId)
        put("target_logical_source_id", targetLogicalSourceId)
        putInstant("corrected_start", correctedStart)
        putInstant("corrected_end", correctedEnd)
        putNullableString("iana_zone_id", ianaTimeZoneId)
        putNullableInt("start_offset_seconds", startZoneOffset?.totalSeconds)
        putNullableInt("end_offset_seconds", endZoneOffset?.totalSeconds)
        putInstant("created", createdAt)
        putProvenance(provenance)
    }

    private fun MedicationEvent.toContentValues(): ContentValues = ContentValues().apply {
        put("id", id)
        put("display_name", displayName)
        putInstant("occurred", occurredAt)
        put("time_zone_id", timeZoneId)
        putInstant("created", createdAt)
        putProvenance(provenance)
    }

    private fun ContentValues.putProvenance(provenance: Provenance) {
        put("acquisition_method", provenance.acquisitionMethod.name)
        put("evidence_status", provenance.evidenceStatus.name)
        put("source_id", provenance.sourceId)
        putNullableString("source_record_id", provenance.sourceRecordId)
        putNullableInstant("source_updated", provenance.sourceUpdatedAt)
    }

    private fun readSleepEpisode(cursor: Cursor): SleepEpisode {
        val start = cursor.instant("start")
        val end = cursor.instant("end")
        val provenance = cursor.provenance()
        return SleepEpisode(
            id = cursor.requiredString("id"),
            logicalSourceId = cursor.requiredString("logical_source_id"),
            start = start,
            end = end,
            ianaTimeZoneId = cursor.optionalString("iana_zone_id"),
            startZoneOffset = cursor.optionalZoneOffset("start_offset_seconds"),
            endZoneOffset = cursor.optionalZoneOffset("end_offset_seconds"),
            provenance = provenance,
        )
    }

    private fun readSleepCorrection(cursor: Cursor): SleepCorrection = SleepCorrection(
        id = cursor.requiredString("id"),
        targetEpisodeId = cursor.requiredString("target_episode_id"),
        targetLogicalSourceId = cursor.requiredString("target_logical_source_id"),
        correctedStart = cursor.instant("corrected_start"),
        correctedEnd = cursor.instant("corrected_end"),
        ianaTimeZoneId = cursor.optionalString("iana_zone_id"),
        startZoneOffset = cursor.optionalZoneOffset("start_offset_seconds"),
        endZoneOffset = cursor.optionalZoneOffset("end_offset_seconds"),
        createdAt = cursor.instant("created"),
        provenance = cursor.provenance(),
    )

    private fun readMedicationEvent(cursor: Cursor): MedicationEvent = MedicationEvent(
        id = cursor.requiredString("id"),
        displayName = cursor.requiredString("display_name"),
        occurredAt = cursor.instant("occurred"),
        timeZoneId = cursor.requiredString("time_zone_id"),
        createdAt = cursor.instant("created"),
        provenance = cursor.provenance(),
    )

    private fun Cursor.provenance(): Provenance = Provenance(
        acquisitionMethod = AcquisitionMethod.valueOf(requiredString("acquisition_method")),
        evidenceStatus = EvidenceStatus.valueOf(requiredString("evidence_status")),
        sourceId = requiredString("source_id"),
        sourceRecordId = optionalString("source_record_id"),
        sourceUpdatedAt = optionalInstant("source_updated"),
    )

    private companion object {
        const val DATABASE_NAME = "zeitboard_local.db"
        const val DATABASE_VERSION = 3
        const val SQL_KEY_CHUNK_SIZE = 800
        const val HEALTH_EPISODES_TABLE = "health_sleep_episodes"
        const val HEALTH_SNAPSHOT_TABLE = "health_sleep_snapshot"
        const val CORRECTIONS_TABLE = "sleep_corrections"
        const val MEDICATION_EVENTS_TABLE = "medication_events"

        val HEALTH_EPISODES_SCHEMA = """
            CREATE TABLE $HEALTH_EPISODES_TABLE (
                id TEXT PRIMARY KEY NOT NULL CHECK(length(id) > 0),
                logical_source_id TEXT NOT NULL CHECK(length(logical_source_id) > 0),
                start_epoch_second INTEGER NOT NULL,
                start_nano INTEGER NOT NULL CHECK(start_nano BETWEEN 0 AND 999999999),
                end_epoch_second INTEGER NOT NULL,
                end_nano INTEGER NOT NULL CHECK(end_nano BETWEEN 0 AND 999999999),
                iana_zone_id TEXT,
                start_offset_seconds INTEGER CHECK(start_offset_seconds BETWEEN -64800 AND 64800),
                end_offset_seconds INTEGER CHECK(end_offset_seconds BETWEEN -64800 AND 64800),
                acquisition_method TEXT NOT NULL,
                evidence_status TEXT NOT NULL,
                source_id TEXT NOT NULL,
                source_record_id TEXT,
                source_updated_epoch_second INTEGER,
                source_updated_nano INTEGER,
                CHECK(
                    end_epoch_second > start_epoch_second OR
                    (end_epoch_second = start_epoch_second AND end_nano > start_nano)
                ),
                CHECK(
                    (source_updated_epoch_second IS NULL AND source_updated_nano IS NULL) OR
                    (source_updated_epoch_second IS NOT NULL AND source_updated_nano IS NOT NULL)
                )
            )
        """.trimIndent()

        val HEALTH_SNAPSHOT_SCHEMA = """
            CREATE TABLE $HEALTH_SNAPSHOT_TABLE (
                episode_id TEXT PRIMARY KEY NOT NULL,
                FOREIGN KEY(episode_id) REFERENCES $HEALTH_EPISODES_TABLE(id) ON DELETE CASCADE
            )
        """.trimIndent()

        val CORRECTIONS_SCHEMA = """
            CREATE TABLE $CORRECTIONS_TABLE (
                id TEXT PRIMARY KEY NOT NULL CHECK(length(id) > 0),
                target_episode_id TEXT NOT NULL CHECK(length(target_episode_id) > 0),
                target_logical_source_id TEXT NOT NULL CHECK(length(target_logical_source_id) > 0),
                corrected_start_epoch_second INTEGER NOT NULL,
                corrected_start_nano INTEGER NOT NULL CHECK(corrected_start_nano BETWEEN 0 AND 999999999),
                corrected_end_epoch_second INTEGER NOT NULL,
                corrected_end_nano INTEGER NOT NULL CHECK(corrected_end_nano BETWEEN 0 AND 999999999),
                iana_zone_id TEXT,
                start_offset_seconds INTEGER CHECK(start_offset_seconds BETWEEN -64800 AND 64800),
                end_offset_seconds INTEGER CHECK(end_offset_seconds BETWEEN -64800 AND 64800),
                created_epoch_second INTEGER NOT NULL,
                created_nano INTEGER NOT NULL CHECK(created_nano BETWEEN 0 AND 999999999),
                acquisition_method TEXT NOT NULL,
                evidence_status TEXT NOT NULL,
                source_id TEXT NOT NULL,
                source_record_id TEXT,
                source_updated_epoch_second INTEGER,
                source_updated_nano INTEGER,
                CHECK(
                    corrected_end_epoch_second > corrected_start_epoch_second OR
                    (corrected_end_epoch_second = corrected_start_epoch_second AND corrected_end_nano > corrected_start_nano)
                ),
                CHECK(
                    (source_updated_epoch_second IS NULL AND source_updated_nano IS NULL) OR
                    (source_updated_epoch_second IS NOT NULL AND source_updated_nano IS NOT NULL)
                )
            )
        """.trimIndent()

        val MEDICATION_EVENTS_SCHEMA = """
            CREATE TABLE $MEDICATION_EVENTS_TABLE (
                id TEXT PRIMARY KEY NOT NULL CHECK(length(id) > 0),
                display_name TEXT NOT NULL,
                occurred_epoch_second INTEGER NOT NULL,
                occurred_nano INTEGER NOT NULL CHECK(occurred_nano BETWEEN 0 AND 999999999),
                time_zone_id TEXT NOT NULL,
                created_epoch_second INTEGER NOT NULL,
                created_nano INTEGER NOT NULL CHECK(created_nano BETWEEN 0 AND 999999999),
                acquisition_method TEXT NOT NULL,
                evidence_status TEXT NOT NULL,
                source_id TEXT NOT NULL,
                source_record_id TEXT,
                source_updated_epoch_second INTEGER,
                source_updated_nano INTEGER,
                CHECK(
                    (source_updated_epoch_second IS NULL AND source_updated_nano IS NULL) OR
                    (source_updated_epoch_second IS NOT NULL AND source_updated_nano IS NOT NULL)
                )
            )
        """.trimIndent()

        val HEALTH_LOGICAL_SOURCE_INDEX =
            "CREATE INDEX idx_health_sleep_logical_source " +
                "ON $HEALTH_EPISODES_TABLE(logical_source_id)"
        val CORRECTIONS_TARGET_INDEX =
            "CREATE INDEX idx_sleep_corrections_target_created " +
                "ON $CORRECTIONS_TABLE(" +
                "target_episode_id, created_epoch_second DESC, created_nano DESC, id DESC)"
        val CORRECTIONS_LOGICAL_SOURCE_INDEX =
            "CREATE INDEX idx_sleep_corrections_logical_created " +
                "ON $CORRECTIONS_TABLE(" +
                "target_logical_source_id, created_epoch_second DESC, created_nano DESC, id DESC)"
        val MEDICATION_OCCURRED_INDEX =
            "CREATE INDEX idx_medication_events_occurred " +
                "ON $MEDICATION_EVENTS_TABLE(occurred_epoch_second DESC, occurred_nano DESC)"
        val BACKFILL_HEALTH_LOGICAL_SOURCE = """
            UPDATE $HEALTH_EPISODES_TABLE
            SET logical_source_id = source_id || char(31) ||
                CASE
                    WHEN source_record_id IS NOT NULL AND length(source_record_id) > 0
                        THEN source_record_id
                    ELSE start_epoch_second || ':' || start_nano || char(31) ||
                        end_epoch_second || ':' || end_nano
                END
            WHERE logical_source_id = ''
        """.trimIndent()
        val BACKFILL_CORRECTION_LOGICAL_SOURCE = """
            UPDATE $CORRECTIONS_TABLE
            SET target_logical_source_id = COALESCE(
                (
                    SELECT e.logical_source_id
                    FROM $HEALTH_EPISODES_TABLE AS e
                    WHERE e.id = $CORRECTIONS_TABLE.target_episode_id
                ),
                target_episode_id
            )
            WHERE target_logical_source_id = ''
        """.trimIndent()
    }
}

private inline fun <T> Cursor.readAll(read: (Cursor) -> T): List<T> = buildList {
    while (moveToNext()) add(read(this@readAll))
}

private fun ContentValues.putInstant(prefix: String, instant: Instant) {
    put("${prefix}_epoch_second", instant.epochSecond)
    put("${prefix}_nano", instant.nano)
}

private fun ContentValues.putNullableInstant(prefix: String, instant: Instant?) {
    if (instant == null) {
        putNull("${prefix}_epoch_second")
        putNull("${prefix}_nano")
    } else {
        putInstant(prefix, instant)
    }
}

private fun ContentValues.putNullableString(column: String, value: String?) {
    if (value == null) putNull(column) else put(column, value)
}

private fun ContentValues.putNullableInt(column: String, value: Int?) {
    if (value == null) putNull(column) else put(column, value)
}

private fun Cursor.requiredString(column: String): String = getString(getColumnIndexOrThrow(column))

private fun Cursor.optionalString(column: String): String? {
    val index = getColumnIndexOrThrow(column)
    return if (isNull(index)) null else getString(index)
}

private fun Cursor.instant(prefix: String): Instant = Instant.ofEpochSecond(
    getLong(getColumnIndexOrThrow("${prefix}_epoch_second")),
    getInt(getColumnIndexOrThrow("${prefix}_nano")).toLong(),
)

private fun Cursor.optionalInstant(prefix: String): Instant? {
    val secondsIndex = getColumnIndexOrThrow("${prefix}_epoch_second")
    val nanosIndex = getColumnIndexOrThrow("${prefix}_nano")
    return if (isNull(secondsIndex) || isNull(nanosIndex)) {
        null
    } else {
        Instant.ofEpochSecond(getLong(secondsIndex), getInt(nanosIndex).toLong())
    }
}

private fun Cursor.optionalZoneOffset(column: String): ZoneOffset? {
    val index = getColumnIndexOrThrow(column)
    return if (isNull(index)) null else ZoneOffset.ofTotalSeconds(getInt(index))
}
