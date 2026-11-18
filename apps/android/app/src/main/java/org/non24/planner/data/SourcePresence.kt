package org.non24.planner.data

import android.database.sqlite.SQLiteDatabase
import java.time.Duration
import java.time.Instant
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

/**
 * A night that Health Connect no longer has (ADR-0072).
 *
 * A watch app, or the owner, can delete a sleep session after the phone has
 * synced it. The phone then says so with a provider correction,
 * `source_deleted`, and says `source_restored` if the session comes back.
 * Neither changes the record's times: the record is kept, and left out of
 * estimates unless the owner's own correction stands on it.
 *
 * A session counts as gone only when a complete read of this phone's own
 * Health Connect no longer returns it. A night another phone read is not this
 * phone's to judge, so only sources this phone has itself read are candidates,
 * and only those that began a day or more inside the window read: near the
 * window's start, a session can fall out of a read without being deleted.
 */
object SourcePresence {
    const val DELETED = "source_deleted"
    const val RESTORED = "source_restored"

    /** How far inside a read's window a session must begin before its absence means anything. */
    val MARGIN: Duration = Duration.ofDays(1)

    /** Whether a correction payload is a provider saying its record is gone or back. */
    fun isWord(payload: JsonObject): Boolean =
        payload["acquisition_method"]?.jsonPrimitive?.content == "health_connect" &&
            payload["reason"]?.jsonPrimitive?.content in setOf(DELETED, RESTORED)
}

/**
 * One read of Health Connect that completed: the window it covered in full,
 * and the sync ids of every session it returned.
 */
data class CompleteRead(val window: ReadWindow, val observationIds: Set<String>)

/** No read has completed: what a repository without Health Connect reports. */
internal val noCompleteRead: StateFlow<CompleteRead?> = MutableStateFlow(null)

/** A source this phone read from its own Health Connect and synced, and whether the provider's last word was that it is gone. */
data class OwnSource(val start: Instant, val removed: Boolean)

/**
 * The provider's last word on each source record, downloaded or waiting to
 * upload: true when it is gone. Records it never spoke of are absent.
 */
internal fun providerWords(db: SQLiteDatabase): Map<String, Boolean> {
    val latest = HashMap<String, Pair<Instant, Boolean>>()
    fun read(query: String) = db.rawQuery(query, null).use { cursor ->
        while (cursor.moveToNext()) {
            val payload = Json.parseToJsonElement(cursor.getString(0)).jsonObject
            if (!SourcePresence.isWord(payload)) continue
            val target = payload.string("target_observation_id")
            val at = payload.instant("created_at")
            val removed = payload.string("reason") == SourcePresence.DELETED
            val known = latest[target]
            if (known == null || at.isAfter(known.first)) latest[target] = at to removed
        }
    }
    read("SELECT payload FROM sync_replica WHERE kind = 'correction'")
    read("SELECT payload FROM sync_outbox WHERE kind = 'correction'")
    return latest.mapValues { it.value.second }
}
