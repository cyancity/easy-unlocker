import Foundation
import LocalAuthentication
import SwiftUI

/// 审批结果页状态（已放行 / 已拒绝），替代安卓的 Screen.Approved 跳转。
struct ApprovalResult: Equatable {
    var approved: Bool
    var item: String
    var requester: String
    var isList: Bool
    var isCert: Bool
}

@MainActor
final class AppState: ObservableObject {
    // MARK: 保险库
    @Published var vaultExists = false
    @Published var unlocked = false
    @Published var recoveryPreview = ""
    @Published var canFingerprint = false
    @Published var hasPassword = false
    @Published var recoveryPath = ""

    // MARK: 数据
    @Published var items: [VaultItem] = []
    @Published var pending: [PendingRequest] = []
    @Published var selectedPending: PendingRequest? = nil
    @Published var history: [HistoryEntry] = []

    // MARK: 网关
    @Published var pairings: [Pairing] = []
    @Published var activePairingId = ""
    var activePairing: Pairing? { pairings.first { $0.id == activePairingId } }
    var paired: Bool { activePairing != nil }
    var deviceName: String { activePairing?.name ?? "" }

    // MARK: 行为与展示
    @Published var rememberAlias = true
    @Published var toast = ""
    @Published var formError = ""
    @Published var message = ""
    @Published var loading = false
    @Published var offline = false
    @Published var lastSync = ""
    @Published var timedOut = false
    @Published var incoming = false
    @Published var result: ApprovalResult? = nil
    @Published var selectedTab = 0
    @Published var haptics = true
    @Published var sound = true
    @Published var autoCloseSeconds = 3
    @Published var lastExportAt = ""
    @Published var lastImportAt = ""

    let repo = VaultRepository()
    private let prefs = Prefs()
    private var historyStore: HistoryStore!
    private var pairingStore: PairingStore!
    private var pollTask: Task<Void, Never>? = nil
    /// 待批准审批期间的引用计数：锁住不让后台切走自动上锁。
    private var vaultHold = 0
    private var pushToken = ""
    private var toastGen = 0

    init() {
        let dir = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first!
            .appendingPathComponent("easy-unlocker", isDirectory: true)
        historyStore = HistoryStore(dir: dir)
        pairingStore = PairingStore(dir: dir)
        reload()
    }

    func reload() {
        vaultExists = repo.exists()
        recoveryPreview = vaultExists ? "" : VaultCrypto.newRecoveryCode()
        pairings = pairingStore.load()
        if !pairings.isEmpty, pairings.allSatisfy({ $0.id != prefs.activePairingId }) {
            prefs.activePairingId = pairings.first!.id
        }
        activePairingId = prefs.activePairingId
        canFingerprint = !Self.uiTesting && repo.canFingerprint()
        hasPassword = repo.hasPassword()
        recoveryPath = repo.recoveryFilePath()
        haptics = prefs.haptics
        sound = prefs.sound
        // UI 测试里禁用结果页自动关闭——3s 后 dismissResult 会把 tab 拉回条目页顶掉断言
        autoCloseSeconds = Self.uiTesting ? 0 : prefs.autoCloseSecondsOrDefault
        lastExportAt = prefs.lastExportAt
        lastImportAt = prefs.lastImportAt
        history = historyStore.load()
    }

    private func client() -> BrokerClient? {
        guard let p = activePairing else { return nil }
        return BrokerClient(brokerUrl: p.url, deviceToken: p.deviceToken)
    }

    // MARK: - 解锁 / 建库

    func createVault() {
        let code = recoveryPreview
        holdVault()
        confirmAuth("确认创建保险库") { [weak self] in
            Task { [weak self] in
                guard let self else { return }
                do {
                    try await Task.detached { try self.repo.create(recoveryCode: code) }.value
                    self.releaseVault()
                    self.openVault()
                } catch {
                    self.releaseVault()
                    self.showToast(error.localizedDescription)
                }
            }
        }
    }

