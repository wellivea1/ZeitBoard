package org.non24.planner

import java.time.Clock
import java.time.Duration
import java.time.Instant
import java.time.ZoneId
import java.time.ZoneOffset
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.serialization.json.jsonObject
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
        override suspend fun readSleep(window: ReadWindow): List<SleepEpisode> {
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

    /** A clock the test moves by hand. */
    private class StepClock(var now: Instant) : Clock() {
        override fun getZone(): ZoneId = ZoneOffset.UTC
        override fun withZone(zone: ZoneId?): Clock = this
        override fun instant(): Instant = now
    }

    private class Fixture {
        val settings = Settings()
        val adapter = Adapter()
        val scheduler = Scheduler()
        val clock = StepClock(Instant.parse("2026-09-01T12:00:00Z"))
        val local = LocalUserDataRepository(InMemoryLocalUserDataStore())
        val health = HealthConnectSleepRepository(adapter, local, clock)
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

    private fun Fixture.queued(kind: String) = outbox.pending(100).filter { it.kind == kind }

    private fun Fixture.reports() = queued(SourceReporting.KIND).map {
        kotlinx.serialization.json.Json.parseToJsonElement(it.payload).jsonObject
    }

    @Test
    fun `permitted background refresh durably queues sleep and requests constrained upload`() = runTest {
        val f = Fixture()
        assertTrue(f.coordinator.refreshBackground())
        assertEquals(1, f.adapter.reads)
        assertEquals(1, f.queued("observation").size)
        assertTrue(f.scheduler.uploads > 0)
        // Restarting the repository reuses the same immutable queue, not a new request id.
        f.coordinator.refreshBackground()
        assertEquals(1, f.queued("observation").size)
    }

    @Test
    fun `permission loss stops new reads but does not erase authorized pending uploads`() = runTest {
        val f = Fixture()
        f.coordinator.refreshBackground()
        f.adapter.permissions = emptySet()
        assertTrue(f.coordinator.refreshBackground())
        assertEquals(1, f.adapter.reads)
        assertEquals(1, f.queued("observation").size)
        assertEquals(HealthPermissionState.REQUIRED, f.health.permissionState.value)
        // The computer is told the permission went (ADR-0054).
        assertEquals("permission_missing", f.reports().last().string("outcome"))
    }

    @Test
    fun `foreground import works without the optional background grant`() = runTest {
        val f = Fixture()
        f.adapter.permissions = setOf(HealthConnectPermissions.READ_SLEEP)
        f.settings.update { it.copy(backgroundSyncEnabled = false) }
        f.coordinator.refreshForeground()
        assertEquals(1, f.adapter.reads)
        assertEquals(1, f.queued("observation").size)
        assertFalse(f.scheduler.scheduled)
        // It reads only while open, and says so.
        assertEquals("false", f.reports().single().getValue("background_reads").toString())
    }

    @Test
    fun `each check tells the computer what it found without repeating itself`() = runTest {
        val f = Fixture()
        f.coordinator.refreshBackground()
        val first = f.reports().single()
        assertEquals("read", first.string("outcome"))
        assertEquals("2026-09-01T12:00:00Z", first.string("reported_at"))
        val covered = first.getValue("covered").jsonObject
        assertEquals("2026-08-02T12:00:00Z", covered.string("start_at"))
        assertEquals("2026-09-01T12:00:00Z", covered.string("end_at"))
        assertEquals("0", first.getValue("held").toString())

        // An hour later the phone reads again and finds the same: nothing new to say.
        f.clock.now = f.clock.now.plus(Duration.ofHours(1))
        f.coordinator.refreshBackground()
        assertEquals(1, f.reports().size)

        // Six hours after the last report it says it again, so a quiet phone shows.
        f.clock.now = f.clock.now.plus(Duration.ofHours(5))
        f.coordinator.refreshBackground()
        assertEquals(listOf("read", "read"), f.reports().map { it.string("outcome") })

        // A failed read is said at once, and so is the next good one.
        f.clock.now = f.clock.now.plus(Duration.ofMinutes(10))
        f.adapter.failRead = true
        f.coordinator.refreshBackground()
        f.clock.now = f.clock.now.plus(Duration.ofHours(1))
        f.adapter.failRead = false
        f.coordinator.refreshBackground()
        assertEquals(listOf("read", "read", "read_failed", "read"), f.reports().map { it.string("outcome") })
        // Only reads state a window.
        assertFalse(f.reports()[2].containsKey("covered"))
    }

    @Test
    fun `nothing is reported without enrollment or in sample mode`() = runTest {
        val unenrolled = Fixture()
        unenrolled.config.clear()
        unenrolled.coordinator.refreshForeground()
        assertTrue(unenrolled.reports().isEmpty())
        val sample = Fixture()
        sample.settings.update { it.copy(dataMode = DataMode.FIXTURE) }
        sample.coordinator.refreshForeground()
        assertTrue(sample.reports().isEmpty())
    }

    @Test
    fun `transient provider failure retains saved evidence and asks for bounded retry`() = runTest {
        val f = Fixture()
        f.coordinator.refreshBackground()
        val saved = f.health.sourceEpisodes.value
        f.adapter.failRead = true
        assertFalse(f.coordinator.refreshBackground())
        assertEquals(saved, f.health.sourceEpisodes.value)
        assertEquals(1, f.queued("observation").size)
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
