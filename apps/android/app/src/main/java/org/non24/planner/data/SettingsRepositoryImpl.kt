package org.non24.planner.data

import android.content.Context
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import org.non24.planner.domain.AppSettings
import org.non24.planner.domain.DataMode

class SharedPreferencesSettingsRepository(
    context: Context,
) : SettingsRepository {
    private val preferences = context.getSharedPreferences("non24_settings", Context.MODE_PRIVATE)
    private val mutableSettings = MutableStateFlow(load())
    private val mutex = Mutex()

    override val settings: StateFlow<AppSettings> = mutableSettings.asStateFlow()

    override suspend fun update(transform: (AppSettings) -> AppSettings) = withContext(Dispatchers.IO) {
        mutex.withLock {
            val updated = transform(mutableSettings.value)
            check(preferences.commitDurably {
                putString(DATA_MODE_KEY, updated.dataMode.name)
                putBoolean(USE_24_HOUR_KEY, updated.use24HourTime)
                putBoolean(BACKGROUND_SYNC_KEY, updated.backgroundSyncEnabled)
            }) { "Settings could not be saved on this device." }
            mutableSettings.value = updated
        }
    }

    private fun load(): AppSettings {
        val mode = runCatching {
            DataMode.valueOf(preferences.getString(DATA_MODE_KEY, DataMode.FIXTURE.name)!!)
        }.getOrDefault(DataMode.FIXTURE)
        return AppSettings(
            dataMode = mode,
            use24HourTime = preferences.getBoolean(USE_24_HOUR_KEY, true),
            backgroundSyncEnabled = preferences.getBoolean(BACKGROUND_SYNC_KEY, false),
        )
    }

    private companion object {
        const val DATA_MODE_KEY = "data_mode"
        const val USE_24_HOUR_KEY = "use_24_hour_time"
        const val BACKGROUND_SYNC_KEY = "background_sync"
    }
}
