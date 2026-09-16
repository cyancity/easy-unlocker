package io.github.cyancity.easyunlocker.crypto

import android.util.Base64
import org.bouncycastle.crypto.digests.SHA256Digest
import org.bouncycastle.crypto.engines.AESEngine
import org.bouncycastle.crypto.generators.HKDFBytesGenerator
import org.bouncycastle.crypto.modes.GCMBlockCipher
import org.bouncycastle.crypto.params.AEADParameters
import org.bouncycastle.crypto.params.HKDFParameters
import org.bouncycastle.crypto.params.KeyParameter
import org.bouncycastle.math.ec.rfc7748.X25519
import java.security.SecureRandom

object BoxPayload {
    private const val VERSION = "v2"
    private const val PUB = 32
    private const val NONCE = 12

    fun seal(recipientPubB64: String, requestId: String, plaintext: ByteArray): String {
        val recipient = b64(recipientPubB64)
        require(recipient.size == PUB) { "invalid seal public key" }
        val senderSk = ByteArray(PUB)
        val senderPk = ByteArray(PUB)
        SecureRandom().nextBytes(senderSk)
        X25519.generatePublicKey(senderSk, 0, senderPk, 0)
        val shared = ByteArray(PUB)
        require(X25519.calculateAgreement(senderSk, 0, recipient, 0, shared, 0)) { "invalid seal public key" }
        senderSk.fill(0)
        val key = hkdf(shared, requestId)
        shared.fill(0)
        val nonce = ByteArray(NONCE).also { SecureRandom().nextBytes(it) }
        val cipher = GCMBlockCipher.newInstance(AESEngine.newInstance())
        cipher.init(true, AEADParameters(KeyParameter(key), 128, nonce, requestId.toByteArray()))
        val out = ByteArray(cipher.getOutputSize(plaintext.size))
        val n = cipher.processBytes(plaintext, 0, plaintext.size, out, 0)
        cipher.doFinal(out, n)
        key.fill(0)
        val raw = ByteArray(PUB + NONCE + out.size)
        System.arraycopy(senderPk, 0, raw, 0, PUB)
        System.arraycopy(nonce, 0, raw, PUB, NONCE)
        System.arraycopy(out, 0, raw, PUB + NONCE, out.size)
        return VERSION + "." + Base64.encodeToString(raw, Base64.NO_WRAP or Base64.URL_SAFE or Base64.NO_PADDING)
    }

    private fun hkdf(shared: ByteArray, requestId: String): ByteArray {
        val gen = HKDFBytesGenerator(SHA256Digest())
        gen.init(HKDFParameters(shared, "easy-unlocker/v2/box/".toByteArray(), requestId.toByteArray()))
        val key = ByteArray(32)
        gen.generateBytes(key, 0, 32)
        return key
    }

    private fun b64(value: String): ByteArray {
        val trimmed = value.trim()
        val url = Base64.decode(trimmed, Base64.NO_WRAP or Base64.URL_SAFE or Base64.NO_PADDING)
        if (url.size == PUB) return url
        return Base64.decode(trimmed, Base64.NO_WRAP)
    }
}
