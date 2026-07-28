package org.non24.planner.data

import android.content.Context
import androidx.health.connect.client.HealthConnectClient
import androidx.health.connect.client.records.SleepSessionRecord
import androidx.health.connect.client.request.ReadRecordsRequest
import androidx.health.connect.client.time.TimeRangeFilter
import java.security.MessageDigest
import java.time.Clock
import java.time.Duration
import java.time.Instant
import java.time.ZoneOffset
import java.util.concurrent.atomic.AtomicLong
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import org.non24.planner.domain.AcquisitionMethod
import org.non24.planner.domain.EvidenceStatus
import org.non24.planner.domain.HealthConnectAvailability
import org.non24.planner.domain.HealthPermissionState
import org.non24.planner.domain.Provenance
import org.non24.planner.domain.SleepEpisode

object HealthConnectPermissions {
    const val READ_SLEEP = "android.permission.health.READ_SLEEP"
    val required: Set<String> = setOf(READ_SLEEP)
}

interface HealthConnectClientAdapter {
    fun availability(): HealthConnectAvailability

    suspend fun grantedPermissions(): Set<String>

    suspend fun readRecentSleep(): List<SleepEpisode>
}

internal class HealthConnectSleepRepository(
    private val client: HealthConnectClientAdapter,
    localUserDataRepository: LocalUserDataRepository =
        LocalUserDataRepository(InMemoryLocalUserDataStore()),
) : CorrectableSleepRepository(localUserDataRepository), HealthConnectRepository {
    private val refreshMutex = Mutex()
    private val refreshRequestSequence = AtomicLong(0)
    private var completedRefreshRequest = 0L
    private val mutableAvailability = MutableStateFlow(client.availability())
    private val mutablePermissionState = MutableStateFlow(initialPermissionState(mutableAvailability.value))
    private val mutableLastRefreshError = MutableStateFlow<String?>(null)

    override val sourceEpisodes: StateFlow<List<SleepEpisode>> = localUserDataRepository.healthEpisodes
    override val availability: StateFlow<HealthConnectAvailability> = mutableAvailability.asStateFlow()
    override val permissionState: StateFlow<HealthPermissionState> = mutablePermissionState.asStateFlow()
    override val lastRefreshError: StateFlow<String?> = mutableLastRefreshError.asStateFlow()
    override val requiredPermissions: Set<String> = HealthConnectPermissions.required

    override suspend fun refreshPermissionState() = refreshMutex.withLock {
        refreshPermissionStateLocked()
    }

    private suspend fun refreshPermissionStateLocked() {
        mutableAvailability.value = client.availability()
        if (mutableAvailability.value != HealthConnectAvailability.AVAILABLE) {
            mutablePermissionState.value = HealthPermissionState.UNAVAILABLE
            return
        }
        val granted = try {
            client.grantedPermissions()
        } catch (_: SecurityException) {
            mutablePermissionState.value = HealthPermissionState.REQUIRED
            mutableLastRefreshError.value =
                "Health Connect permission changed. The last saved sleep snapshot is still shown."
            return
        } catch (exception: CancellationException) {
            throw exception
        } catch (_: Exception) {
            mutablePermissionState.value = HealthPermissionState.UNKNOWN
            mutableLastRefreshError.value =
                "Health Connect could not be queried. The last saved sleep snapshot is still shown."
            return
        }
        mutablePermissionState.value = if (granted.containsAll(requiredPermissions)) {
            HealthPermissionState.GRANTED
        } else {
            HealthPermissionState.REQUIRED
        }
    }

    override suspend fun onPermissionResult(grantedPermissions: Set<String>) {
        if (grantedPermissions.containsAll(requiredPermissions)) {
            requestRefresh()
            return
        }
        refreshMutex.withLock {
            mutablePermissionState.value = HealthPermissionState.REQUIRED
        }
    }

    override suspend fun refresh() {
        requestRefresh()
    }

    private suspend fun requestRefresh() {
        val requestId = refreshRequestSequence.incrementAndGet()
        refreshMutex.withLock {
            if (completedRefreshRequest >= requestId) return
            do {
                val generation = refreshRequestSequence.get()
                refreshLocked()
                completedRefreshRequest = generation
            } while (completedRefreshRequest < refreshRequestSequence.get())
        }
    }

    private suspend fun refreshLocked() {
        refreshPermissionStateLocked()
        if (permissionState.value != HealthPermissionState.GRANTED) {
            return
        }
        try {
            localUserDataRepository.initialize()
            val episodes = client.readRecentSleep().sortedByDescending { it.start }
            localUserDataRepository.replaceHealthConnectSleepSnapshot(episodes)
            mutableLastRefreshError.value = null
        } catch (_: SecurityException) {
            mutablePermissionState.value = HealthPermissionState.REQUIRED
            mutableLastRefreshError.value =
                "Health Connect permission changed. The last saved sleep snapshot is still shown."
        } catch (exception: CancellationException) {
            throw exception
        } catch (_: Exception) {
            mutableLastRefreshError.value =
                "Health Connect refresh failed. The last saved sleep snapshot is still shown."
        }
    }

    private companion object {
        fun initialPermissionState(availability: HealthConnectAvailability): HealthPermissionState =
            if (availability == HealthConnectAvailability.AVAILABLE) {
                HealthPermissionState.UNKNOWN
            } else {
                HealthPermissionState.UNAVAILABLE
            }
    }
}