    func unlock(recoveryCode: String) {
        Task {
            do {
                try await Task.detached { try self.repo.unlock(recoveryCode: recoveryCode) }.value
                openVault()
            } catch { showToast(error.localizedDescription) }
        }
    }

    func unlockWithFingerprint() {
        Task {
            do {
                try await Task.detached { try self.repo.unlockWithFingerprint() }.value
                openVault()
            } catch { showToast(error.localizedDescription) }
        }
    }

    func unlockWithPassword(_ password: String) {
        Task {
            do {
                try await Task.detached { try self.repo.unlockWithPassword(password) }.value
                openVault()
            } catch { showToast(error.localizedDescription) }
        }
    }

    func setPassword(_ password: String) {
        Task {
            do {
                try await Task.detached { try self.repo.setPassword(password) }.value
                hasPassword = true
                showToast("解锁密码已设置")
            } catch { showToast(error.localizedDescription) }
        }
    }

    func clearPassword() {
        repo.clearPassword()
        hasPassword = false
        showToast("解锁密码已清除")
    }

    func lock() {
        if vaultHold > 0 { return }
        repo.lock()
        unlocked = false
        items = []
        message = pending.isEmpty ? "" : "有请求待批准"
        canFingerprint = !Self.uiTesting && repo.canFingerprint()
        hasPassword = repo.hasPassword()
    }

    private func openVault() {
        let bioWasReady = canFingerprint
        vaultExists = true
        unlocked = true
        items = repo.items()
        message = ""
        canFingerprint = !Self.uiTesting && repo.canFingerprint()
        // 密码/恢复码解锁成功后若刚写好生物封存，提示一句——下次开 app 直接刷脸/指纹。
        if !bioWasReady && canFingerprint && !Self.uiTesting {
            showToast("已启用 \(biometryName) 快速解锁")
        }
        hasPassword = repo.hasPassword()
        recoveryPath = repo.recoveryFilePath()
        // 有遗留 pending 时跳到待批准；UI 测试里禁止自动跳 tab（会把设置页顶掉）
        if !pending.isEmpty && !Self.uiTesting { selectedTab = 1 }
        startPolling()
        silentRefresh(showLoading: pending.isEmpty)
    }

    // MARK: - 网关（最小实现：能配对、能切换、能删）

    func pair(url: String, pairingToken: String, name: String) async -> String? {
        let cleanUrl = url.trimmingCharacters(in: .whitespaces)
            .replacingOccurrences(of: "/+$", with: "", options: .regularExpression)
        guard !cleanUrl.isEmpty, !pairingToken.isEmpty else {
            return "Broker 地址和 pairing token 都要填。"
        }
        do {
            let label = name.trimmingCharacters(in: .whitespaces).isEmpty ? gatewayLabel(cleanUrl) : name.trimmingCharacters(in: .whitespaces)
            let deviceToken = try await BrokerClient(brokerUrl: cleanUrl, deviceToken: "").pair(pairingToken: pairingToken, name: label, vaultId: repo.vaultId())
            let existing = pairings.first { normalizeGatewayUrl($0.url) == normalizeGatewayUrl(cleanUrl) }
            let entry = existing.map { Pairing(id: $0.id, name: label, url: cleanUrl, deviceToken: deviceToken, createdAt: $0.createdAt) }
                ?? Pairing(id: UUID().uuidString, name: label, url: cleanUrl, deviceToken: deviceToken, createdAt: Int64(Date().timeIntervalSince1970 * 1000))
            if let existing {
                pairings = pairings.map { $0.id == existing.id ? entry : $0 }
            } else {
                pairings.append(entry)
            }
            pairingStore.save(pairings)
            prefs.activePairingId = entry.id
            activePairingId = entry.id
            pending = []
            selectedPending = nil
            if !pushToken.isEmpty {
                try? await BrokerClient(brokerUrl: cleanUrl, deviceToken: deviceToken).registerPushToken(pushToken)
            }
            startPolling()
            return nil
        } catch {
            return error.localizedDescription
        }
    }

