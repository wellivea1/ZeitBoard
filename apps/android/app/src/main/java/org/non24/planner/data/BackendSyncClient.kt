package org.non24.planner.data

import java.io.BufferedReader
import java.net.HttpURLConnection
import java.net.URL
import javax.net.ssl.HttpsURLConnection

/**
 * Talks to the user's own self-hosted instance.
 *
 * There is no vendor SDK and no third party here by design: the only host this
 * ever contacts is the one the user typed in. It is an interface so the
 * orchestration can be tested without a server, and so a transport change never
 * requires touching the mapping rules.
 */
interface BackendSyncClient {
    /** Exchanges the enrollment secret for a per-device bearer token. */
    suspend fun enroll(baseUrl: String, enrollmentSecret: String, label: String): Result<String>

    /** Pushes a batch. Returns the record ids the server accepted. */
    suspend fun push(baseUrl: String, token: String, records: List<OutboxRecord>): Result<List<String>>
}

/** Raised when the server answers, but not with success. */
class BackendSyncException(val status: Int, message: String) : Exception(message)

/**
 * Bounds one request so a long offline stretch cannot produce a batch the
 * server refuses outright. The server caps the request body; staying well under
 * it here means the phone never has to discover that limit by being rejected.
 */
const val SYNC_BATCH_LIMIT: Int = 100

class HttpBackendSyncClient(
    private val connectTimeoutMillis: Int = 15_000,
    private val readTimeoutMillis: Int = 30_000,
    private val openConnection: (URL) -> HttpURLConnection = { it.openConnection() as HttpURLConnection },
) : BackendSyncClient {

    override suspend fun enroll(
        baseUrl: String,
        enrollmentSecret: String,
        label: String,
    ): Result<String> = runCatching {
        val body = buildString {
            append("{\"enrollmentSecret\":").append(jsonString(enrollmentSecret)).append(",")
            append("\"label\":").append(jsonString(label)).append("}")
        }
        val response = request(baseUrl, "/v1/devices", token = null, body = body)
        val token = extractString(response, "token")
            ?: throw BackendSyncException(200, "The server did not return a device token.")
        token
    }

    override suspend fun push(
        baseUrl: String,
        token: String,
        records: List<OutboxRecord>,
    ): Result<List<String>> = runCatching {
        if (records.isEmpty()) return@runCatching emptyList()
        require(records.size <= SYNC_BATCH_LIMIT) {
            "A sync batch must not exceed $SYNC_BATCH_LIMIT records."
        }
        val body = buildString {
            append("{\"schema_version\":").append(jsonString(SyncContract.SCHEMA_VERSION)).append(",")
            append("\"records\":[")
            records.forEachIndexed { index, record ->
                if (index > 0) append(",")
                append("{\"recordId\":").append(jsonString(record.recordId)).append(",")
                append("\"kind\":").append(jsonString(record.kind)).append(",")
                append("\"createdAt\":").append(jsonString(record.createdAt.toString())).append(",")
                append("\"payload\":").append(record.payload).append("}")
            }
            append("]}")
        }
        request(baseUrl, "/v1/sync/push", token = token, body = body)
        // The server answers with a cursor and an accepted count rather than a
        // list of ids. A 2xx means the whole batch was accepted, so the ids we
        // sent are the ids to mark.
        records.map { it.recordId }
    }

    private fun request(baseUrl: String, path: String, token: String?, body: String): String {
        val url = URL(baseUrl.trimEnd('/') + path)
        require(url.protocol == "https" || isLoopback(url.host)) {
            "Sync requires https outside loopback."
        }
        val connection = openConnection(url)
        try {
            connection.requestMethod = "POST"
            connection.connectTimeout = connectTimeoutMillis
            connection.readTimeout = readTimeoutMillis
            connection.doOutput = true
            connection.setRequestProperty("Content-Type", "application/json")
            if (token != null) {
                connection.setRequestProperty("Authorization", "Bearer $token")
            }
            if (connection is HttpsURLConnection) {
                // Default trust: the operator's certificate must validate. No
                // skip-verify escape hatch ships here, because a phone in a
                // cafe is exactly where one would be exploited.
                connection.hostnameVerifier = HttpsURLConnection.getDefaultHostnameVerifier()
            }
            connection.outputStream.use { it.write(body.toByteArray(Charsets.UTF_8)) }

            val status = connection.responseCode
            val stream = if (status in 200..299) connection.inputStream else connection.errorStream
            val text = stream?.bufferedReader()?.use(BufferedReader::readText).orEmpty()
            if (status !in 200..299) {
                throw BackendSyncException(status, describe(status))
            }
            return text
        } finally {
            connection.disconnect()
        }
    }

    /**
     * Turns a status into something a person can act on. The server's own error
     * text is deliberately not surfaced: it is written for an operator reading
     * logs, not for someone holding a phone.
     */
    private fun describe(status: Int): String = when (status) {
        401, 403 -> "This device is not enrolled with that server, or its token was revoked."
        409 -> "The server already has a different record with one of these ids."
        413 -> "The batch was too large for the server."
        in 500..599 -> "The server is not answering correctly right now."
        else -> "The server refused the request."
    }

    private fun isLoopback(host: String): Boolean =
        host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "10.0.2.2"

    private fun extractString(json: String, key: String): String? {
        val marker = "\"$key\""
        val keyIndex = json.indexOf(marker)
        if (keyIndex < 0) return null
        val colon = json.indexOf(':', keyIndex + marker.length)
        if (colon < 0) return null
        val open = json.indexOf('"', colon)
        if (open < 0) return null
        val builder = StringBuilder()
        var index = open + 1
        while (index < json.length) {
            val character = json[index]
            when {
                character == '\\' && index + 1 < json.length -> {
                    builder.append(json[index + 1])
                    index += 2
                }
                character == '"' -> return builder.toString()
                else -> {
                    builder.append(character)
                    index++
                }
            }
        }
        return null
    }

    private fun jsonString(value: String): String {
        val escaped = value.replace("\\", "\\\\").replace("\"", "\\\"")
        return "\"" + escaped + "\""
    }
}