internal class AndroidHealthConnectClientAdapter(
    private val context: Context,
    private val clock: Clock = Clock.systemUTC(),
) : HealthConnectClientAdapter {
    override fun availability(): HealthConnectAvailability =
        when (HealthConnectClient.getSdkStatus(context, HEALTH_CONNECT_PACKAGE)) {
            HealthConnectClient.SDK_AVAILABLE -> HealthConnectAvailability.AVAILABLE
            HealthConnectClient.SDK_UNAVAILABLE_PROVIDER_UPDATE_REQUIRED -> HealthConnectAvailability.UPDATE_REQUIRED
            else -> HealthConnectAvailability.UNAVAILABLE
        }

    override suspend fun grantedPermissions(): Set<String> {
        val healthClient = availableClient() ?: return emptySet()
        return healthClient.permissionController.getGrantedPermissions()
    }

    override suspend fun readRecentSleep(): List<SleepEpisode> {
        val healthClient = availableClient()
            ?: throw IllegalStateException("Health Connect became unavailable during refresh.")
        val now = Instant.now(clock)
        val timeRange = TimeRangeFilter.between(now.minus(Duration.ofDays(30)), now)
        val records = HealthConnectSleepPager().readAll { pageToken ->
            val response = healthClient.readRecords(
                ReadRecordsRequest(
                    recordType = SleepSessionRecord::class,
                    timeRangeFilter = timeRange,
                    ascendingOrder = false,
                    pageSize = HEALTH_CONNECT_PAGE_SIZE,
                    pageToken = pageToken,
                ),
            )
            HealthConnectSleepPage(
                records = response.records.map(SleepSessionRecord::toImportedRecord),
                nextPageToken = response.pageToken,
            )
        }
        return records.map(::toSleepEpisode).sortedByDescending { it.start }
    }

    private fun availableClient(): HealthConnectClient? =
        if (availability() == HealthConnectAvailability.AVAILABLE) {
            HealthConnectClient.getOrCreate(context)
        } else {
            null
        }

    private companion object {
        const val HEALTH_CONNECT_PACKAGE = "com.google.android.apps.healthdata"
        const val HEALTH_CONNECT_PAGE_SIZE = 1_000
    }
}

internal data class HealthConnectSleepRecord(
    val sourceId: String,
    val sourceRecordId: String?,
    val sourceUpdatedAt: Instant,
    val start: Instant,
    val end: Instant,
    val startZoneOffset: ZoneOffset?,
    val endZoneOffset: ZoneOffset?,
)

internal data class HealthConnectSleepPage(
    val records: List<HealthConnectSleepRecord>,
    val nextPageToken: String?,
)