    func switchPairing(_ id: String) {
        guard id != activePairingId, let target = pairings.first(where: { $0.id == id }) else { return }
        prefs.activePairingId = id
        activePairingId = id
        pending = []
        selectedPending = nil
        incoming = false
        timedOut = false
        offline = false
        showToast("已切到 \(target.name)")
        startPolling()
        silentRefresh(showLoading: true)
    }

    /// 网关改名是本地标签（Broker 那边不知道网关名；设备名走 renameDevice）。
    func renamePairing(_ id: String, name: String) {
        let trimmed = name.trimmingCharacters(in: .whitespaces)
        guard !trimmed.isEmpty else { return }
        pairings = pairings.map {
            $0.id == id ? Pairing(id: $0.id, name: trimmed, url: $0.url, deviceToken: $0.deviceToken, createdAt: $0.createdAt) : $0
        }
        pairingStore.save(pairings)
        showToast("已改名")
    }

    func removePairing(_ id: String) {
        pairings.removeAll { $0.id == id }
        pairingStore.save(pairings)
        let wasActive = id == activePairingId
        if wasActive {
            activePairingId = pairings.first?.id ?? ""
            prefs.activePairingId = activePairingId
            pending = []
            selectedPending = nil
        }
        showToast("已删除网关")
        if activePairingId.isEmpty { stopPolling() } else if wasActive { silentRefresh() }
    }

    private func gatewayLabel(_ url: String) -> String {
        let host = url.replacingOccurrences(of: "https://", with: "")
            .replacingOccurrences(of: "http://", with: "")
            .replacingOccurrences(of: "/+$", with: "", options: .regularExpression)
        return host.isEmpty ? "网关" : host
    }

    // MARK: - APNs

    func registerPushToken(_ token: String) {
        guard !token.isEmpty else { return }
        pushToken = token
        let targets = pairings.filter { !$0.deviceToken.isEmpty }
        Task {
            for p in targets {
                try? await BrokerClient(brokerUrl: p.url, deviceToken: p.deviceToken).registerPushToken(token)
            }
        }
    }

    // MARK: - 轮询

    func startPolling() {
        guard pollTask == nil, activePairing != nil else { return }
        pollTask = Task { [weak self] in
            while !Task.isCancelled {
                guard let self else { return }
                self.silentRefresh()
                let wait: UInt64 = self.pending.isEmpty ? 3_000_000_000 : 1_000_000_000
                try? await Task.sleep(nanoseconds: wait)
            }
        }
    }

    func stopPolling() {
        pollTask?.cancel()
        pollTask = nil
    }

    func refreshPending() { silentRefresh(showLoading: pending.isEmpty) }
    func retryPending() { silentRefresh(showLoading: true) }

    private func silentRefresh(showLoading: Bool = false) {
        guard let broker = client() else { return }
        if showLoading { loading = true; offline = false }
        Task {
            let list: [PendingRequest]
            do {
                list = try await broker.pending().filter { $0.state == "waiting" }
            } catch {
                loading = false
                offline = true
                return
            }
            applyPending(list)
        }
    }

