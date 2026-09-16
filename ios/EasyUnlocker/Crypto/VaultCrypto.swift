import CryptoKit
import Foundation
import Security

/// 保险库落盘形状（vault.eu1），与安卓端 VaultFile 字段一致：
/// argon2id(恢复码, salt, ops, memKiB) → AES-256-GCM(AAD="easy-unlocker-vault")。
struct VaultFile: Codable {
    var v: Int = 1
    var kdf: String = "argon2id"
    var salt: String
    var nonce: String
    var ciphertext: String
    var ops: Int = 3
    var memKiB: Int = 64 * 1024
}

enum VaultCrypto {
    static let aad = Data("easy-unlocker-vault".utf8)

    static func newRecoveryCode() -> String {
        var raw = [UInt8](repeating: 0, count: 16)
        _ = SecRandomCopyBytes(kSecRandomDefault, 16, &raw)
        return raw.map { String(format: "%02x", $0) }.joined()
            .chunked(into: 4).joined(separator: "-")
    }

    /// "xxxx-xxxx-…" → 16 字节；长度不对直接抛错。
    static func normalizeRecovery(_ code: String) throws -> Data {
        let hex = code.lowercased()
            .replacingOccurrences(of: "-", with: "")
            .replacingOccurrences(of: " ", with: "")
        guard hex.count == 32 else { throw EuError.badInput("恢复码长度不对") }
        var out = Data()
        var i = hex.startIndex
        while i < hex.endIndex {
            let j = hex.index(i, offsetBy: 2)
            guard let b = UInt8(hex[i..<j], radix: 16) else {
                throw EuError.badInput("恢复码里有非法字符")
            }
            out.append(b)
            i = j
        }
        return out
    }

    static func deriveKey(recovery: Data, salt: Data, ops: Int, memKiB: Int) throws -> Data {
        try Argon2.deriveKey(password: recovery, salt: salt, ops: UInt32(ops), memKiB: UInt32(memKiB))
    }

    /// AES-256-GCM：返回（nonce, 密文+tag）。tag 128bit 与安卓 GCMBlockCipher 一致。
    static func encryptVault(key: Data, plaintext: Data) throws -> (nonce: Data, ciphertext: Data) {
        let nonce = AES.GCM.Nonce()
        let box = try AES.GCM.seal(plaintext, using: SymmetricKey(data: key), nonce: nonce, authenticating: aad)
        return (Data(nonce), box.ciphertext + box.tag)
    }

    static func decryptVault(key: Data, nonce: Data, ciphertext: Data) throws -> Data {
        guard ciphertext.count >= 16 else { throw EuError.badInput("密文太短") }
        let ct = ciphertext.prefix(ciphertext.count - 16)
        let tag = ciphertext.suffix(16)
        let box = try AES.GCM.SealedBox(nonce: AES.GCM.Nonce(data: nonce), ciphertext: ct, tag: tag)
        return try AES.GCM.open(box, using: SymmetricKey(data: key), authenticating: aad)
    }

    static func b64(_ raw: Data) -> String { raw.base64EncodedString() }
    static func unb64(_ value: String) -> Data? { Data(base64Encoded: value) }
}

enum EuError: LocalizedError {
    case badInput(String)
    case locked
    case crypto(String)
    case network(String)
    var errorDescription: String? {
        switch self {
        case .badInput(let m), .crypto(let m), .network(let m): return m
        case .locked: return "locked"
        }
    }
}

extension String {
    func chunked(into n: Int) -> [String] {
        var out: [String] = []
        var i = startIndex
        while i < endIndex {
            let j = index(i, offsetBy: n, limitedBy: endIndex) ?? endIndex
            out.append(String(self[i..<j]))
            i = j
        }
        return out
    }
}
