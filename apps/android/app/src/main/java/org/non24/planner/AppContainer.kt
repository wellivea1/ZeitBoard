package org.non24.planner

import android.content.Context
import org.non24.planner.data.AndroidHealthConnectClientAdapter
import org.non24.planner.data.FixtureSleepRepository
import org.non24.planner.data.HealthConnectSleepRepository
import org.non24.planner.data.InMemoryMedicationRepository
import org.non24.planner.data.SharedPreferencesSettingsRepository
import org.non24.planner.data.fixtureEstimateRepository

class AppContainer(context: Context) {
    val settingsRepository = SharedPreferencesSettingsRepository(context.applicationContext)
    val fixtureSleepRepository = FixtureSleepRepository()
    val fixtureEstimateRepository = fixtureEstimateRepository()
    val healthConnectRepository = HealthConnectSleepRepository(
        AndroidHealthConnectClientAdapter(context.applicationContext),
    )
    val medicationRepository = InMemoryMedicationRepository()
}
