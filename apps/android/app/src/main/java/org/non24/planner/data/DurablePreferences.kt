package org.non24.planner.data

import android.annotation.SuppressLint
import android.content.SharedPreferences
import androidx.core.content.edit

/**
 * SharedPreferences changes memory before commit returns; restore it if the disk write fails.
 * The change is not made with KTX's edit, which drops the result of commit that decides this.
 */
@SuppressLint("UseKtx")
internal fun SharedPreferences.commitDurably(change: SharedPreferences.Editor.() -> Unit): Boolean {
    val previous = all
    if (edit().apply(change).commit()) return true
    // Put back what was there, written before this returns as the change was to be.
    edit(commit = true) {
        clear()
        previous.forEach { (key, value) ->
            when (value) {
                is String -> putString(key, value)
                is Boolean -> putBoolean(key, value)
                is Int -> putInt(key, value)
                is Long -> putLong(key, value)
                is Float -> putFloat(key, value)
                is Set<*> -> putStringSet(key, value.filterIsInstance<String>().toSet())
            }
        }
    }
    return false
}
