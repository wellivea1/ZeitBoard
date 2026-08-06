package org.non24.planner

import java.time.Clock
import java.time.Instant
import java.time.ZoneOffset
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.test.TestDispatcher
import kotlinx.coroutines.test.UnconfinedTestDispatcher
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TestWatcher
import org.junit.runner.Description
import org.non24.planner.data.DurableLocalDataState
import org.non24.planner.data.EstimateRepository
import org.non24.planner.data.HealthConnectRepository
import org.non24.planner.data.BackendSyncClient
import org.non24.planner.data.BackendSyncRepository
import org.non24.planner.data.MedicationRepository
import org.non24.planner.data.OutboxRecord
import org.non24.planner.data.SyncConfig
import org.non24.planner.data.SyncConfigStore
import org.non24.planner.data.SyncOutboxStore
import org.non24.planner.data.SettingsRepository
import org.non24.planner.data.SleepRepository
import org.non24.planner.domain.AppSettings
import org.non24.planner.domain.EstimateSnapshot
import org.non24.planner.domain.HealthConnectAvailability
import org.non24.planner.domain.HealthPermissionState
import org.non24.planner.domain.MedicationEvent
import org.non24.planner.domain.SleepCorrection
import org.non24.planner.domain.SleepCorrectionReview
import org.non24.planner.domain.SleepEpisode
import org.non24.planner.ui.AppViewModel
import org.non24.planner.ui.MedicationSaveState

@OptIn(ExperimentalCoroutinesApi::class)
class AppViewModelMedicationTest {
    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    @Test
    fun parseFailurePublishesFailureWithoutCallingStorage() = runTest {
        val medicationRepository = RecordingMedicationRepository()
        val viewModel = viewModel(medicationRepository)

        viewModel.addMedicationEvent("Synthetic medication", "not a date")

        assertTrue(viewModel.medicationSaveState.value is MedicationSaveState.Failed)
        assertEquals(0, medicationRepository.calls.size)
    }

    @Test
    fun pendingSaveRejectsDuplicateSubmissionUntilPersistenceCompletes() = runTest {
        val medicationRepository = RecordingMedicationRepository().apply {
            gate = CompletableDeferred()
        }
        val viewModel = viewModel(medicationRepository)

        viewModel.addMedicationEvent("Synthetic medication", VALID_TIME)
        viewModel.addMedicationEvent("Synthetic medication", VALID_TIME)

        assertTrue(viewModel.medicationSaveState.value is MedicationSaveState.Saving)
        assertEquals(1, medicationRepository.calls.size)

        medicationRepository.gate?.complete(Unit)

        assertTrue(viewModel.medicationSaveState.value is MedicationSaveState.Succeeded)
        assertEquals(1, medicationRepository.calls.size)
    }

    @Test
    fun failedPersistenceKeepsStableEventIdentityForIdempotentRetry() = runTest {
        val medicationRepository = RecordingMedicationRepository().apply {
            failure = IllegalStateException("synthetic storage failure")
        }
        val viewModel = viewModel(medicationRepository)

        viewModel.addMedicationEvent("Synthetic medication", VALID_TIME)
        val firstId = medicationRepository.calls.single().id
        assertTrue(viewModel.medicationSaveState.value is MedicationSaveState.Failed)

        medicationRepository.failure = null
        viewModel.addMedicationEvent("Synthetic medication", VALID_TIME)

        assertEquals(firstId, medicationRepository.calls.last().id)
        assertTrue(viewModel.medicationSaveState.value is MedicationSaveState.Succeeded)
    }

    @Test
    fun changedFormAfterFailureGetsNewImmutableEventIdentity() = runTest {
        val medicationRepository = RecordingMedicationRepository().apply {
            failure = IllegalStateException("synthetic storage failure")
        }
        val viewModel = viewModel(medicationRepository)

        viewModel.addMedicationEvent("First label", VALID_TIME)
        val firstId = medicationRepository.calls.single().id

        medicationRepository.failure = null
        viewModel.addMedicationEvent("Changed label", VALID_TIME)

        assertNotEquals(firstId, medicationRepository.calls.last().id)
        assertTrue(viewModel.medicationSaveState.value is MedicationSaveState.Succeeded)
    }

    @Test
    fun startupInitializesLocalStorageIndependentlyOfPermissionProjection() = runTest {
        val dependencies = FakeDependencies(RecordingMedicationRepository())

        viewModel(dependencies)

        assertEquals(1, dependencies.initializeCalls)
        assertEquals(1, dependencies.health.refreshPermissionCalls)
    }

    private fun viewModel(medicationRepository: RecordingMedicationRepository): AppViewModel =
        viewModel(FakeDependencies(medicationRepository))

