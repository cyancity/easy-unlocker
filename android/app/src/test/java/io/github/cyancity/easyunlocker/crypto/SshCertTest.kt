package io.github.cyancity.easyunlocker.crypto

import org.bouncycastle.crypto.params.Ed25519PrivateKeyParameters
import org.bouncycastle.crypto.params.Ed25519PublicKeyParameters
import org.bouncycastle.crypto.signers.Ed25519Signer
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.util.Base64

class SshCertTest {

    @Test
    fun `pem roundtrip keeps the same key`() {
        val ca = OpenSshKey.generate("test-ca")
        val parsed = OpenSshKey.parse(ca.toPem())
        assertTrue(ca.seed.contentEquals(parsed.seed))
        assertTrue(ca.publicKey.contentEquals(parsed.publicKey))
        assertEquals("test-ca", parsed.comment)
        assertEquals(ca.authorizedLine(), parsed.authorizedLine())
    }

    @Test
    fun `generated pem parses after re-encode`() {
        // encode → parse → encode → parse：格式两端自洽
        val ca = OpenSshKey.parse(OpenSshKey.generate().toPem())
        val again = OpenSshKey.parse(ca.toPem())
        assertTrue(ca.publicKey.contentEquals(again.publicKey))
    }

    @Test
    fun `certificate verifies against ca and matches request`() {
        val ca = OpenSshKey.generate()
        val identity = OpenSshKey.generate("ephemeral")
        val subjectLine = identity.authorizedLine()
        val requestId = "abc123"
        val principal = "deployer"
        val certTtl = 600
        val now = 1_800_000_000L

        val line = SshCert.signUser(ca, subjectLine, principal, requestId, certTtl, nowEpoch = now)
        val parts = line.trim().split(" ")
        assertEquals(SshCert.CERT_KEY_TYPE, parts[0])
        val r = SshReader(Base64.getDecoder().decode(parts[1]))
        assertEquals(SshCert.CERT_KEY_TYPE, r.string())
        r.bytes() // nonce
        val certPub = r.bytes()
        assertTrue(identity.publicKey.contentEquals(certPub))
        r.u64() // serial
        assertEquals(1L, r.u32()) // user cert
        assertEquals("easy-unlocker/$requestId", r.string())
        val principals = SshReader(r.bytes())
        assertEquals(principal, principals.string())
        principals.expectEnd()
        assertEquals(now - 120, r.u64())
        assertEquals(now + certTtl, r.u64())
        SshReader(r.bytes()).expectEnd() // critical options 空
        val extensions = SshReader(r.bytes())
        var sawPty = false
        while (extensions.remaining() > 0) {
            if (extensions.string() == "permit-pty") sawPty = true
            extensions.bytes()
        }
        extensions.expectEnd()
        assertTrue("证书要带上 permit-pty，否则登录后没有 pty", sawPty)
        r.bytes() // reserved
        val sigKeyBlob = r.bytes()
        val sigKeyReader = SshReader(sigKeyBlob)
        assertEquals(OpenSshKey.KEY_TYPE, sigKeyReader.string())
        assertTrue(ca.publicKey.contentEquals(sigKeyReader.bytes()))
        // 签名段到此为止；最后一个字段是签名本身
        val signedLength = r.consumed()
        val sigReader = SshReader(r.bytes())
        assertEquals(OpenSshKey.KEY_TYPE, sigReader.string())
        val signature = sigReader.bytes()
        r.expectEnd()

        val verifier = Ed25519Signer()
        verifier.init(false, Ed25519PublicKeyParameters(ca.publicKey, 0))
        val whole = Base64.getDecoder().decode(parts[1])
        verifier.update(whole, 0, signedLength)
        assertTrue("证书签名必须能用 CA 公钥验过", verifier.verifySignature(signature))

        // 篡改签名区域里的一个字节 → 验签必须失败
        val tampered = whole.copyOf()
        tampered[10] = (tampered[10] + 1).toByte()
        val tamperVerifier = Ed25519Signer()
        tamperVerifier.init(false, Ed25519PublicKeyParameters(ca.publicKey, 0))
        tamperVerifier.update(tampered, 0, signedLength)
        org.junit.Assert.assertFalse("被篡改的证书不该验过", tamperVerifier.verifySignature(signature))
    }

    /**
     * 把 CA PEM、CA 公钥、身份公钥、签出的证书写进 build/ssh-crosscheck/，
     * 让 `ssh-keygen -y -f ca`、`ssh-keygen -L -f identity-cert.pub` 能对拍——
     * 我们自己的解析器自洽不算数，OpenSSH 认得才算数。
     */
    @Test
    fun `write crosscheck artifacts`() {
        val dir = java.io.File("build/ssh-crosscheck").apply { mkdirs() }
        val ca = OpenSshKey.generate("crosscheck-ca")
        java.io.File(dir, "ca").writeText(ca.toPem())
        java.io.File(dir, "ca.pub").writeText(ca.authorizedLine() + " crosscheck-ca\n")
        val identity = OpenSshKey.generate("cli-ephemeral")
        java.io.File(dir, "identity.pub").writeText(identity.authorizedLine() + " cli\n")
        val cert = SshCert.signUser(ca, identity.authorizedLine(), "deployer", "req-1", 600)
        java.io.File(dir, "identity-cert.pub").writeText(cert + "\n")
        assertTrue(java.io.File(dir, "identity-cert.pub").length() > 100)
    }

    @Test
    fun `subject public key line parses`() {
        val identity = OpenSshKey.generate("who")
        val key = OpenSshKey.subjectPublicKey(identity.authorizedLine() + " some-comment")
        assertTrue(identity.publicKey.contentEquals(key))
        assertEquals("SHA256:", OpenSshKey.fingerprintOf(identity.authorizedLine()).take(7))
    }

    @Test
    fun `rejects non-ed25519 and garbage`() {
        val ca = OpenSshKey.generate()
        org.junit.Assert.assertThrows(Exception::class.java) {
            SshCert.signUser(ca, "ssh-rsa AAAA", "u", "r", 300)
        }
        org.junit.Assert.assertThrows(Exception::class.java) {
            OpenSshKey.parse("-----BEGIN OPENSSH PRIVATE KEY-----\nAAAA\n-----END OPENSSH PRIVATE KEY-----")
        }
    }
}
