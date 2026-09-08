package org.non24.planner.data

import java.net.HttpURLConnection
import java.net.URI
import java.net.URL
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.longOrNull
import kotlinx.serialization.json.put

interface BackendSyncClient {
    suspend fun enroll(baseUrl: String, enrollmentSecret: String, label: String): Result<String>

    suspend fun push(baseUrl: String, token: String, records: List<OutboxRecord>): Result<List<String>>

    suspend fun pull(baseUrl: String, token: String, since: Long): Result<PullPage>

    suspend fun companion(baseUrl: String, token: String): Result<JsonObject>

    suspend fun sleepReview(baseUrl: String, token: String, observationId: String): Result<JsonObject>
}

class BackendSyncException(val status: Int, message: String) : Exception(message)

const val SYNC_BATCH_LIMIT = 100
internal const val MAX_SYNC_RESPONSE_BYTES = 64 * 1024

/** Credentials, query strings and fragments never belong in configuration. */
internal fun normalizeSyncUrl(value: String): String {
    val uri = URI(value.trim())
    val host = uri.host?.lowercase() ?: error("Enter the server's HTTPS address.")
    val protocol = uri.scheme?.lowercase()
    require(protocol == "https" || (protocol == "http" && host in setOf("localhost", "127.0.0.1", "[::1]"))) {
        "Sync requires HTTPS outside this device's loopback address."
    }
    require(uri.rawUserInfo == null && uri.rawQuery == null && uri.rawFragment == null && uri.port in -1..65535) {
        "Use a server address without credentials, a query or a fragment."
    }
    require(uri.port != 0 && uri.normalize().rawPath == uri.rawPath) { "Enter a valid server address." }
    require(uri.path.orEmpty().split('/').none { it == "." || it == ".." }) { "Enter a valid server path." }
    val port = if ((protocol == "https" && uri.port == 443) || (protocol == "http" && uri.port == 80)) -1 else uri.port
    return "$protocol://$host" + (if (port == -1) "" else ":$port") + uri.rawPath.orEmpty().trimEnd('/')
}

/** Cancellation is control flow; never turn a stopped worker into a retryable failure. */
internal suspend fun <T> syncResult(block: suspend () -> T): Result<T> = try {
    Result.success(block())
} catch (cancelled: CancellationException) {
    throw cancelled
} catch (error: Exception) {
    Result.failure(error)
}

internal fun JsonObject.string(name: String): String =
    (get(name) as? JsonPrimitive)?.takeIf { it.isString }?.content
        ?: throw BackendSyncException(200, "The server returned an invalid response.")