    private fun viewModel(dependencies: FakeDependencies): AppViewModel = AppViewModel(
        container = dependencies,
        clock = Clock.fixed(Instant.parse("2026-06-01T12:00:00Z"), ZoneOffset.UTC),
    )

    private class RecordingMedicationRepository : MedicationRepository {
        override val events = MutableStateFlow<List<MedicationEvent>>(emptyList())
        val calls = mutableListOf<MedicationEvent>()
        var gate: CompletableDeferred<Unit>? = null
        var failure: Throwable? = null

        override suspend fun append(event: MedicationEvent) {
            calls += event
            gate?.await()
            failure?.let { throw it }
            events.value = listOf(event) + events.value.filterNot { it.id == event.id }
        }
    }

    private class FakeDependencies(
        override val medicationRepository: MedicationRepository,
    ) : AppDependencies {
        override val settingsRepository = FakeSettingsRepository()
        override val fixtureSleepRepository: SleepRepository = EmptySleepRepository()
        override val fixtureEstimateRepository: EstimateRepository = EmptyEstimateRepository()
        val health = EmptyHealthConnectRepository()
        override val healthConnectRepository: HealthConnectRepository = health
        override val localDataState = MutableStateFlow<DurableLocalDataState>(DurableLocalDataState.Ready)

        // Sync is off in this fixture: these tests are about medication, and a
        // repository with no configured server is the local-only mode.
        override val backendSyncRepository = BackendSyncRepository(
            outbox = NoOutbox(),
            configStore = NoSyncConfig(),
            client = NoSyncClient(),
        )
        override val syncStatus = backendSyncRepository.status
        var initializeCalls = 0

        override suspend fun initializeLocalUserData() {
            initializeCalls += 1
        }
    }

    private class NoOutbox : SyncOutboxStore {
        override fun pending(limit: Int): List<OutboxRecord> = emptyList()

        override fun enqueue(records: List<OutboxRecord>) = Unit

        override fun markSynced(recordIds: List<String>, at: Instant) = Unit

        override fun syncedRevisions(): Map<String, Instant> = emptyMap()

        override fun pendingCount(): Int = 0

        override fun lastSyncedAt(): Instant? = null

        override fun clear() = Unit
    }

    private class NoSyncConfig : SyncConfigStore {
        override fun load(): SyncConfig? = null

        override fun save(config: SyncConfig) = Unit

        override fun clear() = Unit
    }

    private class NoSyncClient : BackendSyncClient {
        override suspend fun enroll(baseUrl: String, enrollmentSecret: String, label: String) =
            Result.failure<String>(IllegalStateException("Sync is not configured in this test."))

        override suspend fun push(baseUrl: String, token: String, records: List<OutboxRecord>) =
            Result.failure<List<String>>(IllegalStateException("Sync is not configured in this test."))
    }

    private class FakeSettingsRepository : SettingsRepository {
        override val settings = MutableStateFlow(AppSettings())

        override suspend fun update(transform: (AppSettings) -> AppSettings) {
            settings.value = transform(settings.value)
        }
    }

    private open class EmptySleepRepository : SleepRepository {
        override val sourceEpisodes = MutableStateFlow<List<SleepEpisode>>(emptyList())
        override val activeCorrections = MutableStateFlow<Map<String, SleepCorrection>>(emptyMap())
        override val correctionReviews = MutableStateFlow<List<SleepCorrectionReview>>(emptyList())

        override suspend fun refresh() = Unit

        override suspend fun appendCorrection(correction: SleepCorrection): Result<Unit> =
            Result.failure(UnsupportedOperationException("Not used by this test."))
    }

    private class EmptyHealthConnectRepository : EmptySleepRepository(), HealthConnectRepository {
        override val availability = MutableStateFlow(HealthConnectAvailability.UNAVAILABLE)
        override val permissionState = MutableStateFlow(HealthPermissionState.UNAVAILABLE)
        override val requiredPermissions: Set<String> = emptySet()
        override val lastRefreshError = MutableStateFlow<String?>(null)
        var refreshPermissionCalls = 0

        override suspend fun refreshPermissionState() {
            refreshPermissionCalls += 1
        }

        override suspend fun onPermissionResult(grantedPermissions: Set<String>) = Unit
    }

    private class EmptyEstimateRepository : EstimateRepository {
        override val estimate: StateFlow<EstimateSnapshot?> = MutableStateFlow(null)
    }

    private companion object {
        const val VALID_TIME = "2026-06-01 08:00"
    }
}

@OptIn(ExperimentalCoroutinesApi::class)
class MainDispatcherRule(
    private val dispatcher: TestDispatcher = UnconfinedTestDispatcher(),
) : TestWatcher() {
    override fun starting(description: Description) {
        Dispatchers.setMain(dispatcher)
    }

    override fun finished(description: Description) {
        Dispatchers.resetMain()
    }
}
