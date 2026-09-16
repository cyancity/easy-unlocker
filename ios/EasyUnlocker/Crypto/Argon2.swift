import Foundation

/// Argon2id KDF —— 与安卓端 VaultCrypto.deriveKey 同一组参数语义：
/// version 1.3、lanes=1、outlen=32。底层是 Vendor/Argon2 里的参考实现。
enum Argon2 {
    enum Error: LocalizedError {
        case failed(Int32)
        var errorDescription: String? { "Argon2 派生失败（\(_code)）" }
        private var _code: Int32 {
            if case .failed(let c) = self { return c }; return -1
        }
    }

    /// argon2id(pwd, salt, ops, memKiB) -> 32 字节
    static func deriveKey(password: Data, salt: Data, ops: UInt32, memKiB: UInt32) throws -> Data {
        var out = Data(count: 32)
        let rc: Int32 = out.withUnsafeMutableBytes { outPtr in
            password.withUnsafeBytes { pwdPtr in
                salt.withUnsafeBytes { saltPtr in
                    argon2id_hash_raw(
                        ops, memKiB, 1,
                        pwdPtr.baseAddress, pwdPtr.count,
                        saltPtr.baseAddress, saltPtr.count,
                        outPtr.baseAddress, outPtr.count
                    )
                }
            }
        }
        guard rc == ARGON2_OK.rawValue else { throw Error.failed(rc) }
        return out
    }
}