    private func applyPending(_ list: [PendingRequest]) {
        let prev = Dictionary(uniqueKeysWithValues: pending.map { ($0.requestId, ($0.selectedItemId, $0.selectedField, $0.firstSeenAt)) })
        let prevSeen = Dictionary(uniqueKeysWithValues: pending.map { ($0.requestId, $0.firstSeenAt) })
        let currentItems = repo.items()
        var mapped = list.map { req -> PendingRequest in
            var req = req
            let kept = prev[req.requestId]
            req.firstSeenAt = prevSeen[req.requestId] ?? Date()
            let itemId = kept?.0 ?? repo.matchOrNull(req.item)?.id
            let item = itemId.flatMap { id in currentItems.first { $0.id == id } }
            req.selectedItemId = itemId
            req.selectedField = kept?.1 ?? item.map { VaultField.defaultFor(secret: $0.secret, note: $0.note) }
            return req
        }
        let currentId = selectedPending?.requestId
        let selected = mapped.first { $0.requestId == currentId } ?? mapped.first

        let prevIds = Set(pending.map { $0.requestId })
        let arrived = mapped.contains { !prevIds.contains($0.requestId) }
        let vanished = !prevIds.isEmpty && mapped.isEmpty
        if vanished {
            pending.forEach { rememberDecision($0, decision: "expired") }
            if selectedTab == 1 { timedOut = true; showToast("请求已结束") }
        }
        pending = mapped
        selectedPending = selected
        loading = false
        offline = false
        lastSync = Date().formatted(date: .omitted, time: .shortened)
        incoming = arrived && !mapped.isEmpty
        // 新请求到达自动跳待批准；UI 测试模式下禁止——自动跳会顶掉正在操作的页面
        if arrived && !mapped.isEmpty && repo.isUnlocked() && !Self.uiTesting {
            selectedTab = 1
        }
    }

    func selectPending(_ req: PendingRequest) { selectedPending = req }

    func selectItem(_ id: String) {
        guard let cur = selectedPending else { return }
        let item = items.first { $0.id == id }
        var next = cur
        next.selectedItemId = id
        next.selectedField = item.map { VaultField.defaultFor(secret: $0.secret, note: $0.note) }
        pending = pending.map { $0.requestId == cur.requestId ? next : $0 }
        selectedPending = next
    }

    func selectField(_ field: VaultField) {
        guard let cur = selectedPending else { return }
        var next = cur
        next.selectedField = field
        pending = pending.map { $0.requestId == cur.requestId ? next : $0 }
        selectedPending = next
    }

    // MARK: - 审批

    /// 批准前的一次本地记账（结果页要用 requester/item）。
    private func rememberDecision(_ req: PendingRequest, decision: String, itemName: String? = nil, via: String = "", field: String = "") {
        let entry = HistoryEntry(
            requestId: req.requestId,
            at: Int64(Date().timeIntervalSince1970 * 1000),
            item: itemName ?? req.item,
            requester: req.requester,
            purpose: req.purpose,
            target: req.target,
            decision: decision,
            via: via,
            field: field,
            tookMs: Int64(Date().timeIntervalSince(req.firstSeenAt) * 1000),
            commandLine: req.commandLine
        )
        history = historyStore.add(entry)
    }

    func expireRequest(_ id: String) {
        guard let gone = pending.first(where: { $0.requestId == id }) else { return }
        rememberDecision(gone, decision: "expired")
        pending.removeAll { $0.requestId == id }
        if selectedPending?.requestId == id { selectedPending = pending.first }
        timedOut = true
        showToast("已超时")
    }

    /// 批准入口：list / sign / 普通三种。via 只用于本地记录展示。
    func approveSelected(via: String = "faceid") {
        guard let req = selectedPending else { return }
        if ReservedItem.isListRequest(req.item) { approveListRequest(req, via: via); return }
        if req.mode == "sign" { approveSignRequest(req, via: via); return }
        approveValueRequest(req, via: via)
    }

    /// 密码兜底：密码校验过才走正常批准。
    func approveWithPassword(_ password: String) {
        Task {
            let ok = await Task.detached { self.repo.verifyPassword(password) }.value
            if ok { approveSelected(via: "password") } else { showToast("密码不对") }
        }
    }

