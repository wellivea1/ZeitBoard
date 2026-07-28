package org.non24.planner.data

import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import org.non24.planner.domain.MedicationEvent
import org.non24.planner.domain.SleepCorrection
import org.non24.planner.domain.SleepCorrectionReview
import org.non24.planner.domain.SleepEpisode
import org.non24.planner.domain.isCorrectionNewer

sealed interface DurableLocalDataState {
    data object Loading : DurableLocalDataState

    data object Ready : DurableLocalDataState

    data class Failed(
        val message: String = "ZeitBoard could not access its saved local data.",
    ) : DurableLocalDataState
}

internal interface LocalUserDataStore {
    suspend fun loadHealthConnectSleepSnapshot(limit: Int): List<SleepEpisode>

    suspend fun replaceHealthConnectSleepSnapshot(episodes: List<SleepEpisode>)

    suspend fun loadSleepCorrections(limit: Int): List<SleepCorrection>

    suspend fun loadLatestSleepCorrectionsForTargets(
        targetEpisodeIds: Set<String>,
    ): List<SleepCorrection>

    suspend fun loadLatestSleepCorrectionsForLogicalSources(
        logicalSourceIds: Set<String>,
    ): List<SleepCorrection>

    suspend fun appendSleepCorrection(correction: SleepCorrection)

    suspend fun loadMedicationEvents(limit: Int): List<MedicationEvent>

    suspend fun appendMedicationEvent(event: MedicationEvent)
}

internal class InMemoryLocalUserDataStore : LocalUserDataStore {
    private val mutex = Mutex()
    private val healthEpisodes = LinkedHashMap<String, SleepEpisode>()
    private var healthSnapshotIds = emptyList<String>()
    private val sleepCorrections = LinkedHashMap<String, SleepCorrection>()
    private val medicationEvents = LinkedHashMap<String, MedicationEvent>()

    override suspend fun loadHealthConnectSleepSnapshot(limit: Int): List<SleepEpisode> =
        mutex.withLock {
            healthSnapshotIds
                .asSequence()
                .mapNotNull(healthEpisodes::get)
                .sortedByDescending { it.start }
                .take(limit)
                .toList()
        }

    override suspend fun replaceHealthConnectSleepSnapshot(episodes: List<SleepEpisode>) {
        mutex.withLock {
            val unique = requireImmutableIds(episodes) { it.id }
            unique.forEach { episode ->
                val existing = healthEpisodes[episode.id]
                require(existing == null || existing == episode) {
                    "Health Connect observation ${episode.id} changed without a source revision."
                }
            }
            unique.forEach { episode ->
                healthEpisodes.putIfAbsent(episode.id, episode)
            }
            healthSnapshotIds = unique.map { it.id }
        }
    }

    override suspend fun loadSleepCorrections(limit: Int): List<SleepCorrection> =
        mutex.withLock {
            sleepCorrections.values
                .sortedWith(compareByDescending<SleepCorrection> { it.createdAt }.thenByDescending { it.id })
                .take(limit)
                .sortedWith(compareBy<SleepCorrection> { it.createdAt }.thenBy { it.id })
        }

    override suspend fun loadLatestSleepCorrectionsForTargets(
        targetEpisodeIds: Set<String>,
    ): List<SleepCorrection> = mutex.withLock {
        latestCorrections(
            allowedKeys = targetEpisodeIds,
            key = SleepCorrection::targetEpisodeId,
        )
    }

    override suspend fun loadLatestSleepCorrectionsForLogicalSources(
        logicalSourceIds: Set<String>,
    ): List<SleepCorrection> = mutex.withLock {
        latestCorrections(
            allowedKeys = logicalSourceIds,
            key = SleepCorrection::targetLogicalSourceId,
        )
    }

