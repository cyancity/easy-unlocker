import CryptoKit
import Foundation
import Security

/// `openssh-key-v1` 私钥（PEM "OPENSSH PRIVATE KEY"）的解析与生成。
/// 只支持 ssh-ed25519 + ciphername=none（无 passphrase）——条目本来就整体加密存放，
/// CA 私钥只是用来在批准时签证书的。
enum OpenSshKey {
    static let keyType = "ssh-ed25519"
    private static let magic = "openssh-key-v1"
    static let pemBegin = "-----BEGIN OPENSSH PRIVATE KEY-----"
    static let pemEnd = "-----END OPENSSH PRIVATE KEY-----"
    private static let blockSize = 8
    private static let pemWrap = 70

    struct PrivateKey {
        /// 32 字节 ed25519 种子（OpenSSH 私钥段的前半）。
        let seed: Data
        /// 32 字节公钥。
        let publicKey: Data
        let comment: String

        /// CA 公钥的 SSH blob（string keytype + string pubkey），签证书时进 signature key 字段。
        var publicBlob: Data {
            var w = SshWriter()
            w.str(OpenSshKey.keyType)
            w.str(publicKey)
            return w.bytes()
        }

        /// `ssh-ed25519 AAAA…` authorized_keys 一行，给目标机 TrustedUserCAKeys 用。
        func authorizedLine() -> String {
            OpenSshKey.keyType + " " + publicBlob.base64EncodedString()
        }

        /// 和 `ssh-keygen -lf` 同款指纹。
        func fingerprint() -> String {
            "SHA256:" + Data(SHA256.hash(data: publicBlob)).base64EncodedString()
                .replacingOccurrences(of: "=", with: "")
        }

        func toPem() -> String { OpenSshKey.encode(self) }
    }

    /// 这段文本看起来像不像一把 OpenSSH 私钥（只认 PEM 头，不保证能解）。
    static func looksLikePrivateKey(_ text: String) -> Bool { text.contains(pemBegin) }

    /// 解析失败返回 null——调用方用它探这条目是不是 CA。
    static func parseOrNull(_ text: String) -> PrivateKey? { try? parse(text) }

    static func parse(_ pem: String) throws -> PrivateKey {
        guard let beginRange = pem.range(of: pemBegin),
              let endRange = pem.range(of: pemEnd) else {
            throw EuError.badInput("不是 OpenSSH 私钥")
        }
        let body = pem[beginRange.upperBound..<endRange.lowerBound]
            .filter { !$0.isWhitespace }
        guard let blob = Data(base64Encoded: String(body)) else {
            throw EuError.badInput("不是 OpenSSH 私钥")
        }
        var r = SshReader(blob)
        for i in 0...(magic.count) {
            let expected: UInt8 = i < magic.count ? magic.utf8[magic.utf8.index(magic.utf8.startIndex, offsetBy: i)] : 0
            guard try r.byte() == expected else { throw EuError.badInput("不是 openssh-key-v1 格式") }
        }
        guard try r.string() == "none" else {
            throw EuError.badInput("私钥是加密的（ciphername≠none），先在终端解开再放进来")
        }
        guard try r.string() == "none" else { throw EuError.badInput("不支持带 kdf 的私钥") }
        _ = try r.bytes() // kdfoptions
        guard try r.u32() == 1 else { throw EuError.badInput("只支持单钥匙文件") }
        let pubBlob = try r.bytes()
        var pubReader = SshReader(pubBlob)
        guard try pubReader.string() == keyType else { throw EuError.badInput("只支持 ssh-ed25519") }
        let pubKey = try pubReader.bytes()
        try pubReader.expectEnd()
        var privSection = SshReader(try r.bytes())
        try r.expectEnd()
        let check1 = try privSection.u32()
        let check2 = try privSection.u32()
        guard check1 == check2 else { throw EuError.badInput("私钥段校验失败（文件可能损坏）") }
        guard try privSection.string() == keyType else { throw EuError.badInput("只支持 ssh-ed25519") }
        let innerPub = try privSection.bytes()
        let combined = try privSection.bytes()
        guard combined.count == 64 else { throw EuError.badInput("ed25519 私钥段长度不对") }
        guard innerPub == pubKey, combined.subdata(in: 32..<64) == pubKey else {
            throw EuError.badInput("私钥与公钥不匹配")
        }
        let comment = try privSection.string()
        // padding：1,2,3,… 递增。
        var expect: UInt8 = 1
        while privSection.remaining() > 0 {
            guard try privSection.byte() == expect else { throw EuError.badInput("私钥段 padding 损坏") }
            expect += 1
        }
        return PrivateKey(seed: combined.prefix(32), publicKey: pubKey, comment: comment)
    }

