package io.github.cyancity.easyunlocker.crypto

import org.json.JSONObject
import java.io.File
import java.security.SecureRandom
import javax.crypto.Cipher
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.SecretKeySpec

/**
 * 用户自设的解锁密码 → Argon2id → AES-GCM 包裹 vault key，落 `password.wrap`。
 *
 * 与 KeystoreWrap（指纹/锁屏）并列的第二条解锁路径，相当于 Bitwarden 的主密码；
 * 恢复码仍是最终备份。密码只存 KDF 参数 + 包裹结果，不存本体；改密码只重包
 * vault key，不动 vault.eu1。换新库（create/importBytes）时旧 wrap 必须清掉，
 * 否则解出来的是上一个库的钥匙。
 */
class PasswordWrap(private val wrapFile: File) {
    fun hasWrap(): Boolean = wrapFile.exists()

    fun wrap(vaultKey: ByteArray, password: String) {
        val salt = ByteArray(16).also { SecureRandom().nextBytes(it) }
        val derived = derive(password, salt, OPS, MEM_KIB)
        try {
            val cipher = Cipher.getInstance("AES/GCM/NoPadding")
            cipher.init(Cipher.ENCRYPT_MODE, SecretKeySpec(derived, "AES"))
            cipher.updateAAD(AAD)
            val iv = cipher.iv
            val ct = cipher.doFinal(vaultKey)
            wrapFile.writeText(
                JSONObject()
                    .put("v", 1)
                    .put("kdf", "argon2id")
                    .put("salt", VaultCrypto.b64(salt))
                    .put("ops", OPS)
                    .put("memKiB", MEM_KIB)
                    .put("iv", VaultCrypto.b64(iv))
                    .put("ct", VaultCrypto.b64(ct))
                    .toString()
            )
        } finally {
            derived.fill(0)
        }
    }

    /** 密码错 → AEADBadTagException（GCM mac 校验失败）。 */
    fun unwrap(password: String): ByteArray {
        val obj = JSONObject(wrapFile.readText())
        val derived = derive(
            password,
            VaultCrypto.unb64(obj.getString("salt")),
            obj.optInt("ops", OPS),
            obj.optInt("memKiB", MEM_KIB),
        )
        try {
            val cipher = Cipher.getInstance("AES/GCM/NoPadding")
            cipher.init(
                Cipher.DECRYPT_MODE,
                SecretKeySpec(derived, "AES"),
                GCMParameterSpec(128, VaultCrypto.unb64(obj.getString("iv"))),
            )
            cipher.updateAAD(AAD)
            return cipher.doFinal(VaultCrypto.unb64(obj.getString("ct")))
        } finally {
            derived.fill(0)
        }
    }

    fun clear() {
        wrapFile.delete()
    }

    private fun derive(password: String, salt: ByteArray, ops: Int, memKiB: Int): ByteArray {
        val bytes = password.toByteArray(Charsets.UTF_8)
        return try {
            VaultCrypto.deriveKey(bytes, salt, ops, memKiB)
        } finally {
            bytes.fill(0)
        }
    }

    companion object {
        private const val OPS = 3
        private const val MEM_KIB = 64 * 1024
        private val AAD = "easy-unlocker-password-wrap".toByteArray(Charsets.UTF_8)
    }
}