    private fun latestCorrections(
        allowedKeys: Set<String>,
        key: (SleepCorrection) -> String,
    ): List<SleepCorrection> {
        val latest = HashMap<String, SleepCorrection>()
        sleepCorrections.values.forEach { candidate ->
            val candidateKey = key(candidate)
            if (candidateKey !in allowedKeys) return@forEach
            val selected = latest[candidateKey]
            if (selected == null || isCorrectionNewer(candidate, selected)) {
                latest[candidateKey] = candidate
            }
        }
        return latest.values.sortedWith(compareBy<SleepCorrection> { it.createdAt }.thenBy { it.id })
    }

    override suspend fun appendSleepCorrection(correction: SleepCorrection) {
        mutex.withLock {
            val existing = sleepCorrections[correction.id]
            require(existing == null || existing == correction) {
                "Sleep correction ${correction.id} already exists with different content."
            }
            sleepCorrections.putIfAbsent(correction.id, correction)
        }
    }

    override suspend fun loadMedicationEvents(limit: Int): List<MedicationEvent> =
        mutex.withLock {
            medicationEvents.values
                .sortedWith(compareByDescending<MedicationEvent> { it.occurredAt }.thenByDescending { it.id })
                .take(limit)
        }

    override suspend fun appendMedicationEvent(event: MedicationEvent) {
        mutex.withLock {
            val existing = medicationEvents[event.id]
            require(existing == null || existing == event) {
                "Medication event ${event.id} already exists with different content."
            }
            medicationEvents.putIfAbsent(event.id, event)
        }
    }
}

