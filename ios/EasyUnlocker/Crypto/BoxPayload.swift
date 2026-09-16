import CryptoKit
import Foundation
import Security

/// 一次性密封信封，与安卓 BoxPayload / Go boxpayload 同一线上格式：
/// "v2." + b64url-nopad( ephPub(32) || nonce(12) || AES-256-GCM(HKDF(shared), AAD=requestId) )
enum BoxPayload {
    static let version = "v2"
    static let pubLen = 32
    static let nonceLen = 12

    static func seal(recipientPubB64: String, requestId: String, plaintext: Data) throws -> String {
        let recipient = try decodePub(recipientPubB64)
        let eph = Curve25519.KeyAgreement.PrivateKey()
        let shared = try eph.sharedSecretFromKeyAgreement(with: recipient)
        let key = shared.hkdfDerivedSymmetricKey(
            using: SHA256.self,
            salt: Data("easy-unlocker/v2/box/".utf8),
            sharedInfo: Data(requestId.utf8),
            outputByteCount: 32
        )
        var nonceRaw = [UInt8](repeating: 0, count: nonceLen)
        _ = SecRandomCopyBytes(kSecRandomDefault, nonceLen, &nonceRaw)
        let nonce = try AES.GCM.Nonce(data: Data(nonceRaw))
        let box = try AES.GCM.seal(plaintext, using: key, nonce: nonce, authenticating: Data(requestId.utf8))
        var raw = Data()
        raw.append(eph.publicKey.rawRepresentation)
        raw.append(Data(nonceRaw))
        raw.append(box.ciphertext)
        raw.append(box.tag)
        return version + "." + b64url(raw)
    }

    private static func decodePub(_ b64: String) throws -> Curve25519.KeyAgreement.PublicKey {
        let trimmed = b64.trimmingCharacters(in: .whitespaces)
        if let raw = unb64url(trimmed), raw.count == pubLen {
            return try Curve25519.KeyAgreement.PublicKey(rawRepresentation: raw)
        }
        if let raw = Data(base64Encoded: trimmed), raw.count == pubLen {
            return try Curve25519.KeyAgreement.PublicKey(rawRepresentation: raw)
        }
        throw EuError.badInput("invalid seal public key")
    }

    static func b64url(_ raw: Data) -> String {
        raw.base64EncodedString()
            .replacingOccurrences(of: "+", with: "-")
            .replacingOccurrences(of: "/", with: "_")
            .replacingOccurrences(of: "=", with: "")
    }

    static func unb64url(_ value: String) -> Data? {
        var s = value
            .replacingOccurrences(of: "-", with: "+")
            .replacingOccurrences(of: "_", with: "/")
        while s.count % 4 != 0 { s.append("=") }
        return Data(base64Encoded: s)
    }
}
