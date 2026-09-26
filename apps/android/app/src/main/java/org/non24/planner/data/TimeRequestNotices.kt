package org.non24.planner.data

import java.security.MessageDigest
import java.time.Instant
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.longOrNull

/**
 * Notices about time requests (C6).
 *
 * The owner's server keeps a feed of what happened — someone asked for a time,
 * wrote about a request, or a request was answered — and to which request, but
 * never what anyone wrote. This phone reads the feed after its own cursor while
 * notices are turned on, and words each notice itself, so a lock screen can
 * never show private content, even from a compromised server.
 */
enum class NoticeKind { REQUEST, MESSAGE }

data class FeedEvent(
    val eventId: String,
    val kind: String,
    val subject: String,
    val createdAt: Instant,
    val expiresAt: Instant,
)

data class FeedPage(val events: List<FeedEvent>, val cursor: Long, val hasMore: Boolean)

sealed interface NoticeAction {
    data class Show(val subject: String, val kind: NoticeKind) : NoticeAction
    data class Retire(val subject: String) : NoticeAction
}

/** Where a notice is shown. The words are the poster's, never the server's. */
interface NoticePoster {
    /** Makes sure notices can be shown (on Android, their channel exists). */
    fun prepare() {}
    fun show(subject: String, kind: NoticeKind)
    fun retire(subject: String)
    fun retireAll()
}

/** The feed position, kept per enrollment so a new server starts afresh. */
interface NoticeCursorStore {
    fun load(scope: String): Long?
    fun save(scope: String, cursor: Long)
    fun clear()
}

private const val MAX_FEED_BYTES = 64 * 1024
private const val MAX_PAGES_PER_CHECK = 5
private val knownKinds = setOf("visitor_request", "visitor_message", "visitor_decided")

internal fun parseNotificationFeed(json: JsonObject): FeedPage {
    fun invalid(): Nothing = throw BackendSyncException(200, "The server returned an invalid notification feed.")
    if (json.string("schema_version") != SyncContract.SCHEMA_VERSION) invalid()
    val cursor = (json["cursor"] as? JsonPrimitive)?.takeUnless { it.isString }?.longOrNull ?: invalid()
    val hasMore = (json["hasMore"] as? JsonPrimitive)?.booleanOrNull ?: false
    val events = (json["events"] as? JsonArray ?: invalid()).map { element ->
        val event = runCatching { element.jsonObject }.getOrNull() ?: invalid()
        val kind = event.string("kind")
        val subject = event.string("subject")
        if (kind !in knownKinds || subject.isBlank() || subject.length > 256 || subject.any(Char::isISOControl)) invalid()
        FeedEvent(
            eventId = event.string("eventId"),
            kind = kind,
            subject = subject,
            createdAt = runCatching { Instant.parse(event.string("createdAt")) }.getOrNull() ?: invalid(),
            expiresAt = runCatching { Instant.parse(event.string("expiresAt")) }.getOrNull() ?: invalid(),
        )
    }
    if (cursor < 0) invalid()
    return FeedPage(events, cursor, hasMore)
}

/**
 * What to do about a page of events. An expired event is not news. A request
 * answered later in the same page needs no notice at all, and an answer
 * retires whatever notice this phone showed for that request.
 */
internal fun planNotices(events: List<FeedEvent>, now: Instant): List<NoticeAction> {
    val live = events.filter { it.expiresAt.isAfter(now) }
    val answered = live.filter { it.kind == "visitor_decided" }.map { it.subject }.toSet()
    val actions = mutableListOf<NoticeAction>()
    for (event in live) {
        when (event.kind) {
            "visitor_request" -> if (event.subject !in answered) actions += NoticeAction.Show(event.subject, NoticeKind.REQUEST)
            "visitor_message" -> if (event.subject !in answered) actions += NoticeAction.Show(event.subject, NoticeKind.MESSAGE)
            "visitor_decided" -> actions += NoticeAction.Retire(event.subject)
        }
    }
    return actions
}

/** Reads the feed and shows or retires notices; used by the periodic job. */
class TimeRequestNotifier(
    private val configStore: SyncConfigStore,
    private val client: BackendSyncClient,
    private val cursors: NoticeCursorStore,
    private val poster: NoticePoster,
    private val now: () -> Instant = Instant::now,
) {
    /** Returns how many notices were shown. The first check only sets the cursor. */
    suspend fun check(): Result<Int> = syncResult {
        val config = configStore.load() ?: return@syncResult 0
        val scope = MessageDigest.getInstance("SHA-256").digest(config.token.toByteArray())
            .joinToString("") { "%02x".format(it) }
        val start = cursors.load(scope)
        if (start == null) {
            // Turning notices on is about what happens next, not the past week.
            val head = parseNotificationFeed(client.notifications(config.baseUrl, config.token, "latest").getOrThrow())
            cursors.save(scope, head.cursor)
            return@syncResult 0
        }
        var cursor: Long = start
        var shown = 0
        for (page in 1..MAX_PAGES_PER_CHECK) {
            val feed = parseNotificationFeed(client.notifications(config.baseUrl, config.token, cursor.toString()).getOrThrow())
            for (action in planNotices(feed.events, now())) {
                when (action) {
                    is NoticeAction.Show -> { poster.show(action.subject, action.kind); shown++ }
                    is NoticeAction.Retire -> poster.retire(action.subject)
                }
            }
            if (feed.cursor < cursor) throw BackendSyncException(200, "The notification feed went backwards.")
            cursor = feed.cursor
            cursors.save(scope, cursor)
            if (!feed.hasMore) break
        }
        shown
    }

    fun prepare() = poster.prepare()

    /** Turning notices off, or leaving the server, clears what they left behind. */
    fun reset() {
        cursors.clear()
        poster.retireAll()
    }

    internal companion object {
        const val FEED_LIMIT_BYTES = MAX_FEED_BYTES
    }
}
