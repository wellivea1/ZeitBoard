package org.non24.planner

import java.time.Instant
import java.time.ZoneOffset
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.non24.planner.data.HealthConnectClientAdapter
import org.non24.planner.data.HealthConnectPermissions
import org.non24.planner.data.HealthConnectSleepRepository
import org.non24.planner.data.InMemoryLocalUserDataStore
import org.non24.planner.data.LocalUserDataRepository
import org.non24.planner.domain.AcquisitionMethod
import org.non24.planner.domain.EvidenceStatus
import org.non24.planner.domain.HealthConnectAvailability
import org.non24.planner.domain.HealthPermissionState
import org.non24.planner.domain.Provenance
import org.non24.planner.domain.SleepEpisode

class HealthConnectSleepRepositoryTest {
    @Test
    fun permissionProjectionContainsOnlyReadSleep() {
        assertEquals(setOf("android.permission.health.READ_SLEEP"), HealthConnectPermissions.required)
        assertTrue(HealthConnectPermissions.required.none { it.contains("WRITE") })
    }

    @Test
    fun refreshReadsOnlyAfterPermissionIsGranted() = runTest {
        val adapter = FakeHealthConnectClientAdapter()
        val repository = HealthConnectSleepRepository(adapter)

        repository.refresh()
        assertEquals(HealthPermissionState.REQUIRED, repository.permissionState.value)
        assertEquals(0, adapter.readCount)

        adapter.granted = HealthConnectPermissions.required
        repository.onPermissionResult(HealthConnectPermissions.required)

        assertEquals(HealthPermissionState.GRANTED, repository.permissionState.value)
        assertEquals(1, adapter.readCount)
        assertEquals("health-sleep-1", repository.sourceEpisodes.value.single().id)
    }

    @Test
    fun unavailableProviderDoesNotAttemptRead() = runTest {
        val adapter = FakeHealthConnectClientAdapter(
            availability = HealthConnectAvailability.UPDATE_REQUIRED,
        )
        val repository = HealthConnectSleepRepository(adapter)

        repository.refresh()

        assertEquals(HealthPermissionState.UNAVAILABLE, repository.permissionState.value)
        assertEquals(0, adapter.readCount)
    }

    @Test
    fun failedRefreshRetainsLastGoodSnapshot() = runTest {
        val adapter = FakeHealthConnectClientAdapter().apply {
            granted = HealthConnectPermissions.required
        }
        val repository = HealthConnectSleepRepository(adapter)
        repository.refresh()
        val imported = repository.sourceEpisodes.value

        adapter.readFailure = IllegalStateException("provider failed")
        repository.refresh()

        assertEquals(imported, repository.sourceEpisodes.value)
        assertNotNull(repository.lastRefreshError.value)
    }

    @Test
    fun successfulEmptyRefreshReplacesTheSnapshot() = runTest {
        val adapter = FakeHealthConnectClientAdapter().apply {
            granted = HealthConnectPermissions.required
        }
        val repository = HealthConnectSleepRepository(adapter)
        repository.refresh()
        assertTrue(repository.sourceEpisodes.value.isNotEmpty())

        adapter.returnEmptySnapshot = true
        repository.refresh()

        assertTrue(repository.sourceEpisodes.value.isEmpty())
        assertNull(repository.lastRefreshError.value)
    }

    @Test
    fun newRepositoryHydratesSavedSnapshotWhenProviderIsUnavailable() = runTest {
        val store = InMemoryLocalUserDataStore()
        val firstProjection = LocalUserDataRepository(store)
        val availableAdapter = FakeHealthConnectClientAdapter().apply {
            granted = HealthConnectPermissions.required
        }
        val firstRepository = HealthConnectSleepRepository(availableAdapter, firstProjection)
        firstRepository.refresh()

        val restoredProjection = LocalUserDataRepository(store)
        restoredProjection.initialize()
        val restoredRepository = HealthConnectSleepRepository(
            FakeHealthConnectClientAdapter(HealthConnectAvailability.UPDATE_REQUIRED),
            restoredProjection,
        )
        restoredRepository.refreshPermissionState()

        assertEquals(
            firstRepository.sourceEpisodes.value,
            restoredRepository.sourceEpisodes.value,
        )
        assertEquals(HealthPermissionState.UNAVAILABLE, restoredRepository.permissionState.value)
    }

    private class FakeHealthConnectClientAdapter(
        private val availability: HealthConnectAvailability = HealthConnectAvailability.AVAILABLE,
    ) : HealthConnectClientAdapter {
        var granted: Set<String> = emptySet()
        var readCount: Int = 0
        var returnEmptySnapshot: Boolean = false
        var readFailure: Throwable? = null

        override fun availability(): HealthConnectAvailability = availability

        override suspend fun grantedPermissions(): Set<String> = granted

        override suspend fun readRecentSleep(): List<SleepEpisode> {
            readCount += 1
            readFailure?.let { throw it }
            if (returnEmptySnapshot) return emptyList()
            return listOf(
                SleepEpisode(
                    id = "health-sleep-1",
                    logicalSourceId = "health-source-1",
                    start = Instant.parse("2026-06-14T03:55:00Z"),
                    end = Instant.parse("2026-06-14T12:05:00Z"),
                    ianaTimeZoneId = null,
                    startZoneOffset = ZoneOffset.ofHours(-4),
                    endZoneOffset = ZoneOffset.ofHours(-4),
                    provenance = Provenance(
                        acquisitionMethod = AcquisitionMethod.HEALTH_CONNECT,
                        evidenceStatus = EvidenceStatus.IMPORTED,
                        sourceId = "synthetic.test.source",
                    ),
                ),
            )
        }
    }
}
