package org.non24.planner

import java.time.Instant
import java.time.ZoneOffset
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.non24.planner.data.HealthConnectReadLimitException
import org.non24.planner.data.HealthConnectSleepPage
import org.non24.planner.data.HealthConnectSleepPager
import org.non24.planner.data.HealthConnectSleepRecord
import org.non24.planner.data.toSleepEpisode

class HealthConnectSleepPagerTest {
    @Test
    fun readsEveryPageTokenAndDeduplicatesIncrementally() = runTest {
        val first = record("first", "2026-06-14T03:00:00Z")
        val second = record("second", "2026-06-15T03:00:00Z")
        val requestedTokens = mutableListOf<String?>()

        val result = HealthConnectSleepPager().readAll { token ->
            requestedTokens += token
            when (token) {
                null -> HealthConnectSleepPage(listOf(first), "page-2")
                "page-2" -> HealthConnectSleepPage(listOf(first, second), null)
                else -> error("Unexpected page token $token")
            }
        }

        assertEquals(listOf(null, "page-2"), requestedTokens)
        assertEquals(listOf(first, second), result)
    }

    @Test
    fun keepsOnlyTheNewestRevisionForEachSourceRecord() = runTest {
        val older = record(
            sourceRecordId = "revised",
            start = "2026-06-14T03:00:00Z",
            sourceUpdatedAt = "2026-06-15T12:00:00Z",
        )
        val newer = older.copy(
            end = older.end.plusSeconds(60),
            sourceUpdatedAt = older.sourceUpdatedAt.plusSeconds(1),
        )

        val result = HealthConnectSleepPager().readAll { token ->
            when (token) {
                null -> HealthConnectSleepPage(listOf(newer), "page-2")
                "page-2" -> HealthConnectSleepPage(listOf(older), null)
                else -> error("Unexpected page token $token")
            }
        }

        assertEquals(listOf(newer), result)
    }

    @Test
    fun rejectsMoreUniqueRecordsThanTheExplicitCap() = runTest {
        val error = runCatching {
            HealthConnectSleepPager(maxRecords = 2).readAll {
                HealthConnectSleepPage(
                    records = listOf(
                        record("first", "2026-06-14T03:00:00Z"),
                        record("second", "2026-06-15T03:00:00Z"),
                        record("third", "2026-06-16T03:00:00Z"),
                    ),
                    nextPageToken = null,
                )
            }
        }.exceptionOrNull()

        assertTrue(error is HealthConnectReadLimitException)
    }

    @Test
    fun rejectsRepeatedPageTokensInsteadOfLooping() = runTest {
        var page = 0
        val error = runCatching {
            HealthConnectSleepPager().readAll {
                page += 1
                HealthConnectSleepPage(
                    records = emptyList(),
                    nextPageToken = if (page <= 2) "same-token" else null,
                )
            }
        }.exceptionOrNull()

        assertTrue(error is IllegalStateException)
        assertEquals(2, page)
    }

    @Test
    fun mappingPreservesEndpointOffsetsWithoutInventingAnIanaZone() {
        val imported = record(
            sourceRecordId = "travel-session",
            start = "2026-11-01T05:30:00Z",
            startOffset = ZoneOffset.ofHours(-4),
            endOffset = ZoneOffset.ofHours(-5),
        )

        val episode = toSleepEpisode(imported)

        assertNull(episode.ianaTimeZoneId)
        assertEquals(ZoneOffset.ofHours(-4), episode.startZoneOffset)
        assertEquals(ZoneOffset.ofHours(-5), episode.endZoneOffset)
        assertEquals("travel-session", episode.provenance.sourceRecordId)
        assertEquals(imported.sourceUpdatedAt, episode.provenance.sourceUpdatedAt)
        assertEquals(episode.id, toSleepEpisode(imported).id)
        assertNotEquals(
            episode.id,
            toSleepEpisode(imported.copy(sourceUpdatedAt = imported.sourceUpdatedAt.plusSeconds(1))).id,
        )
    }

    private fun record(
        sourceRecordId: String,
        start: String,
        startOffset: ZoneOffset = ZoneOffset.ofHours(-4),
        endOffset: ZoneOffset = ZoneOffset.ofHours(-4),
        sourceUpdatedAt: String = "2026-06-16T12:00:00Z",
    ): HealthConnectSleepRecord {
        val startInstant = Instant.parse(start)
        return HealthConnectSleepRecord(
            sourceId = "synthetic.health.provider",
            sourceRecordId = sourceRecordId,
            sourceUpdatedAt = Instant.parse(sourceUpdatedAt),
            start = startInstant,
            end = startInstant.plusSeconds(8 * 60 * 60),
            startZoneOffset = startOffset,
            endZoneOffset = endOffset,
        )
    }
}
