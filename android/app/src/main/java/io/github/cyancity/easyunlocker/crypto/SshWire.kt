package io.github.cyancity.easyunlocker.crypto

import java.io.ByteArrayOutputStream

/**
 * OpenSSH wire 编码（PROTOCOL.certkeys / PROTOCOL.key）：
 * uint32/uint64 大端；string = u32 长度 + 字节本体。
 * 只实现 ssh-ed25519 证书需要的原语，没有 mpint。
 */
class SshWriter {
    private val buf = ByteArrayOutputStream()

    fun u32(value: Long): SshWriter {
        require(value in 0..0xFFFFFFFFL) { "u32 out of range" }
        buf.write((value ushr 24).toInt() and 0xFF)
        buf.write((value ushr 16).toInt() and 0xFF)
        buf.write((value ushr 8).toInt() and 0xFF)
        buf.write(value.toInt() and 0xFF)
        return this
    }

    fun u64(value: Long): SshWriter {
        for (shift in 56 downTo 0 step 8) {
            buf.write((value ushr shift).toInt() and 0xFF)
        }
        return this
    }

    fun raw(bytes: ByteArray): SshWriter {
        buf.write(bytes, 0, bytes.size)
        return this
    }

    /** string：u32 长度 + 内容。 */
    fun str(bytes: ByteArray): SshWriter {
        u32(bytes.size.toLong())
        return raw(bytes)
    }

    fun str(text: String): SshWriter = str(text.toByteArray(Charsets.UTF_8))

    fun bytes(): ByteArray = buf.toByteArray()
}

class SshReader(private val data: ByteArray) {
    private var pos = 0

    fun byte(): Byte {
        require(remaining() >= 1) { "ssh wire: byte out of bounds" }
        return data[pos++]
    }

    fun u32(): Long {
        require(remaining() >= 4) { "ssh wire: u32 out of bounds" }
        var value = 0L
        repeat(4) { value = (value shl 8) or (data[pos++].toLong() and 0xFF) }
        return value
    }

    fun u64(): Long {
        require(remaining() >= 8) { "ssh wire: u64 out of bounds" }
        var value = 0L
        repeat(8) { value = (value shl 8) or (data[pos++].toLong() and 0xFF) }
        return value
    }

    /** string：u32 长度 + 内容。 */
    fun bytes(): ByteArray {
        val len = u32()
        require(len <= remaining().toLong()) { "ssh wire: string out of bounds" }
        val out = data.copyOfRange(pos, pos + len.toInt())
        pos += len.toInt()
        return out
    }

    fun string(): String = bytes().toString(Charsets.UTF_8)

    fun remaining(): Int = data.size - pos

    /** 已消费的字节数——签证书时用它切出「被签名的那段」。 */
    fun consumed(): Int = pos

    fun expectEnd() {
        require(pos == data.size) { "ssh wire: trailing ${data.size - pos} bytes" }
    }
}
