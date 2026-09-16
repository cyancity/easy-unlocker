import CryptoKit
import Foundation
import Security

/// OpenSSH 用户证书签发（PROTOCOL.certkeys，ssh-ed25519-cert-v01@openssh.com）。
/// 与安卓 SshCert 出同一种形状：ValidAfter = now-120s 回拨，ValidBefore = now+certTTL，
/// principal 恰好一个，extensions 与 `ssh-keygen -s` 默认同组。
enum SshCert {
    static let certKeyType = "ssh-ed25519-cert-v01@openssh.com"
    static let defaultTTL = 300
    static let maxTTL = 86400
    private static let certTypeUser: UInt64 = 1

    private static let defaultExtensions = [
        "permit-X11-forwarding",
        "permit-agent-forwarding",
        "permit-port-forwarding",
        "permit-pty",
        "permit-user-rc",
    ]

    /// 签一张用户证书，返回 authorized_keys 一行（"ssh-ed25519-cert-v01@openssh.com AAAA…"）。
    static func signUser(ca: OpenSshKey.PrivateKey, subjectLine: String, principal: String,
                         requestId: String, certTtlSeconds: Int, nowEpoch: UInt64 = UInt64(Date().timeIntervalSince1970)) throws -> String {
        guard !principal.isEmpty else { throw EuError.badInput("ssh principal is required") }
        let subject = try OpenSshKey.subjectPublicKey(subjectLine)
        let ttl = certTtlSeconds > 0 ? certTtlSeconds : defaultTTL
        guard ttl <= maxTTL else { throw EuError.badInput("cert_ttl 超过上限 \(maxTTL) 秒") }
        var nonce = [UInt8](repeating: 0, count: 32)
        _ = SecRandomCopyBytes(kSecRandomDefault, 32, &nonce)

        var serialRaw = [UInt8](repeating: 0, count: 8)
        _ = SecRandomCopyBytes(kSecRandomDefault, 8, &serialRaw)
        let serialNum = serialRaw.withUnsafeBytes { $0.load(as: UInt64.self) }

        var w = SshWriter()
        w.str(certKeyType)
        w.str(Data(nonce))
        w.str(subject)
        w.u64(serialNum)
        w.u32(certTypeUser)
        w.str("easy-unlocker/\(requestId)")
        w.str(nested { $0.str(principal) })
        // 回拨 120s 吸收手机与目标机的时钟差。
        w.u64(nowEpoch > 120 ? nowEpoch - 120 : 0)
        w.u64(nowEpoch + UInt64(ttl))
        w.str(Data()) // critical options：空
        w.str(nested { w in defaultExtensions.forEach { w.str($0); w.str(Data()) } })
        w.str(Data()) // reserved：空
        w.str(ca.publicBlob)

        let signer = try Curve25519.Signing.PrivateKey(rawRepresentation: ca.seed)
        let signed = w.bytes()
        let signature = try signer.signature(for: signed)
        w.str(nested { $0.str(OpenSshKey.keyType); $0.str(signature) })

        return certKeyType + " " + w.bytes().base64EncodedString()
    }

    private static func nested(_ block: (inout SshWriter) -> Void) -> Data {
        var inner = SshWriter()
        block(&inner)
        return inner.bytes()
    }
}
