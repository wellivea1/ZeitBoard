package org.non24.planner.data

import android.content.Context
import android.text.format.DateFormat
import java.time.Instant
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
    private val context: Context,
) : SettingsRepository {
    private val preferences = context.getSharedPreferences("non24_settings", Context.MODE_PRIVATE)
    private val mutableSettings = MutableStateFlow(load())
    private val mutex = Mutex()

    override val settings: StateFlow<AppSettings> = mutableSettings.asStateFlow()

    override suspend fun update(transform: (AppSettings) -> AppSettings) = withContext(Dispatchers.IO) {
        mutex.withLock {
            val transformed = transform(mutableSettings.value)
            require(transformed.clock in SharedChoices.CLOCKS) { "The clock is system, 12 or 24." }
            val updated = transformed.copy(use24HourTime = twentyFourHour(transformed.clock))
            check(preferences.commitDurably {
                putString(DATA_MODE_KEY, updated.dataMode.name)
                putString(CLOCK_KEY, updated.clock)
                putBoolean(BACKGROUND_SYNC_KEY, updated.backgroundSyncEnabled)
                putBoolean(TIME_REQUEST_NOTICES_KEY, updated.timeRequestNotices)
                putBoolean(SHARE_STATUS_KEY, updated.shareReportedStatus)
                updated.choiceTimes.forEach { (name, at) -> putString(CHOICE_TIME_PREFIX + name, at.toString()) }
            }) { "Settings could not be saved on this device." }
            mutableSettings.value = updated
        }
    }

    private fun load(): AppSettings {
        val mode = runCatching {
            DataMode.valueOf(preferences.getString(DATA_MODE_KEY, DataMode.FIXTURE.name)!!)
        }.getOrDefault(DataMode.FIXTURE)
        val clock = preferences.getString(CLOCK_KEY, SharedChoices.CLOCK_SYSTEM)?.takeIf { it in SharedChoices.CLOCKS }
            ?: SharedChoices.CLOCK_SYSTEM
        val times = listOf(SharedChoices.SHARE_STATUS, SharedChoices.CLOCK).mapNotNull { name ->
            preferences.getString(CHOICE_TIME_PREFIX + name, null)
                ?.let { runCatching { Instant.parse(it) }.getOrNull() }
                ?.let { name to it }
        }.toMap()
        return AppSettings(
            dataMode = mode,
            use24HourTime = twentyFourHour(clock),
            clock = clock,
            choiceTimes = times,
            backgroundSyncEnabled = preferences.getBoolean(BACKGROUND_SYNC_KEY, false),
            timeRequestNotices = preferences.getBoolean(TIME_REQUEST_NOTICES_KEY, false),
            shareReportedStatus = preferences.getBoolean(SHARE_STATUS_KEY, false),
        )
    }

    /** The clock in use: the one chosen, or this phone's own format. */
    private fun twentyFourHour(clock: String): Boolean = when (clock) {
        SharedChoices.CLOCK_24 -> true
        SharedChoices.CLOCK_12 -> false
        else -> DateFormat.is24HourFormat(context)
    }

    private companion object {
        const val DATA_MODE_KEY = "data_mode"
        const val CLOCK_KEY = "clock"
        const val CHOICE_TIME_PREFIX = "choice_time:"
        const val SHARE_STATUS_KEY = "share_reported_status"
        const val BACKGROUND_SYNC_KEY = "background_sync"
        const val TIME_REQUEST_NOTICES_KEY = "time_request_notices"
    }
}
