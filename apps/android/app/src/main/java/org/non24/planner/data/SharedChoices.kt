package org.non24.planner.data

import android.database.sqlite.SQLiteDatabase
import java.time.Instant
import java.time.format.DateTimeFormatter
import java.time.temporal.ChronoUnit
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

/**
 * The owner's choices that travel between their devices through their own
 * server (ADR-0074): whether a status may show on share links by default, and
 * the clock. The computer also has a third, whether its sleep buttons report a
 * status; the phone has no sleep buttons, so it keeps that one without showing
 * it.
 *
 * Each change is one record naming the choice, its value and when it was made.
 * For each choice the record made latest stands on every device. The phone
 * keeps its own copy of each choice with the time it was made, takes a newer
 * one when it downloads it, and sends its own when it is the newer.
 */
object SharedChoices {
    const val KIND = "setting"

    const val SHARE_STATUS = "share_status_by_default"
    const val FOLLOW_SLEEP_TAPS = "follow_sleep_taps"
    const val CLOCK = "clock"

    /** The clock: the device's own format, or 12- or 24-hour everywhere. */
    const val CLOCK_SYSTEM = "system"
    const val CLOCK_12 = "12"
    const val CLOCK_24 = "24"
    val CLOCKS = listOf(CLOCK_SYSTEM, CLOCK_12, CLOCK_24)

    /** One choice made on this phone, as a record for the owner's server. */
    fun record(name: String, value: JsonPrimitive, changedAt: Instant): OutboxRecord {
        val at = changedAt.truncatedTo(ChronoUnit.MILLIS)
        val said = DateTimeFormatter.ISO_INSTANT.format(at)
        val id = "setting-" + SyncContract.digest("$name|$value|$said")
        val payload = buildJsonObject {
            put("setting_id", JsonPrimitive(id))
            put("name", JsonPrimitive(name))
            put("value", value)
            put("changed_at", JsonPrimitive(said))
        }
        return OutboxRecord(recordId = id, kind = KIND, createdAt = at, sourceRevision = at, payload = payload.toString())
    }

    /** When a choice made now is dated: now, or just after the one standing when this phone's clock is behind it. */
    fun nextChangeAt(now: Instant, standing: SharedChoice?): Instant {
        val at = now.truncatedTo(ChronoUnit.MILLIS)
        if (standing == null || at.isAfter(standing.changedAt)) return at
        return standing.changedAt.truncatedTo(ChronoUnit.MILLIS).plusMillis(1)
    }
}

/** One choice the owner made, on this phone or another device. */
data class SharedChoice(val settingId: String, val name: String, val value: JsonPrimitive, val changedAt: Instant)

/** A choice that travels, as this phone holds it: its value, and when it was made, if ever. */
data class HeldChoice(val name: String, val value: JsonPrimitive, val changedAt: Instant?)

/** Where this phone keeps the choices that travel between the owner's devices. */
interface ChoiceKeeper {
    fun held(): List<HeldChoice>

    /** Takes a choice made later on another device. */
    suspend fun adopt(choice: SharedChoice)
}

/** The phone's settings, as the keeper of its shared choices. */
class SettingsChoiceKeeper(private val settings: SettingsRepository) : ChoiceKeeper {
    override fun held(): List<HeldChoice> = settings.settings.value.let { current ->
        listOf(
            HeldChoice(SharedChoices.SHARE_STATUS, JsonPrimitive(current.shareReportedStatus), current.choiceTimes[SharedChoices.SHARE_STATUS]),
            HeldChoice(SharedChoices.CLOCK, JsonPrimitive(current.clock), current.choiceTimes[SharedChoices.CLOCK]),
        )
    }

    override suspend fun adopt(choice: SharedChoice) = settings.update { current ->
        val times = current.choiceTimes + (choice.name to choice.changedAt)
        when (choice.name) {
            SharedChoices.SHARE_STATUS -> current.copy(shareReportedStatus = choice.value.booleanOrNull ?: current.shareReportedStatus, choiceTimes = times)
            SharedChoices.CLOCK -> current.copy(clock = choice.value.content, choiceTimes = times)
            else -> current
        }
    }
}

/** A downloaded choice, checked as the contract states it. */
internal fun parseSharedChoice(id: String, payload: JsonObject): SharedChoice {
    payload.exactKeys(setOf("setting_id", "name", "value", "changed_at"))
    require(validSyncId(id) && payload.string("setting_id") == id)
    val name = payload.string("name")
    val value = payload.getValue("value").jsonPrimitive
    when (name) {
        SharedChoices.SHARE_STATUS, SharedChoices.FOLLOW_SLEEP_TAPS -> require(!value.isString && value.booleanOrNull != null)
        SharedChoices.CLOCK -> require(value.isString && value.content in SharedChoices.CLOCKS)
        else -> error("Unsupported setting.")
    }
    return SharedChoice(id, name, value, payload.instant("changed_at"))
}

/** Whether [a] stands over [b], two records of one choice: made later, or of two made in one instant the greater id. */
internal fun stands(a: SharedChoice, b: SharedChoice): Boolean =
    if (a.changedAt != b.changedAt) a.changedAt.isAfter(b.changedAt) else a.settingId > b.settingId

/** The record that stands for each choice, among those downloaded and this phone's own. */
internal fun standingChoices(db: SQLiteDatabase): Map<String, SharedChoice> {
    val standing = HashMap<String, SharedChoice>()
    fun read(query: String) = db.rawQuery(query, null).use { cursor ->
        while (cursor.moveToNext()) {
            val choice = parseSharedChoice(cursor.getString(0), Json.parseToJsonElement(cursor.getString(1)).jsonObject)
            val known = standing[choice.name]
            if (known == null || stands(choice, known)) standing[choice.name] = choice
        }
    }
    read("SELECT record_id, payload FROM sync_replica WHERE kind = '${SharedChoices.KIND}'")
    read("SELECT record_id, payload FROM sync_outbox WHERE kind = '${SharedChoices.KIND}'")
    return standing
}
