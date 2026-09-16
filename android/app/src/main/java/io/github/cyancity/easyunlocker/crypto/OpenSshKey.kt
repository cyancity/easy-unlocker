package io.github.cyancity.easyunlocker.crypto

import org.bouncycastle.crypto.params.Ed25519PrivateKeyParameters
import java.security.MessageDigest
import java.security.SecureRandom
import java.util.Base64

/**
 * `openssh-key-v1` 私钥（PEM "OPENSSH PRIVATE KEY"）的解析与生成。
 * 只支持 ssh-ed25519 + ciphername=none（无 passphrase）——条目本来就整体加密存放，
 * CA 私钥只是用来在批准时签证书的。
 */
object OpenSshKey {
    const val KEY_TYPE = "ssh-ed25519"
    private const val MAGIC = "openssh-key-v1"
    private const val PEM_BEGIN = "-----BEGIN OPENSSH PRIVATE KEY-----"
    private const val PEM_END = "-----END OPENSSH PRIVATE KEY-----"
    private const val BLOCK_SIZE = 8
    private const val PEM_WRAP = 70

    class Private(
        /** 32 字节 ed25519 种子（OpenSSH 私钥段的前半）。 */
        val seed: ByteArray,
        /** 32 字节公钥。 */
        val publicKey: ByteArray,
        val comment: String,
    ) {
        /** CA 公钥的 SSH blob（string keytype + string pubkey），签证书时进 signature key 字段。 */
        val publicBlob: ByteArray get() = SshWriter().str(KEY_TYPE).str(publicKey).bytes()

        /** `ssh-ed25519 AAAA…` authorized_keys 一行，给目标机 TrustedUserCAKeys 用。 */
        fun authorizedLine(): String = KEY_TYPE + " " + Base64.getEncoder().encodeToString(publicBlob)

        /** 和 `ssh-keygen -lf` 同款指纹。 */
        fun fingerprint(): String = "SHA256:" + Base64.getEncoder().withoutPadding()
            .encodeToString(MessageDigest.getInstance("SHA-256").digest(publicBlob))

        fun toPem(): String = encode(this)
    }

    /** 这段文本看起来像不像一把 OpenSSH 私钥（只认 PEM 头，不保证能解）。 */
    fun looksLikePrivateKey(text: String): Boolean = text.contains(PEM_BEGIN)

    /** 解析失败返回 null——调用方用它探这条目是不是 CA。 */
    fun parseOrNull(text: String): Private? = runCatching { parse(text) }.getOrNull()

    fun parse(pem: String): Private {
        val body = pem.substringAfter(PEM_BEGIN, "").substringBefore(PEM_END, "")
        require(body.isNotBlank()) { "不是 OpenSSH 私钥" }
        val blob = Base64.getMimeDecoder().decode(body.filterNot { it.isWhitespace() })
        val r = SshReader(blob)
        repeat(MAGIC.length + 1) { i ->
            val expected = if (i < MAGIC.length) MAGIC[i].code.toByte() else 0
            require(r.byte() == expected) { "不是 openssh-key-v1 格式" }
        }
        require(r.string() == "none") { "私钥是加密的（ciphername≠none），先在终端解开再放进来" }
        require(r.string() == "none") { "不支持带 kdf 的私钥" }
        r.bytes() // kdfoptions
        require(r.u32() == 1L) { "只支持单钥匙文件" }
        val pubBlob = r.bytes()
        val pubReader = SshReader(pubBlob)
        require(pubReader.string() == KEY_TYPE) { "只支持 ssh-ed25519" }
        val pubKey = pubReader.bytes()
        pubReader.expectEnd()
        val privSection = SshReader(r.bytes())
        r.expectEnd()
        val check1 = privSection.u32()
        val check2 = privSection.u32()
        require(check1 == check2) { "私钥段校验失败（文件可能损坏）" }
        require(privSection.string() == KEY_TYPE) { "只支持 ssh-ed25519" }
        val innerPub = privSection.bytes()
        val combined = privSection.bytes()
        require(combined.size == 64) { "ed25519 私钥段长度不对" }
        require(innerPub.contentEquals(pubKey) && combined.copyOfRange(32, 64).contentEquals(pubKey)) {
            "私钥与公钥不匹配"
        }
        val comment = privSection.string()
        // padding：1,2,3,… 递增。
        var expect = 1
        while (privSection.remaining() > 0) {
            require(privSection.byte() == expect.toByte()) { "私钥段 padding 损坏" }
            expect++
        }
        return Private(combined.copyOf(32), pubKey, comment)
    }

    /** 生成一把新 CA：随机 ed25519，comment 默认 easy-unlocker-ca。 */
    fun generate(comment: String = "easy-unlocker-ca"): Private {
        val params = Ed25519PrivateKeyParameters(SecureRandom())
        return Private(params.encoded, params.generatePublicKey().encoded, comment)
    }

    /** 序列化成 PEM（无加密，sshd 与 ssh-keygen 都能直接读）。 */
    fun encode(key: Private): String {
        val pubBlob = key.publicBlob
        val checkInt = SecureRandom().nextInt().toLong() and 0xFFFFFFFFL
        val inner = SshWriter()
        inner.u32(checkInt)
        inner.u32(checkInt)
        inner.str(KEY_TYPE)
        inner.str(key.publicKey)
        inner.str(key.seed + key.publicKey)
        inner.str(key.comment)
        // OpenSSH 的写法：padding 补到 8 的倍数；正好对齐时补一整块（padlen 1..8）。
        val padlen = BLOCK_SIZE - inner.bytes().size % BLOCK_SIZE
        for (i in 1..padlen) inner.raw(byteArrayOf(i.toByte()))
        val outer = SshWriter()
        outer.raw(MAGIC.toByteArray() + byteArrayOf(0))
        outer.str("none")
        outer.str("none")
        outer.str(ByteArray(0))
        outer.u32(1)
        outer.str(pubBlob)
        outer.str(inner.bytes())
        val b64 = Base64.getEncoder().encodeToString(outer.bytes())
        return buildString {
            append(PEM_BEGIN).append('\n')
            b64.chunked(PEM_WRAP).forEach { append(it).append('\n') }
            append(PEM_END).append('\n')
        }
    }

    /** 从 authorized_keys 行（"ssh-ed25519 AAAA [comment]"）取出 32 字节公钥。 */
    fun subjectPublicKey(line: String): ByteArray {
        val parts = line.trim().split(Regex("\\s+"))
        require(parts.size >= 2 && parts[0] == KEY_TYPE) { "公钥行不是 ssh-ed25519" }
        val r = SshReader(Base64.getDecoder().decode(parts[1]))
        require(r.string() == KEY_TYPE) { "公钥 blob 不是 ssh-ed25519" }
        val key = r.bytes()
        r.expectEnd()
        require(key.size == 32) { "ed25519 公钥长度不对" }
        return key
    }

    /** 公钥行的 SHA256 指纹，展示用（`ssh-keygen -lf` 同款）。 */
    fun fingerprintOf(line: String): String {
        val parts = line.trim().split(Regex("\\s+"))
        require(parts.size >= 2) { "不是 authorized_keys 行" }
        val blob = Base64.getDecoder().decode(parts[1])
        return "SHA256:" + Base64.getEncoder().withoutPadding()
            .encodeToString(MessageDigest.getInstance("SHA-256").digest(blob))
    }
}
