package org.non24.planner

import java.time.Instant
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.non24.planner.data.*
import org.non24.planner.domain.AppSettings

// ADR-0074: the owner's sharing choice and clock travel between devices
// through their server, and the newest choice made anywhere stands.
class SharedChoicesTest {
    private val at = Instant.parse("2026-11-17T12:00:00Z")

    private class Settings(initial: AppSettings) : SettingsRepository {
        private val state = MutableStateFlow(initial)
        override val settings: StateFlow<AppSettings> = state
        override suspend fun update(transform: (AppSettings) -> AppSettings) { state.value = transform(state.value) }
    }

    private fun choice(name: String, value: JsonPrimitive, changedAt: Instant, id: String = "setting-remote-${changedAt.epochSecond}") =
        SharedChoice(id, name, value, changedAt)

    private fun repository(settings: Settings, outbox: FakeOutbox, standing: Map<String, SharedChoice>) = BackendSyncRepository(
        outbox, FakeConfigStore(SyncConfig("https://synthetic.test", "token", "UTC", "synthetic-scope")), FakeClient(),
        replica = object : SyncReplicaStore by FakeReplica() {
            override fun standingChoices() = standing
        },
        choices = SettingsChoiceKeeper(settings),
    )

    @Test
    fun `a choice made here is sent as the contract states it, and read back`() {
        val record = SharedChoices.record(SharedChoices.CLOCK, JsonPrimitive("24"), at.plusNanos(1_500_000))
        val payload = Json.parseToJsonElement(record.payload).jsonObject
        assertEquals(SharedChoices.KIND, record.kind)
        assertEquals("2026-11-17T12:00:00.001Z", payload.getValue("changed_at").let { (it as JsonPrimitive).content })
        assertEquals(record.recordId, SharedChoices.record(SharedChoices.CLOCK, JsonPrimitive("24"), at.plusNanos(1_500_000)).recordId)
        assertTrue(SyncContract.isValidIdentifier(record.recordId))
        validatePulledPayload(record.recordId, record.kind, payload)

        // The server's refusals are the phone's.
        listOf(
            JsonObject(payload + ("value" to JsonPrimitive(true))),
            JsonObject(payload + ("name" to JsonPrimitive("theme"))),
            JsonObject(payload + ("value" to JsonPrimitive("36"))),
            JsonObject(payload - "changed_at"),
        ).forEach { tampered ->
            assertTrue(runCatching { validatePulledPayload(record.recordId, record.kind, tampered) }.isFailure)
        }
    }

    @Test
    fun `a newer choice made elsewhere is taken, and an older one is not`() = runTest {
        val settings = Settings(AppSettings(shareReportedStatus = false, clock = "system",
            choiceTimes = mapOf(SharedChoices.CLOCK to at)))
        val outbox = FakeOutbox()
        val repository = repository(settings, outbox, mapOf(
            SharedChoices.SHARE_STATUS to choice(SharedChoices.SHARE_STATUS, JsonPrimitive(true), at.minusSeconds(3600)),
            SharedChoices.CLOCK to choice(SharedChoices.CLOCK, JsonPrimitive("12"), at.minusSeconds(60)),
        ))
        assertEquals(1, repository.shareChoices())
        // Never chosen here, so the computer's choice is taken, even an old one.
        assertTrue(settings.settings.value.shareReportedStatus)
        assertEquals(at.minusSeconds(3600), settings.settings.value.choiceTimes[SharedChoices.SHARE_STATUS])
        // Chosen here after the computer's, so this phone's stands and is sent.
        assertEquals("system", settings.settings.value.clock)
        assertEquals(1, outbox.pendingCount())
    }

    @Test
    fun `a choice made here later than any standing is sent once`() = runTest {
        val settings = Settings(AppSettings(clock = "24", choiceTimes = mapOf(SharedChoices.CLOCK to at)))
        val outbox = FakeOutbox()
        val repository = repository(settings, outbox, mapOf(
            SharedChoices.CLOCK to choice(SharedChoices.CLOCK, JsonPrimitive("12"), at.minusSeconds(60)),
        ))
        assertEquals(1, repository.shareChoices())
        val sent = Json.parseToJsonElement(outbox.pending(10).single().payload).jsonObject
        assertEquals("24", (sent.getValue("value") as JsonPrimitive).content)
        assertEquals("24", settings.settings.value.clock)
        // Once queued, it is the newest: nothing more is sent.
        assertEquals(1, outbox.pendingCount())
    }

    @Test
    fun `a choice made now is dated after the one standing`() {
        val standing = choice(SharedChoices.CLOCK, JsonPrimitive("12"), at.plusNanos(500_000))
        assertEquals(at.plusMillis(1), SharedChoices.nextChangeAt(at.minusSeconds(3600), standing))
        assertEquals(at.plusSeconds(60), SharedChoices.nextChangeAt(at.plusSeconds(60), standing))
        assertFalse(SharedChoices.nextChangeAt(at, null).isAfter(at))
    }
}
