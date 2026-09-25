package org.non24.planner

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.After
import org.junit.Assert.*
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.non24.planner.data.SQLiteLocalUserDataStore

@RunWith(AndroidJUnit4::class)
class SQLiteSchemaTest {
    private val context: Context = ApplicationProvider.getApplicationContext()
    private val databaseName = "zeitboard-current-schema-test.db"
    @Before fun setup() { context.deleteDatabase(databaseName) }
    @After fun cleanup() { context.deleteDatabase(databaseName) }

    @Test fun freshDatabaseContainsTheWholeCurrentSchema() {
        SQLiteLocalUserDataStore(context, databaseName).use { store ->
            val db = store.writableDatabase
            val tables = db.rawQuery("SELECT name FROM sqlite_master WHERE type = 'table'", null).use {
                buildSet { while (it.moveToNext()) add(it.getString(0)) }
            }
            assertTrue(tables.containsAll(setOf("health_sleep_episodes", "sleep_corrections", "medication_events",
                "sync_outbox", "sync_replica", "sync_replica_meta", "sync_erased_records", "sync_erased_tasks", "erased_health_sources")))
        }
    }

    @Test fun obsoleteDevelopmentDatabaseIsRefusedWithoutSilentErasure() {
        context.openOrCreateDatabase(databaseName, Context.MODE_PRIVATE, null).use {
            it.execSQL("CREATE TABLE synthetic_marker(value TEXT)")
            it.execSQL("INSERT INTO synthetic_marker VALUES ('preserve')")
            it.version = 1
        }
        SQLiteLocalUserDataStore(context, databaseName).use {
            assertTrue(runCatching { it.writableDatabase }.isFailure)
        }
        context.openOrCreateDatabase(databaseName, Context.MODE_PRIVATE, null).use { db ->
            db.rawQuery("SELECT value FROM synthetic_marker", null).use {
                assertTrue(it.moveToFirst()); assertEquals("preserve", it.getString(0))
            }
        }
    }
}
