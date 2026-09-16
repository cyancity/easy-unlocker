package io.github.cyancity.easyunlocker.crypto

import android.os.Build
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import org.json.JSONObject
import java.io.File
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

class KeystoreWrap(private val wrapFile: File) {
    fun hasWrap(): Boolean = wrapFile.exists()

    fun wrap(vaultKey: ByteArray) {
        val key = secretKey()
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.ENCRYPT_MODE, key)
        val iv = cipher.iv
        val ct = cipher.doFinal(vaultKey)
        wrapFile.writeText(
            JSONObject()
                .put("iv", VaultCrypto.b64(iv))
                .put("ct", VaultCrypto.b64(ct))
                .toString()
        )
    }

    fun unwrap(): ByteArray {
        val obj = JSONObject(wrapFile.readText())
        val iv = VaultCrypto.unb64(obj.getString("iv"))
        val ct = VaultCrypto.unb64(obj.getString("ct"))
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.DECRYPT_MODE, secretKey(), GCMParameterSpec(128, iv))
        return cipher.doFinal(ct)
    }

    private fun secretKey(): SecretKey {
        val ks = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        ks.getKey(ALIAS, null)?.let { return it as SecretKey }
        val gen = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore")
        val builder = KeyGenParameterSpec.Builder(
            ALIAS,
            KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT,
        )
            .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
            .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
            .setUserAuthenticationRequired(true)
            .setKeySize(256)
        if (Build.VERSION.SDK_INT >= 30) {
            builder.setUserAuthenticationParameters(
                30,
                android.security.keystore.KeyProperties.AUTH_BIOMETRIC_STRONG or
                    android.security.keystore.KeyProperties.AUTH_DEVICE_CREDENTIAL,
            )
        } else {
            @Suppress("DEPRECATION")
            builder.setUserAuthenticationValidityDurationSeconds(30)
        }
        gen.init(builder.build())
        return gen.generateKey()
    }

    companion object {
        private const val ALIAS = "easy-unlocker-vault-wrap"
    }
}
