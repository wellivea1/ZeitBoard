package org.non24.planner.data

import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.io.InputStream
import java.security.KeyStore
import java.security.MessageDigest
import java.security.cert.CertificateException
import java.security.cert.CertificateFactory
import java.security.cert.X509Certificate
import java.time.Instant
import java.util.Base64
import java.util.concurrent.ConcurrentHashMap
import javax.net.ssl.SSLContext
import javax.net.ssl.SSLSocketFactory
import javax.net.ssl.TrustManagerFactory
import javax.net.ssl.X509TrustManager

/**
 * The owner's server: its address, and the one certificate it is trusted by
 * beside the system's roots, when the owner gave one (ADR-0055). Verification
 * stays on either way; the server must present a valid certificate for its
 * address.
 */
data class SyncServer(val baseUrl: String, val certificate: String? = null)

/** A certificate the owner chose, and what they check it by. */
data class ServerCertificate(
    val pem: String,
    /** SHA-256, as "AB:CD:…", the form the server prints. */
    val fingerprint: String,
    val subject: String,
    val expiresAt: Instant,
) {
    companion object {
        /** A certificate file is a few kilobytes; anything far larger is not one. */
        const val MAX_BYTES = 64 * 1024
        private const val MAX_CERTIFICATES = 8

        /**
         * Reads a certificate file, PEM or a single DER certificate, and
         * describes the first certificate in it. A private key is refused.
         */
        fun parse(data: ByteArray): ServerCertificate {
            require(data.size <= MAX_BYTES) { "The file is too large to be a certificate." }
            val text = data.toString(Charsets.US_ASCII)
            require("PRIVATE KEY" !in text) { "That file holds a private key. Choose the server's certificate." }
            val certificates = try {
                CertificateFactory.getInstance("X.509")
                    .generateCertificates(ByteArrayInputStream(data))
                    .map { it as X509Certificate }
            } catch (_: CertificateException) {
                emptyList()
            }
            require(certificates.isNotEmpty()) { "That file holds no certificate." }
            require(certificates.size <= MAX_CERTIFICATES) { "That file holds more certificates than a server's chain." }
            val first = certificates.first()
            return ServerCertificate(
                pem = certificates.joinToString("") { pem(it) },
                fingerprint = fingerprint(first),
                subject = first.subjectX500Principal.name
                    .split(',').firstOrNull { it.trim().startsWith("CN=") }?.trim()?.removePrefix("CN=")
                    ?: first.subjectX500Principal.name,
                expiresAt = first.notAfter.toInstant(),
            )
        }

        /** Reads a chosen file, refusing one too large to be a certificate. */
        fun read(stream: InputStream): ServerCertificate {
            val output = ByteArrayOutputStream()
            val buffer = ByteArray(8192)
            while (true) {
                val count = stream.read(buffer)
                if (count < 0) break
                output.write(buffer, 0, count)
                require(output.size() <= MAX_BYTES) { "The file is too large to be a certificate." }
            }
            return parse(output.toByteArray())
        }

        fun fingerprint(certificate: X509Certificate): String =
            MessageDigest.getInstance("SHA-256").digest(certificate.encoded)
                .joinToString(":") { "%02X".format(it.toInt() and 0xff) }

        private fun pem(certificate: X509Certificate): String =
            "-----BEGIN CERTIFICATE-----\n" +
                Base64.getMimeEncoder(64, "\n".toByteArray()).encodeToString(certificate.encoded) +
                "\n-----END CERTIFICATE-----\n"
    }
}

/** Socket factories that trust the system's roots and one chosen certificate. */
internal object ServerTrust {
    private val factories = ConcurrentHashMap<String, SSLSocketFactory>()

    fun socketFactory(certificatePem: String): SSLSocketFactory =
        factories.getOrPut(certificatePem) {
            SSLContext.getInstance("TLS").apply {
                init(null, arrayOf(trustManager(certificatePem)), null)
            }.socketFactory
        }

    /** The system's roots, and then the chosen certificate as a further anchor. */
    fun trustManager(certificatePem: String): X509TrustManager {
        val system = managerFor(null)
        val anchors = KeyStore.getInstance(KeyStore.getDefaultType()).apply {
            load(null, null)
            CertificateFactory.getInstance("X.509")
                .generateCertificates(ByteArrayInputStream(certificatePem.toByteArray(Charsets.US_ASCII)))
                .forEachIndexed { index, certificate -> setCertificateEntry("owner-$index", certificate) }
        }
        val owner = managerFor(anchors)
        return object : X509TrustManager {
            override fun checkClientTrusted(chain: Array<out X509Certificate>, authType: String) =
                system.checkClientTrusted(chain, authType)

            override fun checkServerTrusted(chain: Array<out X509Certificate>, authType: String) {
                try {
                    system.checkServerTrusted(chain, authType)
                } catch (_: CertificateException) {
                    owner.checkServerTrusted(chain, authType)
                }
            }

            override fun getAcceptedIssuers(): Array<X509Certificate> = system.acceptedIssuers + owner.acceptedIssuers
        }
    }

    private fun managerFor(keyStore: KeyStore?): X509TrustManager =
        TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm()).run {
            init(keyStore)
            trustManagers.filterIsInstance<X509TrustManager>().first()
        }
}
