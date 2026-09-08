package org.non24.planner.data

import android.content.SharedPreferences

/** SharedPreferences changes memory before commit returns; restore it if the disk write fails. */
internal fun SharedPreferences.commitDurably(change: SharedPreferences.Editor.() -> Unit): Boolean {
    val previous = all
    if (edit().apply(change).commit()) return true
    val rollback = edit().clear()
    previous.forEach { (key, value) ->
        when (value) {
            is String -> rollback.putString(key, value)
            is Boolean -> rollback.putBoolean(key, value)
            is Int -> rollback.putInt(key, value)
            is Long -> rollback.putLong(key, value)
            is Float -> rollback.putFloat(key, value)
            is Set<*> -> rollback.putStringSet(key, value.filterIsInstance<String>().toSet())
        }
    }
    rollback.commit()
    return false
}
