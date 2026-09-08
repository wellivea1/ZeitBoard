package org.non24.planner

import android.content.SharedPreferences
import org.junit.Assert.*
import org.junit.Test
import org.non24.planner.data.commitDurably

class DurablePreferencesTest {
    @Test
    fun `failed disk commit restores the previously authorized configuration in memory`() {
        val preferences = MemoryPreferences(mutableMapOf("token" to "synthetic-old", "scope" to "old-scope", "background" to false))
        preferences.failNextCommit = true
        assertFalse(preferences.commitDurably {
            putString("token", "synthetic-new")
            putString("scope", "new-scope")
            putBoolean("background", true)
        })
        assertEquals("synthetic-old", preferences.getString("token", null))
        assertEquals("old-scope", preferences.getString("scope", null))
        assertFalse(preferences.getBoolean("background", true))
        preferences.failNextCommit = true
        assertFalse(preferences.commitDurably { clear() })
        assertEquals("synthetic-old", preferences.getString("token", null))
    }

    private class MemoryPreferences(private val values: MutableMap<String, Any>) : SharedPreferences {
        var failNextCommit = false
        override fun getAll(): MutableMap<String, *> = values.toMutableMap()
        override fun getString(key: String?, defValue: String?) = values[key] as? String ?: defValue
        override fun getStringSet(key: String?, defValues: MutableSet<String>?) = defValues
        override fun getInt(key: String?, defValue: Int) = values[key] as? Int ?: defValue
        override fun getLong(key: String?, defValue: Long) = values[key] as? Long ?: defValue
        override fun getFloat(key: String?, defValue: Float) = values[key] as? Float ?: defValue
        override fun getBoolean(key: String?, defValue: Boolean) = values[key] as? Boolean ?: defValue
        override fun contains(key: String?) = values.containsKey(key)
        override fun registerOnSharedPreferenceChangeListener(listener: SharedPreferences.OnSharedPreferenceChangeListener?) = Unit
        override fun unregisterOnSharedPreferenceChangeListener(listener: SharedPreferences.OnSharedPreferenceChangeListener?) = Unit
        override fun edit(): SharedPreferences.Editor = object : SharedPreferences.Editor {
            val pending = values.toMutableMap()
            private fun put(key: String?, value: Any?): SharedPreferences.Editor {
                if (key != null) { if (value == null) pending.remove(key) else pending[key] = value }
                return this
            }
            override fun putString(key: String?, value: String?) = put(key, value)
            override fun putStringSet(key: String?, values: MutableSet<String>?) = put(key, values)
            override fun putInt(key: String?, value: Int) = put(key, value)
            override fun putLong(key: String?, value: Long) = put(key, value)
            override fun putFloat(key: String?, value: Float) = put(key, value)
            override fun putBoolean(key: String?, value: Boolean) = put(key, value)
            override fun remove(key: String?) = put(key, null)
            override fun clear(): SharedPreferences.Editor { pending.clear(); return this }
            override fun commit(): Boolean {
                values.clear()
                values.putAll(pending)
                val success = !failNextCommit
                failNextCommit = false
                return success
            }
            override fun apply() { commit() }
        }
    }
}