internal class LocalUserDataRepository(
    private val store: LocalUserDataStore,
    staticSleepEpisodes: List<SleepEpisode> = emptyList(),
    private val correctionHistoryLimit: Int = MAX_CORRECTIONS,
) {
    init {
        require(correctionHistoryLimit > 0) { "Correction history limit must be positive." }
    }

    private val mutex = Mutex()
    private val staticSleepEpisodes = requireCurrentLogicalSources(staticSleepEpisodes)
    private var initialized = false
    private var latestCorrectionsByLogicalSource = emptyMap<String, SleepCorrection>()
    private val mutableHealthEpisodes = MutableStateFlow<List<SleepEpisode>>(emptyList())
    private val mutableActiveCorrections = MutableStateFlow<Map<String, SleepCorrection>>(emptyMap())
    private val mutableCorrectionHistory = MutableStateFlow<List<SleepCorrection>>(emptyList())
    private val mutableCorrectionReviews = MutableStateFlow<List<SleepCorrectionReview>>(emptyList())
    private val mutableMedicationEvents = MutableStateFlow<List<MedicationEvent>>(emptyList())
    private val mutableState = MutableStateFlow<DurableLocalDataState>(DurableLocalDataState.Loading)

    val healthEpisodes: StateFlow<List<SleepEpisode>> = mutableHealthEpisodes.asStateFlow()
    val activeCorrections: StateFlow<Map<String, SleepCorrection>> = mutableActiveCorrections.asStateFlow()
    val correctionHistory: StateFlow<List<SleepCorrection>> = mutableCorrectionHistory.asStateFlow()
    val correctionReviews: StateFlow<List<SleepCorrectionReview>> = mutableCorrectionReviews.asStateFlow()
    val medicationEvents: StateFlow<List<MedicationEvent>> = mutableMedicationEvents.asStateFlow()
    val state: StateFlow<DurableLocalDataState> = mutableState.asStateFlow()

    suspend fun initialize() {
        mutex.withLock {
            runStorageOperationLocked { initializeLocked() }
        }
    }

    suspend fun replaceHealthConnectSleepSnapshot(episodes: List<SleepEpisode>) {
        val unique = requireImmutableIds(episodes) { it.id }
        require(unique.size <= MAX_HEALTH_EPISODES) {
            "Sleep snapshot exceeds the $MAX_HEALTH_EPISODES record safety limit."
        }
        val sorted = requireCurrentLogicalSources(unique).sortedByDescending { it.start }
        mutex.withLock {
            runStorageOperationLocked {
                initializeLocked()
                store.replaceHealthConnectSleepSnapshot(sorted)
                val projection = loadCorrectionProjection(sorted)
                mutableHealthEpisodes.value = sorted
                publishCorrectionProjection(projection, sorted)
            }
        }
    }

    suspend fun appendSleepCorrection(correction: SleepCorrection) {
        mutex.withLock {
            runStorageOperationLocked {
                initializeLocked()
                store.appendSleepCorrection(correction)
                mutableCorrectionHistory.value = insertSortedBounded(
                    current = mutableCorrectionHistory.value,
                    value = correction,
                    comparator = CORRECTION_COMPARATOR,
                    limit = correctionHistoryLimit,
                    keepNewestAtEnd = true,
                ) { it.id }

                val latestByTarget = mutableActiveCorrections.value.toMutableMap()
                val selectedForTarget = latestByTarget[correction.targetEpisodeId]
                if (selectedForTarget == null || isCorrectionNewer(correction, selectedForTarget)) {
                    latestByTarget[correction.targetEpisodeId] = correction
                }
                val latestByLogical = latestCorrectionsByLogicalSource.toMutableMap()
                val selectedForLogical = latestByLogical[correction.targetLogicalSourceId]
                if (selectedForLogical == null || isCorrectionNewer(correction, selectedForLogical)) {
                    latestByLogical[correction.targetLogicalSourceId] = correction
                }
                publishCorrectionProjection(
                    CorrectionProjection(latestByTarget, latestByLogical),
                    mutableHealthEpisodes.value,
                )
            }
        }
    }

    suspend fun appendMedicationEvent(event: MedicationEvent) {
        mutex.withLock {
            runStorageOperationLocked {
                initializeLocked()
                store.appendMedicationEvent(event)
                mutableMedicationEvents.value = insertSortedBounded(
                    current = mutableMedicationEvents.value,
                    value = event,
                    comparator = MEDICATION_COMPARATOR,
                    limit = MAX_MEDICATION_EVENTS,
                    keepNewestAtEnd = false,
                ) { it.id }
            }
        }
    }

    private suspend fun initializeLocked() {
        if (initialized && mutableState.value == DurableLocalDataState.Ready) return
        val health = requireCurrentLogicalSources(
            store.loadHealthConnectSleepSnapshot(MAX_HEALTH_EPISODES),
        ).sortedByDescending { it.start }
        val correctionHistory = store.loadSleepCorrections(correctionHistoryLimit)
        val medications = store.loadMedicationEvents(MAX_MEDICATION_EVENTS)
        val projection = loadCorrectionProjection(health)

        mutableHealthEpisodes.value = health.take(MAX_HEALTH_EPISODES)
        mutableCorrectionHistory.value = requireImmutableIds(correctionHistory) { it.id }
            .sortedWith(CORRECTION_COMPARATOR)
            .takeLast(correctionHistoryLimit)
        mutableMedicationEvents.value = requireImmutableIds(medications) { it.id }
            .sortedWith(MEDICATION_COMPARATOR)
            .take(MAX_MEDICATION_EVENTS)
        publishCorrectionProjection(projection, health)
        initialized = true
    }

    private suspend fun loadCorrectionProjection(
        healthEpisodes: List<SleepEpisode>,
    ): CorrectionProjection {
        val currentSources = currentSleepSources(healthEpisodes)
        val targetIds = currentSources.mapTo(LinkedHashSet(), SleepEpisode::id)
        val logicalSourceIds = currentSources.mapTo(LinkedHashSet(), SleepEpisode::logicalSourceId)
        val latestByTarget = store.loadLatestSleepCorrectionsForTargets(targetIds)
            .associateBy(SleepCorrection::targetEpisodeId)
        val latestByLogical = store.loadLatestSleepCorrectionsForLogicalSources(logicalSourceIds)
            .associateBy(SleepCorrection::targetLogicalSourceId)
        return CorrectionProjection(latestByTarget, latestByLogical)
    }

    private fun publishCorrectionProjection(
        projection: CorrectionProjection,
        healthEpisodes: List<SleepEpisode>,
    ) {
        val currentSources = currentSleepSources(healthEpisodes)
        val currentTargetIds = currentSources.mapTo(HashSet(), SleepEpisode::id)
        val currentByLogicalSource = currentSources.associateBy(SleepEpisode::logicalSourceId)
        val active = projection.latestByTarget.filterKeys(currentTargetIds::contains)
        val latestByLogical = projection.latestByLogicalSource
            .filterKeys(currentByLogicalSource::containsKey)

        mutableActiveCorrections.value = active
        latestCorrectionsByLogicalSource = latestByLogical
        mutableCorrectionReviews.value = latestByLogical.values
            .mapNotNull { correction ->
                val current = currentByLogicalSource[correction.targetLogicalSourceId]
                    ?: return@mapNotNull null
                if (correction.targetEpisodeId == current.id) {
                    null
                } else {
                    SleepCorrectionReview(correction, current.id)
                }
            }
            .sortedWith(
                compareByDescending<SleepCorrectionReview> { it.correction.createdAt }
                    .thenByDescending { it.correction.id },
            )
    }

    private fun currentSleepSources(healthEpisodes: List<SleepEpisode>): List<SleepEpisode> =
        requireCurrentLogicalSources(staticSleepEpisodes + healthEpisodes)

    private suspend fun <T> runStorageOperationLocked(operation: suspend () -> T): T {
        return try {
            operation().also { mutableState.value = DurableLocalDataState.Ready }
        } catch (exception: CancellationException) {
            throw exception
        } catch (exception: Exception) {
            mutableState.value = DurableLocalDataState.Failed()
            throw exception
        }
    }

    private companion object {
        const val MAX_HEALTH_EPISODES = 10_000
        const val MAX_CORRECTIONS = 50_000
        const val MAX_MEDICATION_EVENTS = 10_000
        val CORRECTION_COMPARATOR: Comparator<SleepCorrection> =
            compareBy<SleepCorrection> { it.createdAt }.thenBy { it.id }
        val MEDICATION_COMPARATOR: Comparator<MedicationEvent> =
            compareByDescending<MedicationEvent> { it.occurredAt }.thenByDescending { it.id }
    }
}

