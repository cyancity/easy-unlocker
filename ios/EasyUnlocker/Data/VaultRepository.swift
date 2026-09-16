import CryptoKit
import Foundation

/// 保险库：vault.eu1（argon2id + AES-256-GCM）+ Keychain Face ID 封存 + 解锁密码包裹。
/// 文件形状与安卓 VaultRepository 一致，备份文件理论上跨平台通用。
final class VaultRepository {
    private let dir: URL
    private let file: URL
    private let wrap = KeychainWrap()
    private let passwordWrapFile: URL
    private let recoveryInternal: URL

    /// 解锁后的派生 key 与明文缓存；只活在内存。
    private var key: Data?
    private var cache: VaultPlaintext?

    init() {
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first!
        let dir = base.appendingPathComponent("easy-unlocker", isDirectory: true)
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        self.dir = dir
        file = dir.appendingPathComponent("vault.eu1")
        passwordWrapFile = dir.appendingPathComponent("password.wrap")
        recoveryInternal = dir.appendingPathComponent("recovery.txt")
    }

    /// Documents 下的恢复码文件，用户在「文件」App 里能看到。
    private func publicRecoveryFile() -> URL {
        let docs = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask).first!
        return docs.appendingPathComponent("easy-unlocker-recovery.txt")
    }

    func recoveryFilePath() -> String { "Documents/\(publicRecoveryFile().lastPathComponent)" }
    func exists() -> Bool { FileManager.default.fileExists(atPath: file.path) }
    func canFingerprint() -> Bool { wrap.hasWrap() }
    func isUnlocked() -> Bool { key != nil }

    func lock() {
        if let count = key?.count {
            key?.resetBytes(in: 0..<count)
        }
        key = nil
        cache = nil
    }

    func create(recoveryCode: String) throws {
        var salt = Data(count: 16)
        _ = salt.withUnsafeMutableBytes { SecRandomCopyBytes(kSecRandomDefault, 16, $0.baseAddress!) }
        var recovery = try VaultCrypto.normalizeRecovery(recoveryCode)
        let derived = try VaultCrypto.deriveKey(recovery: recovery, salt: salt, ops: 3, memKiB: 64 * 1024)
        recovery.resetBytes(in: 0..<recovery.count)
        let fresh = VaultPlaintext(vaultId: UUID().uuidString)
        try persist(derived: derived, salt: salt, ops: 3, memKiB: 64 * 1024, vault: fresh)
        key = derived
        cache = fresh
        saveRecovery(recoveryCode)
        clearPassword()
        wrap.wrap(derived)
    }

    func unlock(recoveryCode: String) throws {
        let derived = try unlockFromRecovery(recoveryCode)
        try openWithDerived(derived)
        saveRecovery(recoveryCode)
        wrap.wrap(key!)
    }

    func unlockWithFingerprint() throws {
        if wrap.hasWrap() {
            let derived = try wrap.unwrap(prompt: "解锁保险库")
            try openWithDerived(derived)
            return
        }
        guard let stored = readStoredRecovery() else {
            throw EuError.badInput("还没有 Face ID 封存，请用恢复码解锁一次")
        }
        try unlock(recoveryCode: stored)
    }

    // MARK: - 解锁密码（批准兜底认证）

    func hasPassword() -> Bool { FileManager.default.fileExists(atPath: passwordWrapFile.path) }

    /// 设/改解锁密码：只重包内存里的 vault key，不重加密条目，所以要先解锁。
    func setPassword(_ password: String) throws {
        guard let k = key else { throw EuError.locked }
        guard !password.isEmpty else { throw EuError.badInput("密码不能为空") }
        try passwordWrap(vaultKey: k, password: password)
    }

    func clearPassword() {
        try? FileManager.default.removeItem(at: passwordWrapFile)
    }

    func unlockWithPassword(_ password: String) throws {
        let derived: Data
        do {
            derived = try passwordUnwrap(password)
        } catch {
            throw EuError.badInput("密码不对")
        }
        try openWithDerived(derived)
        wrap.wrap(key!)
    }

    /// 批准时的密码兜底：解一层 wrap 与内存里的 key 常数时间比对。
    func verifyPassword(_ password: String) -> Bool {
        guard let k = key, hasPassword() else { return false }
        guard var unwrapped = try? passwordUnwrap(password) else { return false }
        defer { unwrapped.resetBytes(in: 0..<unwrapped.count) }
        return unwrapped == k
    }

    // MARK: - password.wrap 文件（argon2id + AES-GCM，与安卓 PasswordWrap 同形）

    private let passwordAAD = Data("easy-unlocker-password-wrap".utf8)

    private func passwordWrap(vaultKey: Data, password: String) throws {
        var salt = Data(count: 16)
        _ = salt.withUnsafeMutableBytes { SecRandomCopyBytes(kSecRandomDefault, 16, $0.baseAddress!) }
        var derived = try Argon2.deriveKey(password: Data(password.utf8), salt: salt, ops: 3, memKiB: 64 * 1024)
        defer { derived.resetBytes(in: 0..<derived.count) }
        let box = try AES.GCM.seal(vaultKey, using: SymmetricKey(data: derived), authenticating: passwordAAD)
        let obj: [String: Any] = [
            "v": 1, "kdf": "argon2id",
            "salt": salt.base64EncodedString(),
            "ops": 3, "memKiB": 64 * 1024,
            "iv": Data(box.nonce).base64EncodedString(),
            "ct": (box.ciphertext + box.tag).base64EncodedString(),
        ]
        try JSONSerialization.data(withJSONObject: obj).write(to: passwordWrapFile, options: .atomic)
    }

    private func passwordUnwrap(_ password: String) throws -> Data {
        let raw = try Data(contentsOf: passwordWrapFile)
        guard let obj = try JSONSerialization.jsonObject(with: raw) as? [String: Any],
              let saltB64 = obj["salt"] as? String, let salt = Data(base64Encoded: saltB64),
              let ivB64 = obj["iv"] as? String, let iv = Data(base64Encoded: ivB64),
              let ctB64 = obj["ct"] as? String, let ct = Data(base64Encoded: ctB64) else {
            throw EuError.badInput("密码文件损坏")
        }
        var derived = try Argon2.deriveKey(
            password: Data(password.utf8), salt: salt,
            ops: UInt32(obj["ops"] as? Int ?? 3), memKiB: UInt32(obj["memKiB"] as? Int ?? 64 * 1024)
        )
        defer { derived.resetBytes(in: 0..<derived.count) }
        guard ct.count >= 16 else { throw EuError.badInput("密码文件损坏") }
        let box = try AES.GCM.SealedBox(
            nonce: AES.GCM.Nonce(data: iv),
            ciphertext: ct.prefix(ct.count - 16),
            tag: ct.suffix(16)
        )
        return try AES.GCM.open(box, using: SymmetricKey(data: derived), authenticating: passwordAAD)
    }

    // MARK: - 恢复码文件

    private func saveRecovery(_ code: String) {
        try? code.write(to: recoveryInternal, atomically: true, encoding: .utf8)
        try? (code + "\n").write(to: publicRecoveryFile(), atomically: true, encoding: .utf8)
    }

    private func readStoredRecovery() -> String? {
        for f in [recoveryInternal, publicRecoveryFile()] {
            if let text = try? String(contentsOf: f, encoding: .utf8).trimmingCharacters(in: .whitespacesAndNewlines),
               !text.isEmpty {
                return text
            }
        }
        return nil
    }

    // MARK: - 条目 CRUD

    func items() -> [VaultItem] { cache?.items ?? [] }

    /// 租户锚点（库 UUID）；锁着时拿不到，返回空串由 Broker 归 default 租户。
    func vaultId() -> String { cache?.vaultId ?? "" }

    /// 保留名（#items）不许被条目占用，否则 CLI 再也请求不到名单。
    private func requireNotReserved(_ name: String) throws {
        if ReservedItem.isReserved(name) {
            throw EuError.badInput("\(ReservedItem.list) 是保留名，不能用作条目名或别名")
        }
    }

    func add(name: String, secret: String, note: String = "", aliases: [String] = []) throws {
        guard let vault = cache else { throw EuError.locked }
        let trimmed = name.trimmingCharacters(in: .whitespaces)
        guard !trimmed.isEmpty else { throw EuError.badInput("名称不能为空") }
        try requireNotReserved(trimmed)
        for a in aliases { try requireNotReserved(a) }
        guard !secret.isEmpty || !note.isEmpty else { throw EuError.badInput("密码和备注至少填一个") }
        let names = Set(vault.items.flatMap { [$0.name] + $0.aliases }.map { $0.lowercased() })
        guard !names.contains(trimmed.lowercased()) else { throw EuError.badInput("名称或别名重复") }
        for a in aliases where names.contains(a.lowercased()) { throw EuError.badInput("名称或别名重复") }
        var next = vault
        next.items.append(VaultItem(
            id: UUID().uuidString, name: trimmed,
            aliases: aliases.map { $0.trimmingCharacters(in: .whitespaces) }.filter { !$0.isEmpty },
            secret: secret, note: note
        ))
        try save(next)
    }

    func update(id: String, name: String, secret: String, note: String = "") throws {
        guard let vault = cache else { throw EuError.locked }
        let trimmed = name.trimmingCharacters(in: .whitespaces)
        guard !trimmed.isEmpty else { throw EuError.badInput("名称不能为空") }
        try requireNotReserved(trimmed)
        guard !secret.isEmpty || !note.isEmpty else { throw EuError.badInput("密码和备注至少填一个") }
        let taken = vault.items.filter { $0.id != id }.flatMap { [$0.name] + $0.aliases }.map { $0.lowercased() }
        guard !taken.contains(trimmed.lowercased()) else { throw EuError.badInput("名称或别名重复") }
        var next = vault
        next.items = vault.items.map { $0.id == id ? VaultItem(id: id, name: trimmed, aliases: $0.aliases, secret: secret, note: note) : $0 }
        try save(next)
    }

    func delete(id: String) throws {
        guard let vault = cache else { throw EuError.locked }
        var next = vault
        next.items.removeAll { $0.id == id }
        try save(next)
    }

    func addAlias(id: String, alias: String) throws {
        let trimmed = alias.trimmingCharacters(in: .whitespaces)
        if trimmed.isEmpty { return }
        try requireNotReserved(trimmed)
        guard let vault = cache else { throw EuError.locked }
        let taken = vault.items.flatMap { [$0.name] + $0.aliases }.map { $0.lowercased() }
        guard !taken.contains(trimmed.lowercased()) else { throw EuError.badInput("名称或别名重复") }
        var next = vault
        next.items = vault.items.map {
            $0.id == id ? VaultItem(id: id, name: $0.name, aliases: $0.aliases + [trimmed], secret: $0.secret, note: $0.note) : $0
        }
        try save(next)
    }

    func matchOrNull(_ requestName: String) -> VaultItem? {
        let key = requestName.lowercased()
        let hits = items().filter { $0.name.lowercased() == key || $0.aliases.contains { $0.lowercased() == key } }
        return hits.count == 1 ? hits[0] : nil
    }

    /// 批量导入合并（对齐安卓 importEntries）：重名跳过但可补空备注；
    /// 同一用户名出现在多条候选上时整批不记别名（语义歧义交人工）。
    @discardableResult
    func importEntries(_ entries: [ImportCandidate]) throws -> ImportMergeResult {
        guard let vault = cache else { throw EuError.locked }
        var taken = Set(vault.items.flatMap { [$0.name] + $0.aliases }.map { $0.lowercased() })
        let byName = Dictionary(uniqueKeysWithValues: vault.items.map { ($0.name.lowercased(), $0) })
        var shared = Set<String>()
        var seen = Set<String>()
        for e in entries {
            for a in e.aliases.map({ $0.trimmingCharacters(in: .whitespaces).lowercased() }).filter({ !$0.isEmpty }) {
                if !seen.insert(a).inserted { shared.insert(a) }
            }
        }
        var next = vault.items
        var r = ImportMergeResult()
        for e in entries {
            let name = e.name.trimmingCharacters(in: .whitespaces)
            // 保留名与空名整条跳过：导入是批量动作，不该因一条坏名中断整批。
            if name.isEmpty || ReservedItem.isReserved(name)
                || (e.secret.trimmingCharacters(in: .whitespaces).isEmpty
                    && e.note.trimmingCharacters(in: .whitespaces).isEmpty) {
                r.skippedName += 1
                continue
            }
            if let existing = byName[name.lowercased()] {
                if existing.note.trimmingCharacters(in: .whitespaces).isEmpty
                    && !e.note.trimmingCharacters(in: .whitespaces).isEmpty,
                   let at = next.firstIndex(where: { $0.id == existing.id }) {
                    next[at] = VaultItem(id: existing.id, name: existing.name,
                                         aliases: existing.aliases, secret: existing.secret, note: e.note)
                    r.backfilled += 1
                }
                r.skippedName += 1
                continue
            }
            if taken.contains(name.lowercased()) {
                r.skippedName += 1
                continue
            }
            let wanted = e.aliases.map { $0.trimmingCharacters(in: .whitespaces) }
                .filter { !$0.isEmpty && $0.lowercased() != name.lowercased() && !ReservedItem.isReserved($0) }
            let kept = wanted.filter { !shared.contains($0.lowercased()) && !taken.contains($0.lowercased()) }
            r.droppedAliases += wanted.count - kept.count
            next.append(VaultItem(id: UUID().uuidString, name: name, aliases: kept, secret: e.secret, note: e.note))
            taken.insert(name.lowercased())
            kept.forEach { taken.insert($0.lowercased()) }
            r.imported += 1
        }
        if r.imported > 0 || r.backfilled > 0 {
            try save(VaultPlaintext(items: next, vaultId: vault.vaultId))
        }
        return r
    }

    func exportBytes() -> Data? { try? Data(contentsOf: file) }

    // MARK: - 落盘

    private func unlockFromRecovery(_ recoveryCode: String) throws -> Data {
        let parsed = try readFile()
        guard let salt = VaultCrypto.unb64(parsed.salt) else { throw EuError.badInput("保险库文件损坏") }
        var recovery = try VaultCrypto.normalizeRecovery(recoveryCode)
        defer { recovery.resetBytes(in: 0..<recovery.count) }
        return try VaultCrypto.deriveKey(recovery: recovery, salt: salt, ops: parsed.ops, memKiB: parsed.memKiB)
    }

    private func openWithDerived(_ derived: Data) throws {
        let parsed = try readFile()
        guard let nonce = VaultCrypto.unb64(parsed.nonce),
              let ciphertext = VaultCrypto.unb64(parsed.ciphertext) else {
            throw EuError.badInput("保险库文件损坏")
        }
        let plain = try VaultCrypto.decryptVault(key: derived, nonce: nonce, ciphertext: ciphertext)
        var decoded = try JSONDecoder().decode(VaultPlaintext.self, from: plain)
        key = derived
        cache = decoded
        // 老库没有 vault_id：解锁时补一个并落盘，之后配对就能声明租户（对齐安卓）。
        if decoded.vaultId.isEmpty {
            decoded.vaultId = UUID().uuidString
            try save(decoded)
        }
    }

    private func save(_ next: VaultPlaintext) throws {
        guard let k = key else { throw EuError.locked }
        var salt = Data(count: 16)
        var ops = 3, memKiB = 64 * 1024
        if let parsed = try? readFile(), let s = VaultCrypto.unb64(parsed.salt) {
            salt = s
            ops = parsed.ops
            memKiB = parsed.memKiB
        } else {
            _ = salt.withUnsafeMutableBytes { SecRandomCopyBytes(kSecRandomDefault, 16, $0.baseAddress!) }
        }
        try persist(derived: k, salt: salt, ops: ops, memKiB: memKiB, vault: next)
        cache = next
    }

    private func persist(derived: Data, salt: Data, ops: Int, memKiB: Int, vault: VaultPlaintext) throws {
        let plain = try JSONEncoder().encode(vault)
        let (nonce, ct) = try VaultCrypto.encryptVault(key: derived, plaintext: plain)
        let file = VaultFile(
            salt: VaultCrypto.b64(salt), nonce: VaultCrypto.b64(nonce),
            ciphertext: VaultCrypto.b64(ct), ops: ops, memKiB: memKiB
        )
        try JSONEncoder().encode(file).write(to: self.file, options: .atomic)
    }

    private func readFile() throws -> VaultFile {
        let raw = try Data(contentsOf: file)
        return try JSONDecoder().decode(VaultFile.self, from: raw)
    }
}

private extension Data {
    mutating func resetBytes(in range: Range<Int>) {
        withUnsafeMutableBytes { ptr in
            guard let base = ptr.baseAddress else { return }
            memset(base.advanced(by: range.lowerBound), 0, range.count)
        }
    }
}