    /// 生成一把新 CA：随机 ed25519，comment 默认 easy-unlocker-ca。
    static func generate(comment: String = "easy-unlocker-ca") -> PrivateKey {
        let key = Curve25519.Signing.PrivateKey()
        return PrivateKey(seed: key.rawRepresentation, publicKey: key.publicKey.rawRepresentation, comment: comment)
    }

    /// 序列化成 PEM（无加密，sshd 与 ssh-keygen 都能直接读）。
    static func encode(_ key: PrivateKey) -> String {
        let pubBlob = key.publicBlob
        var checkRaw = [UInt8](repeating: 0, count: 4)
        _ = SecRandomCopyBytes(kSecRandomDefault, 4, &checkRaw)
        let checkInt = UInt64(UInt32(bigEndian: checkRaw.withUnsafeBytes { $0.load(as: UInt32.self) }))
        var inner = SshWriter()
        inner.u32(checkInt)
        inner.u32(checkInt)
        inner.str(keyType)
        inner.str(key.publicKey)
        inner.str(key.seed + key.publicKey)
        inner.str(key.comment)
        // OpenSSH 的写法：padding 补到 8 的倍数；正好对齐时补一整块（padlen 1..8）。
        let padlen = blockSize - inner.bytes().count % blockSize
        for i in 1...padlen { inner.raw(Data([UInt8(i)])) }
        var outer = SshWriter()
        outer.raw(Data(magic.utf8) + Data([0]))
        outer.str("none")
        outer.str("none")
        outer.str(Data())
        outer.u32(1)
        outer.str(pubBlob)
        outer.str(inner.bytes())
        let b64 = outer.bytes().base64EncodedString()
        var out = pemBegin + "\n"
        for chunk in b64.chunked(into: pemWrap) { out += chunk + "\n" }
        out += pemEnd + "\n"
        return out
    }

    /// 从 authorized_keys 行（"ssh-ed25519 AAAA [comment]"）取出 32 字节公钥。
    static func subjectPublicKey(_ line: String) throws -> Data {
        let parts = line.trimmingCharacters(in: .whitespaces)
            .components(separatedBy: .whitespaces).filter { !$0.isEmpty }
        guard parts.count >= 2, parts[0] == keyType,
              let blob = Data(base64Encoded: parts[1]) else {
            throw EuError.badInput("公钥行不是 ssh-ed25519")
        }
        var r = SshReader(blob)
        guard try r.string() == keyType else { throw EuError.badInput("公钥 blob 不是 ssh-ed25519") }
        let key = try r.bytes()
        try r.expectEnd()
        guard key.count == 32 else { throw EuError.badInput("ed25519 公钥长度不对") }
        return key
    }

    /// 公钥行的 SHA256 指纹，展示用（`ssh-keygen -lf` 同款）。
    static func fingerprintOf(_ line: String) throws -> String {
        let parts = line.trimmingCharacters(in: .whitespaces)
            .components(separatedBy: .whitespaces).filter { !$0.isEmpty }
        guard parts.count >= 2, let blob = Data(base64Encoded: parts[1]) else {
            throw EuError.badInput("不是 authorized_keys 行")
        }
        return "SHA256:" + Data(SHA256.hash(data: blob)).base64EncodedString()
            .replacingOccurrences(of: "=", with: "")
    }
}
