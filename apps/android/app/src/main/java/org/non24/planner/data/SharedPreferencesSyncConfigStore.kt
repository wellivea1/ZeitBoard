package org.non24.planner.data

import android.content.Context
import android.content.SharedPreferences

/**
 * Persists the enrollment token.
 *
 * It lives in a private preferences file rather than in the app database,
 * because a database export or backup is a thing the user may reasonably share
 * with a clinician, and a bearer token is not health data they intend to hand
 * over. Keeping the two apart means an export can never carry it.
 *
 * This is not a hardware-backed keystore. On a device without full-disk
 * encryption, or one that is rooted, the token is readable — the same residual
 * the desktop records for its own token file. Moving to `EncryptedSharedPrefs`
 * is a worthwhile follow-up and is not a substitute for the server's ability to
 * revoke a device.
 */
class SharedPreferencesSyncConfigStore(
    context: Context,
    fileName: String = PREFERENCES_FILE,
) : SyncConfigStore {

    private val preferences: SharedPreferences =
        context.applicationContext.getSharedPreferences(fileName, Context.MODE_PRIVATE)

    override fun load(): SyncConfig? {
        val baseUrl = preferences.getString(KEY_BASE_URL, null) ?: return null
        val token = preferences.getString(KEY_TOKEN, null) ?: return null
        val zone = preferences.getString(KEY_HOME_ZONE, null) ?: return null
        if (baseUrl.isBlank() || token.isBlank() || zone.isBlank()) return null
        return SyncConfig(baseUrl = baseUrl, token = token, homeZoneId = zone)
    }

    override fun save(config: SyncConfig) {
        preferences.edit()
            .putString(KEY_BASE_URL, config.baseUrl)
            .putString(KEY_TOKEN, config.token)
            .putString(KEY_HOME_ZONE, config.homeZoneId)
            .apply()
    }

    override fun clear() {
        preferences.edit().clear().apply()
    }

    companion object {
        const val PREFERENCES_FILE = "zeitboard_sync"
        private const val KEY_BASE_URL = "base_url"
        private const val KEY_TOKEN = "token"
        private const val KEY_HOME_ZONE = "home_zone"
    }
}
