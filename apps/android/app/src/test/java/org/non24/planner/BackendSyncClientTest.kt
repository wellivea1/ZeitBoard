package org.non24.planner

import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.net.HttpURLConnection
import java.net.URL
import java.time.Instant
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.*
import org.junit.Test
import org.non24.planner.data.HttpBackendSyncClient
import org.non24.planner.data.MAX_SYNC_RESPONSE_BYTES
import org.non24.planner.data.OutboxRecord
import org.non24.planner.data.normalizeSyncUrl

class BackendSyncClientTest {
    private class Connection(val response: String, private val status: Int = 200) : HttpURLConnection(URL("https://synthetic.test")) {
        val body = ByteArrayOutputStream()
        var closed = false
        override fun connect() = Unit
        override fun disconnect() { closed = true }
        override fun usingProxy() = false
        override fun getOutputStream() = body
        override fun getInputStream() = ByteArrayInputStream(response.toByteArray(Charsets.UTF_8))
        override fun getResponseCode() = status
    }

    private val record = OutboxRecord("hc-synthetic", "observation", Instant.EPOCH, Instant.EPOCH, "{}")

    @Test
    fun `enrollment uses server field names and escapes input without persisting it`() = runTest {
        val connection = Connection("""{"schema_version":"v1","token":"synthetic-token"}""")
        val client = HttpBackendSyncClient(openConnection = { connection })
        val secret = "synthetic\nsecret\"\\"
        assertEquals("synthetic-token", client.enroll("https://synthetic.test", secret, "phone").getOrNull())
        val request = Json.parseToJsonElement(connection.body.toString("UTF-8")).jsonObject
        assertEquals(secret, request.getValue("enrollmentSecret").jsonPrimitive.content)
        assertEquals("phone", request.getValue("label").jsonPrimitive.content)
        assertFalse(connection.instanceFollowRedirects)
        assertTrue(connection.closed)
    }

    @Test
    fun `duplicate acknowledgment with zero new rows still acknowledges the full atomic batch`() = runTest {
        val connection = Connection("""{"schema_version":"v1","cursor":5,"accepted":0}""")
        val client = HttpBackendSyncClient(openConnection = { connection })
        assertEquals(listOf(record.recordId), client.push("https://synthetic.test", "synthetic-token", listOf(record)).getOrNull())
        assertEquals("Bearer synthetic-token", connection.getRequestProperty("Authorization"))
    }

    @Test
    fun `HTML empty malformed and incompatible responses never acknowledge an upload`() = runTest {
        for (body in listOf("", "<html>proxy login</html>", "{}",
            """{"schema_version":"unsupported","cursor":1,"accepted":1}""",
            """{"schema_version":"v1","cursor":-1,"accepted":1}""",
            """{"schema_version":"v1","cursor":1,"accepted":2}""")) {
            val client = HttpBackendSyncClient(openConnection = { Connection(body) })
            assertTrue(client.push("https://synthetic.test", "synthetic-token", listOf(record)).isFailure)
        }
    }

    @Test
    fun `redirects cannot carry enrollment or bearer credentials to another destination`() = runTest {
        val connection = Connection("", 302)
        val client = HttpBackendSyncClient(openConnection = { connection })
        assertTrue(client.push("https://synthetic.test", "synthetic-token", listOf(record)).isFailure)
        assertFalse(connection.instanceFollowRedirects)
        assertTrue(connection.closed)
    }

    @Test
    fun `oversize server response fails within the bounded buffer`() = runTest {
        val connection = Connection(" ".repeat(MAX_SYNC_RESPONSE_BYTES + 2))
        val client = HttpBackendSyncClient(openConnection = { connection })
        assertTrue(client.enroll("https://synthetic.test", "synthetic-secret", "phone").isFailure)
        assertTrue(connection.closed)
    }

    @Test
    fun `only secure URLs and true device loopback are accepted`() {
        assertEquals("https://synthetic.test/api", normalizeSyncUrl(" https://SYNTHETIC.test:443/api/ "))
        assertEquals("http://127.0.0.1:8765", normalizeSyncUrl("http://127.0.0.1:8765/"))
        for (address in listOf("http://10.0.2.2", "http://192.168.1.2", "ftp://localhost", "https://user:secret@synthetic.test", "https://synthetic.test?secret=abc", "https://synthetic.test#fragment", "https://synthetic.test/../other")) {
            assertTrue(runCatching { normalizeSyncUrl(address) }.isFailure)
        }
    }
}
