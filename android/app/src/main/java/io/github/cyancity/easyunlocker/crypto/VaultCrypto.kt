package io.github.cyancity.easyunlocker.crypto

import android.util.Base64
import org.bouncycastle.crypto.generators.Argon2BytesGenerator
import org.bouncycastle.crypto.params.Argon2Parameters
import org.bouncycastle.crypto.engines.AESEngine
import org.bouncycastle.crypto.modes.GCMBlockCipher
import org.bouncycastle.crypto.params.AEADParameters
import org.bouncycastle.crypto.params.KeyParameter
import java.security.SecureRandom

data class VaultFile(
    val v: Int = 1,
    val kdf: String = "argon2id",
    val salt: String,
    val nonce: String,
    val ciphertext: String,
    val ops: Int = 3,
    val memKiB: Int = 64 * 1024,
)

object VaultCrypto {
    fun newRecoveryCode(): String {
        val raw = ByteArray(16)
        SecureRandom().nextBytes(raw)
        return raw.joinToString("") { "%02x".format(it) }.chunked(4).joinToString("-")
    }

    fun normalizeRecovery(code: String): ByteArray {
        val hex = code.lowercase().replace("-", "").replace(" ", "")
        require(hex.length == 32) { "恢复码长度不对" }
        return hex.chunked(2).map { it.toInt(16).toByte() }.toByteArray()
    }

    fun deriveKey(recovery: ByteArray, salt: ByteArray, ops: Int, memKiB: Int): ByteArray {
        val params = Argon2Parameters.Builder(Argon2Parameters.ARGON2_id)
            .withSalt(salt)
            .withIterations(ops)
            .withMemoryAsKB(memKiB)
            .withParallelism(1)
            .build()
        val gen = Argon2BytesGenerator()
        gen.init(params)
        val out = ByteArray(32)
        gen.generateBytes(recovery, out)
        return out
    }

    fun encryptVault(key: ByteArray, plaintext: ByteArray): Pair<ByteArray, ByteArray> {
        val nonce = ByteArray(12).also { SecureRandom().nextBytes(it) }
        val cipher = GCMBlockCipher.newInstance(AESEngine.newInstance())
        cipher.init(true, AEADParameters(KeyParameter(key), 128, nonce, "easy-unlocker-vault".toByteArray()))
        val out = ByteArray(cipher.getOutputSize(plaintext.size))
        val n = cipher.processBytes(plaintext, 0, plaintext.size, out, 0)
        cipher.doFinal(out, n)
        return nonce to out
    }

    fun decryptVault(key: ByteArray, nonce: ByteArray, ciphertext: ByteArray): ByteArray {
        val cipher = GCMBlockCipher.newInstance(AESEngine.newInstance())
        cipher.init(false, AEADParameters(KeyParameter(key), 128, nonce, "easy-unlocker-vault".toByteArray()))
        val out = ByteArray(cipher.getOutputSize(ciphertext.size))
        val n = cipher.processBytes(ciphertext, 0, ciphertext.size, out, 0)
        cipher.doFinal(out, n)
        return out
    }

    fun b64(raw: ByteArray): String = Base64.encodeToString(raw, Base64.NO_WRAP)
    fun unb64(value: String): ByteArray = Base64.decode(value, Base64.NO_WRAP)
}
