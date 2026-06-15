package org.non24.planner.data

import android.content.Context
import androidx.core.content.edit
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import org.non24.planner.domain.AppSettings
import org.non24.planner.domain.DataMode

class SharedPreferencesSettingsRepository(
    context: Context,
) : SettingsRepository {
    private val preferences = context.getSharedPreferences("non24_settings", Context.MODE_PRIVATE)
    private val mutableSettings = MutableStateFlow(load())

    override val settings: StateFlow<AppSettings> = mutableSettings.asStateFlow()

    override suspend fun update(transform: (AppSettings) -> AppSettings) {
        val updated = transform(mutableSettings.value)
        preferences.edit {
            putString(DATA_MODE_KEY, updated.dataMode.name)
            putBoolean(USE_24_HOUR_KEY, updated.use24HourTime)
        }
        mutableSettings.value = updated
    }

    private fun load(): AppSettings {
        val mode = runCatching {
            DataMode.valueOf(preferences.getString(DATA_MODE_KEY, DataMode.FIXTURE.name)!!)
        }.getOrDefault(DataMode.FIXTURE)
        return AppSettings(
            dataMode = mode,
            use24HourTime = preferences.getBoolean(USE_24_HOUR_KEY, true),
        )
    }

    private companion object {
        const val DATA_MODE_KEY = "data_mode"
        const val USE_24_HOUR_KEY = "use_24_hour_time"
    }
}