internal class HealthConnectSleepPager(
    private val maxRecords: Int = DEFAULT_MAX_RECORDS,
    private val maxPages: Int = DEFAULT_MAX_PAGES,
) {
    init {
        require(maxRecords > 0) { "Health Connect record limit must be positive." }
        require(maxPages > 0) { "Health Connect page limit must be positive." }
    }

    suspend fun readAll(
        readPage: suspend (pageToken: String?) -> HealthConnectSleepPage,
    ): List<HealthConnectSleepRecord> {
        val unique = LinkedHashMap<String, HealthConnectSleepRecord>()
        val seenPageTokens = HashSet<String>()
        var pageToken: String? = null

        repeat(maxPages) {
            val page = readPage(pageToken)
            page.records.forEach { record ->
                val sourceKey = stableHealthConnectSourceKey(record)
                val existing = unique[sourceKey]
                when {
                    existing == null -> {
                        if (unique.size == maxRecords) {
                            throw HealthConnectReadLimitException(maxRecords)
                        }
                        unique[sourceKey] = record
                    }
                    record.sourceUpdatedAt.isAfter(existing.sourceUpdatedAt) ->
                        unique[sourceKey] = record
                    record.sourceUpdatedAt.isBefore(existing.sourceUpdatedAt) -> Unit
                    existing != record ->
                        throw IllegalStateException(
                            "Health Connect returned conflicting revisions for one source record.",
                        )
                }
            }

            val nextPageToken = page.nextPageToken ?: return unique.values.toList()
            if (!seenPageTokens.add(nextPageToken)) {
                throw IllegalStateException("Health Connect returned a repeated page token.")
            }
            pageToken = nextPageToken
        }
        throw IllegalStateException("Health Connect exceeded the $maxPages page safety limit.")
    }

    private companion object {
        const val DEFAULT_MAX_RECORDS = 10_000
        const val DEFAULT_MAX_PAGES = 100
    }
}

internal class HealthConnectReadLimitException(maxRecords: Int) :
    IllegalStateException("Health Connect returned more than $maxRecords sleep records.")

private fun SleepSessionRecord.toImportedRecord(): HealthConnectSleepRecord =
    HealthConnectSleepRecord(
        sourceId = metadata.dataOrigin.packageName,
        sourceRecordId = metadata.id
            .ifBlank { metadata.clientRecordId.orEmpty() }
            .ifBlank { null },
        sourceUpdatedAt = metadata.lastModifiedTime,
        start = startTime,
        end = endTime,
        startZoneOffset = startZoneOffset,
        endZoneOffset = endZoneOffset,
    )

internal fun toSleepEpisode(record: HealthConnectSleepRecord): SleepEpisode {
    val logicalSourceId = stableHealthConnectSourceKey(record)
    return SleepEpisode(
        id = stableHealthConnectEpisodeId(logicalSourceId, record.sourceUpdatedAt),
        logicalSourceId = logicalSourceId,
        start = record.start,
        end = record.end,
        ianaTimeZoneId = null,
        startZoneOffset = record.startZoneOffset,
        endZoneOffset = record.endZoneOffset,
        provenance = Provenance(
            acquisitionMethod = AcquisitionMethod.HEALTH_CONNECT,
            evidenceStatus = EvidenceStatus.IMPORTED,
            sourceId = record.sourceId,
            sourceRecordId = record.sourceRecordId,
            sourceUpdatedAt = record.sourceUpdatedAt,
        ),
    )
}

private fun stableHealthConnectSourceKey(record: HealthConnectSleepRecord): String =
    healthConnectLogicalSourceId(
        record.sourceId,
        record.sourceRecordId,
        record.start,
        record.end,
    )

private fun stableHealthConnectEpisodeId(logicalSourceId: String, sourceUpdatedAt: Instant): String {
    val material = listOf(logicalSourceId, sourceUpdatedAt.toString())
        .joinToString(separator = "\u0000")
    val digest = MessageDigest.getInstance("SHA-256").digest(material.toByteArray(Charsets.UTF_8))
    return "health-connect-" + digest.take(16).joinToString(separator = "") { byte ->
        "%02x".format(byte.toInt() and 0xff)
    }
}

internal fun healthConnectLogicalSourceId(
    sourceId: String,
    sourceRecordId: String?,
    start: Instant,
    end: Instant,
): String {
    val sourceIdentity = sourceRecordId?.takeIf(String::isNotBlank)
        ?: "${start.epochSecond}:${start.nano}\u001f${end.epochSecond}:${end.nano}"
    return "$sourceId\u001f$sourceIdentity"
}
