package org.non24.planner

import android.content.Context
import kotlinx.coroutines.flow.StateFlow
import org.non24.planner.data.AndroidHealthConnectClientAdapter
import org.non24.planner.data.DurableLocalDataState
import org.non24.planner.data.EstimateRepository
import org.non24.planner.data.FixtureSleepRepository
import org.non24.planner.data.HealthConnectSleepRepository
import org.non24.planner.data.HealthConnectRepository
import org.non24.planner.data.LocalMedicationRepository
import org.non24.planner.data.LocalUserDataRepository
import org.non24.planner.data.MedicationRepository
import org.non24.planner.data.SQLiteLocalUserDataStore
import org.non24.planner.data.SharedPreferencesSettingsRepository
import org.non24.planner.data.SleepRepository
import org.non24.planner.data.SettingsRepository
import org.non24.planner.data.fixtureEstimateRepository
import org.non24.planner.data.fixtureSleepEpisodes

interface AppDependencies {
    val settingsRepository: SettingsRepository
    val fixtureSleepRepository: SleepRepository
    val fixtureEstimateRepository: EstimateRepository
    val healthConnectRepository: HealthConnectRepository
    val medicationRepository: MedicationRepository
    val localDataState: StateFlow<DurableLocalDataState>

    suspend fun initializeLocalUserData()
}

class AppContainer(context: Context) : AppDependencies {
    private val applicationContext = context.applicationContext
    private val fixtureEpisodes = fixtureSleepEpisodes()
    private val localUserDataRepository = LocalUserDataRepository(
        SQLiteLocalUserDataStore(applicationContext),
        staticSleepEpisodes = fixtureEpisodes,
    )

    override val settingsRepository = SharedPreferencesSettingsRepository(applicationContext)
    override val fixtureSleepRepository: SleepRepository =
        FixtureSleepRepository(localUserDataRepository, fixtureEpisodes)
    override val fixtureEstimateRepository = fixtureEstimateRepository()
    override val healthConnectRepository: HealthConnectRepository = HealthConnectSleepRepository(
        AndroidHealthConnectClientAdapter(applicationContext),
        localUserDataRepository,
    )
    override val medicationRepository: MedicationRepository = LocalMedicationRepository(localUserDataRepository)
    override val localDataState: StateFlow<DurableLocalDataState> = localUserDataRepository.state

    override suspend fun initializeLocalUserData() {
        localUserDataRepository.initialize()
    }
}