class HttpBackendSyncClient(
    private val connectTimeoutMillis: Int = 15_000,
    private val readTimeoutMillis: Int = 30_000,
    private val openConnection: (URL) -> HttpURLConnection = { it.openConnection() as HttpURLConnection },
) : BackendSyncClient {
    override suspend fun sleepReview(baseUrl: String, token: String, observationId: String): Result<JsonObject> = syncResult {
        require(validSyncId(observationId))
        request(baseUrl, "/v1/sleep/$observationId/review", token, null, 256 * 1024).also { require(parseSleepReview(it).observationId == observationId) }
    }

    override suspend fun pull(baseUrl: String, token: String, since: Long): Result<PullPage> = syncResult {
        require(since >= 0)
        parsePullPage(request(baseUrl, "/v1/sync/pull?since=$since&limit=$SYNC_PULL_LIMIT", token, null, 8 * 1024 * 1024), since)
    }

    override suspend fun companion(baseUrl: String, token: String): Result<JsonObject> = syncResult {
        request(baseUrl, "/v2/companion", token, null, 256 * 1024).also { parseCompanion(it) }
    }
    override suspend fun enroll(baseUrl: String, enrollmentSecret: String, label: String): Result<String> =
        syncResult {
            require(enrollmentSecret.isNotBlank() && label.isNotBlank()) { "An enrollment secret and device label are required." }
            val body = buildJsonObject {
                put("enrollmentSecret", enrollmentSecret)
                put("label", label)
            }
            val response = request(baseUrl, "/v1/devices", null, body.toString())
            require(response.string("schema_version") == "v1") { "The server uses an unsupported enrollment contract." }
            response.string("token").also {
                require(it.isNotBlank() && it.length <= 4096 && it.none(Char::isISOControl)) {
                    "The server did not return a valid device token."
                }
            }
        }

    override suspend fun push(baseUrl: String, token: String, records: List<OutboxRecord>): Result<List<String>> =
        syncResult {
            if (records.isEmpty()) return@syncResult emptyList()
            require(records.size <= SYNC_BATCH_LIMIT)
            val body = buildJsonObject {
                put("schema_version", SyncContract.SCHEMA_VERSION)
                put("records", buildJsonArray {
                    records.forEach { record ->
                        add(buildJsonObject {
                            put("recordId", record.recordId)
                            put("kind", record.kind)
                            put("createdAt", record.createdAt.toString())
                            put("payload", Json.parseToJsonElement(record.payload).jsonObject)
                        })
                    }
                })
            }
            val response = request(baseUrl, "/v1/sync/push", token, body.toString())
            val accepted = response["accepted"]?.jsonPrimitive?.takeUnless { it.isString }?.intOrNull
            val cursor = response["cursor"]?.jsonPrimitive?.takeUnless { it.isString }?.longOrNull
            require(response.string("schema_version") == SyncContract.SCHEMA_VERSION &&
                accepted != null && accepted in 0..records.size && cursor != null && cursor >= 0) {
                "The server returned an invalid upload acknowledgment."
            }
            // Append is atomic. Accepted counts newly inserted rows only, so zero
            // is valid when retrying an already committed batch.
            records.map { it.recordId }
        }

    private suspend fun request(baseUrl: String, path: String, token: String?, body: String?, responseLimit: Int = MAX_SYNC_RESPONSE_BYTES): JsonObject =
        withContext(Dispatchers.IO) {
            val bytes = body?.toByteArray(Charsets.UTF_8) ?: byteArrayOf()
            require(bytes.size <= 1024 * 1024) { "The upload exceeds the server's request limit." }
            val connection = openConnection(URL(normalizeSyncUrl(baseUrl) + path))
            try {
                connection.requestMethod = if (body == null) "GET" else "POST"
                connection.instanceFollowRedirects = false
                connection.connectTimeout = connectTimeoutMillis
                connection.readTimeout = readTimeoutMillis
                connection.doOutput = body != null
                connection.setRequestProperty("Content-Type", "application/json")
                connection.setRequestProperty("Accept", "application/json")
                if (token != null) connection.setRequestProperty("Authorization", "Bearer $token")
                if (body != null) {
                    connection.setFixedLengthStreamingMode(bytes.size)
                    connection.outputStream.use { it.write(bytes) }
                }
                val status = connection.responseCode
                if (status !in 200..299) throw BackendSyncException(status, describe(status))
                val response = connection.inputStream.use { stream ->
                    val output = java.io.ByteArrayOutputStream()
                    val buffer = ByteArray(8192)
                    while (true) {
                        val count = stream.read(buffer, 0, minOf(buffer.size, responseLimit + 1 - output.size()))
                        if (count < 0) break
                        output.write(buffer, 0, count)
                        require(output.size() <= responseLimit) { "The server response exceeds the size limit." }
                    }
                    output.toByteArray()
                }
                Json.parseToJsonElement(response.toString(Charsets.UTF_8)).jsonObject
            } finally {
                connection.disconnect()
            }
        }

    private fun describe(status: Int): String = when (status) {
        401, 403 -> "Enrollment was refused or this device was revoked. Check the secret or enroll again."
        409 -> "The server has conflicting records. The upload is retained for review."
        404 -> "This server does not support the companion feature. Update ZeitBoard on your server."
        413 -> "The upload was too large for the server."
        429 -> "The server is busy. Try again later."
        in 300..399 -> "The server redirected the request. Enter its final HTTPS address."
        in 500..599 -> "The server is not answering correctly right now."
        else -> "The server refused the request."
    }
}
