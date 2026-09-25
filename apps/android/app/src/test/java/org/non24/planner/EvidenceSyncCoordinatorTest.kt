package org.non24.planner

import java.time.Instant
import java.time.ZoneOffset
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.test.runTest
import org.junit.Assert.*
import org.junit.Test
import org.non24.planner.data.*
import org.non24.planner.domain.*

class EvidenceSyncCoordinatorTest {
    private class Settings : SettingsRepository {
        override val settings = MutableStateFlow(AppSettings(DataMode.HEALTH_CONNECT, backgroundSyncEnabled = true))
        override suspend fun update(transform: (AppSettings) -> AppSettings) { settings.value = transform(settings.value) }
    }

    private class Scheduler : BackgroundSyncScheduler {
        var scheduled = false
        var uploads = 0
        override fun reconcile(enabled: Boolean) { scheduled = enabled }
        override fun requestUpload() { uploads++ }
        override fun cancelAll() { scheduled = false }
    }

    private class Adapter : HealthConnectClientAdapter {
        var permissions = setOf(HealthConnectPermissions.READ_SLEEP, HealthConnectPermissions.READ_BACKGROUND)
        var supported = true
        var reads = 0
        var failRead = false
        override fun availability() = HealthConnectAvailability.AVAILABLE
        override fun backgroundReadAvailable() = supported
        override suspend fun grantedPermissions() = permissions
        override suspend fun readRecentSleep(): List<SleepEpisode> {
            reads++
            if (failRead) error("Synthetic provider failure")
            return listOf(SleepEpisode(
                id = "synthetic-revision", logicalSourceId = "synthetic-provider|sleep-1",
                start = Instant.parse("2026-09-01T02:00:00Z"), end = Instant.parse("2026-09-01T10:00:00Z"),
                ianaTimeZoneId = null, startZoneOffset = ZoneOffset.UTC, endZoneOffset = ZoneOffset.UTC,
                provenance = Provenance(AcquisitionMethod.HEALTH_CONNECT, EvidenceStatus.IMPORTED, "synthetic-provider", "sleep-1", Instant.parse("2026-09-01T11:00:00Z")),
            ))
        }
    }

    private class Fixture {
        val settings = Settings()
        val adapter = Adapter()
        val scheduler = Scheduler()
        val local = LocalUserDataRepository(InMemoryLocalUserDataStore())
        val health = HealthConnectSleepRepository(adapter, local)
        val config = FakeConfigStore(SyncConfig("https://synthetic.test", "synthetic-token", "UTC", "synthetic-scope"))
        val outbox = FakeOutbox()
        val sync = BackendSyncRepository(outbox, config, FakeClient(), replica = FakeReplica())
        val coordinator = EvidenceSyncCoordinator(settings, health, sync, scheduler) {
            local.initialize()
            sync.initialise()
        }
    }

    @Test
    fun `background collection requires the setting enrollment real mode and background permission`() = runTest {
        for (gate in 0..4) {
            val f = Fixture()
            when (gate) {
                0 -> f.settings.update { it.copy(backgroundSyncEnabled = false) }
                1 -> f.config.clear()
                2 -> f.settings.update { it.copy(dataMode = DataMode.FIXTURE) }
                3 -> f.adapter.permissions = setOf(HealthConnectPermissions.READ_SLEEP)
                4 -> f.adapter.supported = false
            }
            assertTrue(f.coordinator.refreshBackground())
            assertEquals(0, f.adapter.reads)
            assertEquals(0, f.outbox.pendingCount())
        }
    }

    @Test
    fun `permitted background refresh durably queues sleep and requests constrained upload`() = runTest {
        val f = Fixture()
        assertTrue(f.coordinator.refreshBackground())
        assertEquals(1, f.adapter.reads)
        assertEquals(1, f.outbox.pendingCount())
        assertTrue(f.scheduler.uploads > 0)
        // Restarting the repository reuses the same immutable queue, not a new request id.
        f.coordinator.refreshBackground()
        assertEquals(1, f.outbox.pendingCount())
    }

    @Test
    fun `permission loss stops new reads but does not erase authorized pending uploads`() = runTest {
        val f = Fixture()
        f.coordinator.refreshBackground()
        f.adapter.permissions = emptySet()
        assertTrue(f.coordinator.refreshBackground())
        assertEquals(1, f.adapter.reads)
        assertEquals(1, f.outbox.pendingCount())
        assertEquals(HealthPermissionState.REQUIRED, f.health.permissionState.value)
    }

    @Test
    fun `foreground import works without the optional background grant`() = runTest {
        val f = Fixture()
        f.adapter.permissions = setOf(HealthConnectPermissions.READ_SLEEP)
        f.settings.update { it.copy(backgroundSyncEnabled = false) }
        f.coordinator.refreshForeground()
        assertEquals(1, f.adapter.reads)
        assertEquals(1, f.outbox.pendingCount())
        assertFalse(f.scheduler.scheduled)
    }

    @Test
    fun `transient provider failure retains saved evidence and asks for bounded retry`() = runTest {
        val f = Fixture()
        f.coordinator.refreshBackground()
        val saved = f.health.sourceEpisodes.value
        f.adapter.failRead = true
        assertFalse(f.coordinator.refreshBackground())
        assertEquals(saved, f.health.sourceEpisodes.value)
        assertEquals(1, f.outbox.pendingCount())
    }

    @Test
    fun `sample mode cancels automatic import without manufacturing a real estimate`() = runTest {
        val f = Fixture()
        f.coordinator.reconcileSchedule()
        assertTrue(f.scheduler.scheduled)
        f.settings.update { it.copy(dataMode = DataMode.FIXTURE) }
        f.coordinator.reconcileSchedule()
        f.coordinator.refreshForeground()
        assertFalse(f.scheduler.scheduled)
        assertEquals(0, f.adapter.reads)
        assertTrue(f.coordinator.uploadNow().isFailure)
    }

    @Test
    fun `retry budget excludes permanent refusals and terminates repeated transient failures`() {
        assertFalse(shouldRetrySync(BackendSyncException(401, "synthetic"), 0))
        assertFalse(shouldRetrySync(BackendSyncException(409, "synthetic"), 0))
        assertTrue(shouldRetrySync(BackendSyncException(503, "synthetic"), 0))
        assertTrue(shouldRetrySync(BackendSyncException(429, "synthetic"), 3))
        assertFalse(shouldRetrySync(null, 4))
        assertFalse(shouldRetrySync(BackendSyncException(503, "synthetic"), 100))
    }
}