    private func approveValueRequest(_ req: PendingRequest, via: String) {
        guard let itemId = req.selectedItemId else { showToast("请先选一条凭据"); return }
        guard repo.isUnlocked() else { showToast("库已锁上，请先解锁再批准"); return }
        guard let item = repo.items().first(where: { $0.id == itemId }) else { showToast("找不到这条凭据"); return }
        let field = req.selectedField ?? VaultField.defaultFor(secret: item.secret, note: item.note)
        let value = field == .note ? item.note : item.secret
        guard !value.isEmpty else {
            showToast(field == .note ? "这条没有备注" : "这条没有密码")
            return
        }
        guard let broker = client() else { showToast("没有可用网关，请先添加"); return }
        holdVault()
        Task {
            do {
                guard !req.sealPublicKey.isEmpty else {
                    throw EuError.badInput("这次请求没有传输公钥。用当前仓库重新编译的 ask 再发一次。")
                }
                let envelope = try BoxPayload.seal(recipientPubB64: req.sealPublicKey, requestId: req.requestId, plaintext: Data(value.utf8))
                try await broker.decide(requestId: req.requestId, decision: "approve", payload: envelope)
                if rememberAlias && !req.item.isEmpty
                    && req.item.lowercased() != item.name.lowercased()
                    && !item.aliases.map({ $0.lowercased() }).contains(req.item.lowercased()) {
                    try? repo.addAlias(id: item.id, alias: req.item)
                }
                finishApproval(req, itemName: item.name, via: via, field: field == .note ? "备注" : "密码", isList: false, isCert: false)
            } catch {
                releaseVault()
                showToast(error.localizedDescription)
            }
        }
    }

    /// 保留名请求：只把条目名和别名封回去，不放任何值。
    private func approveListRequest(_ req: PendingRequest, via: String) {
        guard repo.isUnlocked() else { showToast("库已锁上，请先解锁再批准"); return }
        guard let broker = client() else { showToast("没有可用网关，请先添加"); return }
        let items = repo.items()
        holdVault()
        Task {
            do {
                guard !req.sealPublicKey.isEmpty else {
                    throw EuError.badInput("这次请求没有传输公钥。用当前仓库重新编译的 easyGet 再发一次。")
                }
                let envelope = try BoxPayload.seal(recipientPubB64: req.sealPublicKey, requestId: req.requestId, plaintext: ItemListWire.encode(items))
                try await broker.decide(requestId: req.requestId, decision: "approve", payload: envelope)
                finishApproval(req, itemName: ReservedItem.list, via: via, field: "名单", isList: true, isCert: false)
            } catch {
                releaseVault()
                showToast(error.localizedDescription)
            }
        }
    }

    /// sign 请求：选中的条目当 CA 用，封回去的是证书本体——CA 私钥永远不会被放出。
    private func approveSignRequest(_ req: PendingRequest, via: String) {
        guard let itemId = req.selectedItemId else { showToast("先选一条当 CA 的条目"); return }
        guard repo.isUnlocked() else { showToast("库已锁上，请先解锁再批准"); return }
        guard let item = repo.items().first(where: { $0.id == itemId }) else { showToast("找不到这条凭据"); return }
        guard let ca = OpenSshKey.parseOrNull(item.note) ?? OpenSshKey.parseOrNull(item.secret) else {
            showToast("这条不是 OpenSSH ed25519 私钥，换一条当 CA")
            return
        }
        guard let broker = client() else { showToast("没有可用网关，请先添加"); return }
        holdVault()
        Task {
            do {
                guard !req.sealPublicKey.isEmpty else {
                    throw EuError.badInput("这次请求没有传输公钥。用当前仓库重新编译的 easyGet 再发一次。")
                }
                guard !req.publicKey.isEmpty, !req.sshUser.isEmpty else {
                    throw EuError.badInput("这次 sign 请求没带公钥或用户名，没法签。")
                }
                let cert = try SshCert.signUser(ca: ca, subjectLine: req.publicKey, principal: req.sshUser, requestId: req.requestId, certTtlSeconds: req.certTtl)
                let envelope = try BoxPayload.seal(recipientPubB64: req.sealPublicKey, requestId: req.requestId, plaintext: Data(cert.utf8))
                try await broker.decide(requestId: req.requestId, decision: "approve", payload: envelope)
                if rememberAlias && !req.item.isEmpty
                    && req.item.lowercased() != item.name.lowercased()
                    && !item.aliases.map({ $0.lowercased() }).contains(req.item.lowercased()) {
                    try? repo.addAlias(id: item.id, alias: req.item)
                }
                finishApproval(req, itemName: item.name, via: via, field: "证书", isList: false, isCert: true)
            } catch {
                releaseVault()
                showToast(error.localizedDescription)
            }
        }
    }

