package org.non24.planner

import java.io.ByteArrayInputStream
import java.security.cert.CertificateException
import java.security.cert.CertificateFactory
import java.security.cert.X509Certificate
import java.util.Base64
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test
import org.non24.planner.data.ServerCertificate
import org.non24.planner.data.ServerTrust

// Two synthetic self-signed certificates for one name, made with openssl for
// this test; only their public halves are kept (ADR-0055).
class ServerTrustTest {
    private fun resource(name: String): ByteArray =
        requireNotNull(javaClass.classLoader?.getResourceAsStream("certificates/$name")) { "missing $name" }.use { it.readBytes() }

    private fun certificate(name: String): X509Certificate =
        CertificateFactory.getInstance("X.509").generateCertificate(ByteArrayInputStream(resource(name))) as X509Certificate

    @Test
    fun `a chosen certificate is described as the server prints it`() {
        val chosen = ServerCertificate.parse(resource("owner.pem"))
        assertEquals(
            "B2:36:C2:67:4F:69:88:EE:6A:E1:52:29:F8:F4:0A:02:F6:E1:85:9F:9C:3F:C5:F1:AE:CB:17:73:C9:10:06:A6",
            chosen.fingerprint,
        )
        assertEquals("zeitboard.home.test", chosen.subject)
        // DER, as some tools save one, reads the same.
        val der = Base64.getMimeDecoder().decode(
            String(resource("owner.pem")).lines().filterNot { it.startsWith("-----") }.joinToString(""),
        )
        val fromDer = ServerCertificate.parse(der)
        assertEquals(chosen.fingerprint, fromDer.fingerprint)
        assertEquals(chosen.pem, fromDer.pem)
        assertEquals(chosen, ServerCertificate.read(ByteArrayInputStream(resource("owner.pem"))))
    }

    @Test
    fun `only certificates are chosen`() {
        val key = "-----BEGIN PRIVATE KEY-----\nc3ludGhldGlj\n-----END PRIVATE KEY-----\n".toByteArray()
        for (bad in listOf(ByteArray(0), "not a certificate".toByteArray(), resource("owner.pem") + key)) {
            assertThrows(IllegalArgumentException::class.java) { ServerCertificate.parse(bad) }
        }
        assertThrows(IllegalArgumentException::class.java) {
            ServerCertificate.read(ByteArrayInputStream(ByteArray(ServerCertificate.MAX_BYTES + 1)))
        }
    }

    @Test
    fun `the owner's certificate is trusted and another with the same name is not`() {
        val trust = ServerTrust.trustManager(ServerCertificate.parse(resource("owner.pem")).pem)
        trust.checkServerTrusted(arrayOf(certificate("owner.pem")), "ECDHE_ECDSA")
        assertThrows(CertificateException::class.java) {
            trust.checkServerTrusted(arrayOf(certificate("other.pem")), "ECDHE_ECDSA")
        }
    }
}
