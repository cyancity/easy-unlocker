import Foundation
import LocalAuthentication
import Security

/// Face ID 封存 —— 安卓 KeystoreWrap 的 iOS 对应物。
/// vault key 本体存进 Keychain，访问控制要求生物识别（Face ID）；
/// 读这一条时系统自己弹认证，不用我们管 UI。
final class KeychainWrap {
    private let service = "io.github.cyancity.easyunlocker.vault-wrap"
    private let account = "vault-key"

    func hasWrap() -> Bool {
        // iOS 26 上 UISkip 查询对访问控制条目会谎报 errSecItemNotFound——
        // 用 interactionNotAllowed 的 LAContext：存在→-25308，不存在→-25300，区分可靠。
        let ctx = LAContext()
        ctx.interactionNotAllowed = true
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecReturnData as String: false,
            kSecUseAuthenticationContext as String: ctx,
        ]
        let status = SecItemCopyMatching(query as CFDictionary, nil)
        return status == errSecSuccess || status == errSecInteractionNotAllowed
    }

    /// 把 vault key 封进 Keychain（生物识别才能取出；.biometryAny 同时覆盖
    /// Face ID / Touch ID / Optic ID）。先删旧条目再写。返回是否写入成功。
    @discardableResult
    func wrap(_ vaultKey: Data) -> Bool {
        // 失败（比如没有生物识别）只是没有这条快速解锁路径，不影响恢复码解锁。
        guard let access = SecAccessControlCreateWithFlags(
            nil,
            kSecAttrAccessibleWhenUnlockedThisDeviceOnly,
            .biometryAny,
            nil
        ) else { return false }
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
        ]
        SecItemDelete(query as CFDictionary)
        var attrs = query
        attrs[kSecValueData as String] = vaultKey
        attrs[kSecAttrAccessControl as String] = access
        return SecItemAdd(attrs as CFDictionary, nil) == errSecSuccess
    }

    /// 取 vault key —— 系统自己弹生物识别；用户取消或失败抛错。
    func unwrap(prompt: String) throws -> Data {
        let context = LAContext()
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecReturnData as String: true,
            kSecUseAuthenticationContext as String: context,
            kSecUseOperationPrompt as String: prompt,
        ]
        var result: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &result)
        guard status == errSecSuccess, let data = result as? Data else {
            throw EuError.crypto(status == errSecUserCanceled ? "已取消" : "生物识别解锁失败")
        }
        return data
    }

    func clear() {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
        ]
        SecItemDelete(query as CFDictionary)
    }
}
