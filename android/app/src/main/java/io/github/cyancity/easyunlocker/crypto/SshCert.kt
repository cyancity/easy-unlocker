package io.github.cyancity.easyunlocker.crypto

import org.bouncycastle.crypto.params.Ed25519PrivateKeyParameters
import org.bouncycastle.crypto.signers.Ed25519Signer
import java.security.SecureRandom
import java.util.Base64

/**
 * OpenSSH 用户证书签发（PROTOCOL.certkeys，ssh-ed25519-cert-v01@openssh.com）。
 *
 * 签名字段覆盖「从证书类型串到 signature key 字段」的全部字节；整张证书 = 已签段 +
 * string signatureBlob（nested：string "ssh-ed25519" + string 64B 签名）。
 *
 * 与 `cli/ssh.go` 的 VerifyCertificate、fakephone 的 signCert 出同一种形状：
 * ValidAfter = now-120s 回拨（吸收目标机时钟差），ValidBefore = now + certTTL，principal 恰好一个。
 */
object SshCert {
    const val CERT_KEY_TYPE = "ssh-ed25519-cert-v01@openssh.com"
    const val DEFAULT_TTL = 300
    const val MAX_TTL = 86400
    private const val CERT_TYPE_USER = 1L

    /**
     * 与 `ssh-keygen -s` 默认发出的同一组 permit-* 扩展（按字典序）。
     * 空扩展段的证书在 sshd 那边拿不到 pty / 转发等能力，等于登上去是个死 shell。
     */
    private val DEFAULT_EXTENSIONS = listOf(
        "permit-X11-forwarding",
        "permit-agent-forwarding",
        "permit-port-forwarding",
        "permit-pty",
        "permit-user-rc",
    )

    /**
     * 签一张用户证书，返回 authorized_keys 一行（"ssh-ed25519-cert-v01@openssh.com AAAA…"）。
     * subjectLine 是 CLI 发来的 `public_key`（"ssh-ed25519 AAAA…"）。
     */
    fun signUser(
        ca: OpenSshKey.Private,
        subjectLine: String,
        principal: String,
        requestId: String,
        certTtlSeconds: Int,
        nowEpoch: Long = System.currentTimeMillis() / 1000,
    ): String {
        require(principal.isNotBlank()) { "ssh principal is required" }
        val subject = OpenSshKey.subjectPublicKey(subjectLine)
        val ttl = if (certTtlSeconds > 0) certTtlSeconds else DEFAULT_TTL
        require(ttl <= MAX_TTL) { "cert_ttl 超过上限 $MAX_TTL 秒" }
        val nonce = ByteArray(32).also { SecureRandom().nextBytes(it) }

        val w = SshWriter()
        w.str(CERT_KEY_TYPE)
        w.str(nonce)
        w.str(subject)
        w.u64(SecureRandom().nextLong())
        w.u32(CERT_TYPE_USER)
        w.str("easy-unlocker/$requestId")
        w.str(nested { str(principal) })
        // 回拨 120s 吸收手机与目标机的时钟差：回拨不足会在 sshd 侧报
        // "Certificate invalid: not yet valid"（2026-09-15 真机冒烟实测目标机时钟慢 ~45s）。
        w.u64(nowEpoch - 120)
        w.u64(nowEpoch + ttl)
        w.str(ByteArray(0)) // critical options：空
        w.str(nested { DEFAULT_EXTENSIONS.forEach { str(it); str(ByteArray(0)) } })
        w.str(ByteArray(0)) // reserved：空
        w.str(ca.publicBlob)

        val signer = Ed25519Signer()
        signer.init(true, Ed25519PrivateKeyParameters(ca.seed, 0))
        val signed = w.bytes()
        signer.update(signed, 0, signed.size)
        val signature = signer.generateSignature()
        w.str(nested { str(OpenSshKey.KEY_TYPE); str(signature) })

        return CERT_KEY_TYPE + " " + Base64.getEncoder().encodeToString(w.bytes())
    }

    private fun nested(block: SshWriter.() -> Unit): ByteArray {
        val inner = SshWriter()
        inner.block()
        return inner.bytes()
    }
}