    private func finishApproval(_ req: PendingRequest, itemName: String, via: String, field: String, isList: Bool, isCert: Bool) {
        rememberDecision(req, decision: "approved", itemName: itemName, via: via, field: field)
        pending.removeAll { $0.requestId == req.requestId }
        selectedPending = pending.first
        items = repo.items()
        result = ApprovalResult(approved: true, item: itemName, requester: req.requester, isList: isList, isCert: isCert)
        timedOut = false
        releaseVault()
    }

    func denySelected() {
        guard let req = selectedPending, let broker = client() else { return }
        Task {
            try? await broker.decide(requestId: req.requestId, decision: "deny", payload: nil)
            rememberDecision(req, decision: "denied")
            pending.removeAll { $0.requestId == req.requestId }
            selectedPending = pending.first
            result = ApprovalResult(approved: false, item: req.item, requester: req.requester, isList: ReservedItem.isListRequest(req.item), isCert: req.mode == "sign")
        }
    }

    func dismissResult() {
        result = nil
        selectedTab = 0
    }

    private func holdVault() { vaultHold += 1 }
    private func releaseVault() {
        vaultHold = max(0, vaultHold - 1)
    }

    /// UI 测试模式：`-ui-testing` 启动参数或 `UI_TESTING=1` 环境变量关闭生物识别闸门与
    /// Face ID 快捷路径——真机/模拟器上生物信号不可靠，测试要确定性。
    /// 环境变量是兜底：部分 XCUITest 启动通道不传递 argv。
    static let uiTesting =
        ProcessInfo.processInfo.arguments.contains("-ui-testing")
        || ProcessInfo.processInfo.environment["UI_TESTING"] == "1"
        || ProcessInfo.processInfo.environment["XCTestConfigurationFilePath"] != nil
        || UserDefaults.standard.bool(forKey: "ui-testing")

    // MARK: - 生物识别展示（Face ID / Touch ID / Optic ID——按硬件动态出文案）