private data class CorrectionProjection(
    val latestByTarget: Map<String, SleepCorrection>,
    val latestByLogicalSource: Map<String, SleepCorrection>,
)

private fun <T> insertSortedBounded(
    current: List<T>,
    value: T,
    comparator: Comparator<T>,
    limit: Int,
    keepNewestAtEnd: Boolean,
    id: (T) -> String,
): List<T> {
    val recordId = id(value)
    current.firstOrNull { id(it) == recordId }?.let { existing ->
        require(existing == value) { "Record $recordId already exists with different content." }
        return current
    }
    val index = current.binarySearch(value, comparator).let { result ->
        if (result >= 0) result else -result - 1
    }
    val updated = ArrayList<T>(minOf(current.size + 1, limit + 1))
    updated.addAll(current.subList(0, index))
    updated.add(value)
    updated.addAll(current.subList(index, current.size))
    return when {
        updated.size <= limit -> updated
        keepNewestAtEnd -> updated.subList(updated.size - limit, updated.size).toList()
        else -> updated.subList(0, limit).toList()
    }
}

private fun <T> requireImmutableIds(values: List<T>, id: (T) -> String): List<T> {
    val unique = LinkedHashMap<String, T>()
    values.forEach { value ->
        val recordId = id(value)
        val existing = unique[recordId]
        require(existing == null || existing == value) {
            "Record $recordId appears more than once with different content."
        }
        unique.putIfAbsent(recordId, value)
    }
    return unique.values.toList()
}

private fun requireCurrentLogicalSources(values: List<SleepEpisode>): List<SleepEpisode> {
    val unique = requireImmutableIds(values, SleepEpisode::id)
    val byLogicalSource = HashMap<String, SleepEpisode>()
    unique.forEach { episode ->
        val existing = byLogicalSource[episode.logicalSourceId]
        require(existing == null || existing.id == episode.id) {
            "Logical sleep source ${episode.logicalSourceId} has multiple current revisions."
        }
        byLogicalSource[episode.logicalSourceId] = episode
    }
    return unique
}
