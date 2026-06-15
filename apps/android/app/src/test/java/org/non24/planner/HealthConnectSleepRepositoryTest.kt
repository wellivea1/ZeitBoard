package org.non24.planner

import java.time.Instant
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.non24.planner.data.HealthConnectClientAdapter
import org.non24.planner.data.HealthConnectPermissions
import org.non24.planner.data.HealthConnectSleepRepository
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

    private class FakeHealthConnectClientAdapter(
        private val availability: HealthConnectAvailability = HealthConnectAvailability.AVAILABLE,
    ) : HealthConnectClientAdapter {
        var granted: Set<String> = emptySet()
        var readCount: Int = 0

        override fun availability(): HealthConnectAvailability = availability

        override suspend fun grantedPermissions(): Set<String> = granted

        override suspend fun readRecentSleep(): List<SleepEpisode> {
            readCount += 1
            return listOf(
                SleepEpisode(
                    id = "health-sleep-1",
                    start = Instant.parse("2026-06-14T03:55:00Z"),
                    end = Instant.parse("2026-06-14T12:05:00Z"),
                    timeZoneId = "America/New_York",
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