    /// 当前设备的生物识别类型。无生物识别硬件/未录入时返回 .none。
    var biometry: LABiometryType {
        let ctx = LAContext()
        _ = ctx.canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: nil)
        return ctx.biometryType
    }

    var biometryName: String {
        switch biometry {
        case .faceID: return "Face ID"
        case .touchID: return "Touch ID"
        case .opticID: return "Optic ID"
        default: return "生物识别"
        }
    }

    var biometryIcon: String {
        switch biometry {
        case .faceID: return "faceid"
        case .touchID: return "touchid"
        case .opticID: return "opticid"
        default: return "lock"
        }
    }

    /// 记录到批准历史的 via 值。
    var biometryKey: String {
        switch biometry {
        case .faceID: return "faceid"
        case .touchID: return "touchid"
        case .opticID: return "opticid"
        default: return "biometric"
        }
    }

    /// 生物认证闸门（对齐 Android confirmBiometric）：有 Face ID/Touch ID 就过，
    /// 没有生物认证的设备直接放行（密码批准是另一条路）。
    func confirmAuth(_ reason: String, completion: @escaping () -> Void) {
        if Self.uiTesting { completion(); return }
        let ctx = LAContext()
        var err: NSError?
        guard ctx.canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: &err) else {
            completion()
            return
        }
        ctx.evaluatePolicy(.deviceOwnerAuthenticationWithBiometrics,
                           localizedReason: reason) { ok, _ in
            Task { @MainActor in
                if ok { completion() }
            }
        }
    }

    // MARK: - 条目 CRUD

    /// 返回 nil 表示保存成功；否则是错误文案。
    func saveItem(editingId: String?, name: String, secret: String, note: String) -> String? {
        let trimmed = name.trimmingCharacters(in: .whitespaces)
        if trimmed.isEmpty { return "给它一个名称。" }
        if secret.isEmpty && note.isEmpty { return "密码和备注至少填一个。" }
        let taken = items.filter { $0.id != editingId }
            .flatMap { [$0.name] + $0.aliases }
            .map { $0.lowercased() }
        if taken.contains(trimmed.lowercased()) {
            return "这个名称或别名已经存在，库内必须唯一。"
        }
        do {
            if let editingId {
                try repo.update(id: editingId, name: trimmed, secret: secret, note: note)
            } else {
                try repo.add(name: trimmed, secret: secret, note: note)
            }
            items = repo.items()
            return nil
        } catch {
            return error.localizedDescription
        }
    }

    func deleteItem(_ id: String) {
        do {
            try repo.delete(id: id)
            items = repo.items()
            showToast("已删除")
        } catch { showToast(error.localizedDescription) }
    }

    func exportVault() -> Data? {
        let bytes = repo.exportBytes()
        let stamp = Date().formatted(.dateTime.month(.wide).day().locale(Locale(identifier: "zh_CN")))
        prefs.lastExportAt = stamp
        lastExportAt = stamp
        showToast("已生成密文备份 · 需要恢复码才能解开")
        return bytes
    }

    // MARK: - Bitwarden 导入

    func importNotice(_ r: BitwardenAdapter.Result) -> String {
        var parts = ["共 \(r.candidateCount) 条可导入条目"]
        if r.skippedOtherType > 0 { parts.append("\(r.skippedOtherType) 条非登录/非安全笔记已忽略") }
        if r.skippedNoSecret > 0 { parts.append("\(r.skippedNoSecret) 条密码和备注都是空的") }
        if r.skippedDuplicate > 0 { parts.append("\(r.skippedDuplicate) 条重名") }
        return parts.joined(separator: " · ")
    }

    func importResultText(_ m: ImportMergeResult) -> String {
        if m.imported == 0 && m.backfilled == 0 {
            return "没有导入：名称都已存在且备注非空（跳过 \(m.skippedName) 条）"
        }
        var parts = [String]()
        if m.imported > 0 { parts.append("已导入 \(m.imported) 条") }
        if m.backfilled > 0 { parts.append("\(m.backfilled) 条同名条目补上了备注") }
        if m.skippedName > 0 { parts.append("跳过 \(m.skippedName) 条重名") }
        if m.droppedAliases > 0 { parts.append("\(m.droppedAliases) 个用户名和别的条目重名，没记成别名") }
        return parts.joined(separator: " · ")
    }

    /// 确认导入勾选项；返回 nil 表示失败（已 toast）。
    func importEntries(_ entries: [ImportCandidate]) -> ImportMergeResult? {
        do {
            let r = try repo.importEntries(entries)
            items = repo.items()
            let stamp = Date().formatted(.dateTime.month(.wide).day().locale(Locale(identifier: "zh_CN")))
            prefs.lastImportAt = stamp
            lastImportAt = stamp
            showToast(importResultText(r))
            return r
        } catch {
            showToast(error.localizedDescription)
            return nil
        }
    }

    // MARK: - 偏好

    func setHaptics(_ v: Bool) { prefs.haptics = v; haptics = v }
    func setSound(_ v: Bool) { prefs.sound = v; sound = v }
    func setAutoCloseSeconds(_ v: Int) { prefs.autoCloseSeconds = v; autoCloseSeconds = v }

    // MARK: - Toast

    func showToast(_ msg: String) {
        toast = msg
        toastGen += 1
        let gen = toastGen
        Task {
            try? await Task.sleep(nanoseconds: 1_800_000_000)
            if toastGen == gen { toast = "" }
        }
    }
}

/// 「条目名名单」的线上形状在 Models.swift 的 ItemListWire。
