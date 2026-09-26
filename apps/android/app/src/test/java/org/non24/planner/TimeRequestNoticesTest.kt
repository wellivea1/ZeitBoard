package org.non24.planner

import java.time.Instant
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test
import org.non24.planner.data.BackendSyncClient
import org.non24.planner.data.BackendSyncException
import org.non24.planner.data.FeedEvent
import org.non24.planner.data.NoticeAction
import org.non24.planner.data.NoticeCursorStore
import org.non24.planner.data.NoticeKind
import org.non24.planner.data.NoticePoster
import org.non24.planner.data.OutboxRecord
import org.non24.planner.data.PullPage
import org.non24.planner.data.SyncConfig
import org.non24.planner.data.SyncConfigStore
import org.non24.planner.data.TimeRequestNotifier
import org.non24.planner.data.parseNotificationFeed
import org.non24.planner.data.planNotices

class TimeRequestNoticesTest {
    private val now = Instant.parse("2026-09-26T12:00:00Z")

    private fun feed(cursor: Long, vararg events: String, hasMore: Boolean = false): JsonObject =
        Json.parseToJsonElement(
            """{"schema_version":"v1","cursor":$cursor,"hasMore":$hasMore,"events":[${events.joinToString(",")}]}""",
        ).jsonObject

    private fun event(id: String, kind: String, subject: String, expires: String = "2026-10-03T12:00:00Z") =
        """{"eventId":"$id","kind":"$kind","subject":"$subject","createdAt":"2026-09-26T11:50:00Z","expiresAt":"$expires"}"""

    private fun feedEvent(kind: String, subject: String, expiresAt: Instant = now.plusSeconds(3600)) =
        FeedEvent("e-$kind-$subject", kind, subject, now.minusSeconds(60), expiresAt)

    @Test
    fun parsesOnlyTheFeedItKnows() {
        val page = parseNotificationFeed(feed(7, event("e1", "visitor_request", "p1")))
        assertEquals(7L, page.cursor)
        assertEquals("p1", page.events.single().subject)

        for (bad in listOf(
            feed(7, event("e1", "visitor_telepathy", "p1")),
            Json.parseToJsonElement("""{"schema_version":"v1","events":[]}""").jsonObject,
            Json.parseToJsonElement("""{"schema_version":"v9","cursor":1,"events":[]}""").jsonObject,
            feed(-1),
        )) {
            assertThrows(BackendSyncException::class.java) { parseNotificationFeed(bad) }
        }
    }

    @Test
    fun anAnswerRetiresItsNoticeAndSuppressesOneStillUnshown() {
        val actions = planNotices(
            listOf(
                feedEvent("visitor_request", "p1"),
                feedEvent("visitor_request", "p2"),
                feedEvent("visitor_message", "p2"),
                feedEvent("visitor_decided", "p1"),
                feedEvent("visitor_request", "p3", expiresAt = now.minusSeconds(1)),
            ),
            now,
        )
        assertEquals(
            listOf(
                NoticeAction.Show("p2", NoticeKind.REQUEST),
                NoticeAction.Show("p2", NoticeKind.MESSAGE),
                NoticeAction.Retire("p1"),
            ),
            actions,
        )
    }

    @Test
    fun turningNoticesOnStartsAtTheHeadThenShowsWhatComesNext() = runTest {
        val client = FeedClient()
        val cursors = MemoryCursors()
        val poster = RecordingPoster()
        val notifier = TimeRequestNotifier(Configured, client, cursors, poster) { now }

        client.pages += feed(40)
        assertEquals(0, notifier.check().getOrThrow())
        assertEquals(listOf("latest"), client.asked)
        assertTrue(poster.log.isEmpty())

        client.pages += feed(42, event("e41", "visitor_request", "p9"), event("e42", "visitor_message", "p9"))
        assertEquals(2, notifier.check().getOrThrow())
        assertEquals("40", client.asked.last())
        assertEquals(listOf("show p9 REQUEST", "show p9 MESSAGE"), poster.log)

        client.pages += feed(43, event("e43", "visitor_decided", "p9"))
        notifier.check().getOrThrow()
        assertEquals("42", client.asked.last())
        assertEquals("retire p9", poster.log.last())
    }

    @Test
    fun aFailedReadKeepsItsPlace() = runTest {
        val client = FeedClient()
        val cursors = MemoryCursors()
        val notifier = TimeRequestNotifier(Configured, client, cursors, RecordingPoster()) { now }
        client.pages += feed(10)
        notifier.check().getOrThrow()

        client.failNext = true
        assertTrue(notifier.check().isFailure)
        client.pages += feed(10)
        notifier.check().getOrThrow()
        assertEquals("10", client.asked.last())
    }

    @Test
    fun resetForgetsThePlaceAndClearsNotices() = runTest {
        val cursors = MemoryCursors()
        val poster = RecordingPoster()
        val client = FeedClient()
        val notifier = TimeRequestNotifier(Configured, client, cursors, poster) { now }
        client.pages += feed(5)
        notifier.check().getOrThrow()
        notifier.reset()
        assertNull(cursors.saved)
        assertEquals("retire all", poster.log.last())
    }

    @Test
    fun withoutAServerThereIsNothingToRead() = runTest {
        val client = FeedClient()
        val notifier = TimeRequestNotifier(Unconfigured, client, MemoryCursors(), RecordingPoster()) { now }
        assertEquals(0, notifier.check().getOrThrow())
        assertTrue(client.asked.isEmpty())
    }

    private object Configured : SyncConfigStore {
        override fun load() = SyncConfig("https://zeitboard.example.test", "synthetic-token", "UTC", "scope")
        override fun save(config: SyncConfig) = Unit
        override fun clear() = Unit
    }

    private object Unconfigured : SyncConfigStore {
        override fun load(): SyncConfig? = null
        override fun save(config: SyncConfig) = Unit
        override fun clear() = Unit
    }

    private class MemoryCursors : NoticeCursorStore {
        var saved: Pair<String, Long>? = null
        override fun load(scope: String): Long? = saved?.takeIf { it.first == scope }?.second
        override fun save(scope: String, cursor: Long) { saved = scope to cursor }
        override fun clear() { saved = null }
    }

    private class RecordingPoster : NoticePoster {
        val log = mutableListOf<String>()
        override fun show(subject: String, kind: NoticeKind) { log += "show $subject $kind" }
        override fun retire(subject: String) { log += "retire $subject" }
        override fun retireAll() { log += "retire all" }
    }

    private class FeedClient : BackendSyncClient {
        val pages = ArrayDeque<JsonObject>()
        val asked = mutableListOf<String>()
        var failNext = false

        override suspend fun notifications(baseUrl: String, token: String, after: String): Result<JsonObject> {
            asked += after
            if (failNext) {
                failNext = false
                return Result.failure(BackendSyncException(503, "unavailable"))
            }
            return Result.success(pages.removeFirst())
        }

        override suspend fun enroll(baseUrl: String, enrollmentSecret: String, label: String): Result<String> = error("unused")
        override suspend fun push(baseUrl: String, token: String, records: List<OutboxRecord>): Result<List<String>> = error("unused")
        override suspend fun pull(baseUrl: String, token: String, since: Long): Result<PullPage> = error("unused")
        override suspend fun companion(baseUrl: String, token: String): Result<JsonObject> = error("unused")
        override suspend fun sleepReview(baseUrl: String, token: String, observationId: String): Result<JsonObject> = error("unused")
    }
}
