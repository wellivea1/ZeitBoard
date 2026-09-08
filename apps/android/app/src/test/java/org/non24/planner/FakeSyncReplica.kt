package org.non24.planner

import java.time.Instant
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.non24.planner.data.*
import org.non24.planner.domain.SleepEpisode

/** The transport/outbox unit tests use an empty downloaded history. Native tests cover SQLite replay. */
internal class FakeReplica : SyncReplicaStore {
    private var current = CompanionState()
    var review: SleepReview? = null
    override fun cachedReview(observationId: String) = review?.takeIf { it.observationId == observationId }
    override fun cacheReview(review: JsonObject) { this.review = parseSleepReview(review) }
    override fun activateScope(scope: String) = Unit
    override fun cursor() = 0L
    override fun apply(page: PullPage, receivedAt: Instant) { check(page.cursor == 0L && page.records.isEmpty()) }
    override fun cache(projection: JsonObject, receivedAt: Instant) { current = CompanionState(parseCompanion(projection), downloadedAt = receivedAt) }
    override fun state() = current
    override fun knownSources() = emptyMap<String, SourceSyncRevision>()
    override fun clear() { current = CompanionState(); review = null }
}

internal fun emptyServerProjection() = JsonObject(companionFixture() + ("source_cursor" to JsonPrimitive(0)))

internal suspend fun BackendSyncRepository.uploadEpisodes(episodes: List<SleepEpisode>): Result<Int> {
    enqueue(episodes)
    return synchronize()
}
