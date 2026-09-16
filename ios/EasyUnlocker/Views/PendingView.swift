import LocalAuthentication
import SwiftUI

/// 证书有效期显示成「5 分钟 / 1 小时 / 45 秒」，与安卓 fmtTtl 一致。
func fmtTtl(_ seconds: Int) -> String {
    let ttl = seconds > 0 ? seconds : SshCert.defaultTTL
    switch ttl {
    case ..<60: return "\(ttl) 秒"
    case ..<3600: return "\(ttl / 60) 分钟"
    case ..<86400: return "\(ttl / 3600) 小时"
    default: return "\(ttl / 86400) 天"
    }
}

/// 待批准（screens/pending.html）—— 审批闭环核心屏。
/// 普通请求 / #items 名单 / sign 证书 三种 body 共用一张请求卡。
struct PendingView: View {
    @EnvironmentObject var state: AppState
    @State private var now = Date()
    @State private var picking = false
    @State private var passwordMode = false
    @State private var password = ""
    @State private var bioBusy = false
    @FocusState private var pwFocused: Bool // 批准前释放，否则键盘残留盖住 tab bar
    @State private var expiredIds = Set<String>()

    private let clock = Timer.publish(every: 1, on: .main, in: .common).autoconnect()

    private var live: [PendingRequest] {
        state.pending.filter { $0.remainingSeconds(now: now) > 0 }
    }
    private var req: PendingRequest? {
        live.first ?? state.selectedPending.flatMap { $0.remainingSeconds(now: now) > 0 ? $0 : nil }
    }

    var body: some View {
        ZStack {
            if let result = state.result {
                ResultView(result: result) { state.dismissResult() }
            } else {
                pendingContent
            }

            // 生物识别浮层（设计稿 .faceid）
            if bioBusy {
                Color.black.opacity(0.45)
                    .ignoresSafeArea()
                    .transition(.opacity)
                VStack(spacing: 12) {
                    Image(systemName: state.biometryIcon)
                        .font(.system(size: 48, weight: .light))
                        .foregroundStyle(Tokens.fg)
                    Text(state.biometryName)
                        .font(.system(size: 14))
                        .foregroundStyle(Tokens.muted)
                }
                .frame(width: 148)
                .padding(.vertical, 26)
                .background(Tokens.bg)
                .clipShape(RoundedRectangle(cornerRadius: 18))
                .shadow(color: .black.opacity(0.1), radius: 24, y: 8)
                .transition(.scale(scale: 0.92).combined(with: .opacity))
            }
        }
        .background(Tokens.surface)
        .navigationTitle("待批准")
        .toolbar {
            if state.paired {
                ToolbarItem(placement: .topBarTrailing) {
                    // iOS 26 会给 toolbar 自定义视图套玻璃胶囊——纯文本，让系统那层当边框，
                    // 避免和 Chip 自己的描边叠出双框。
                    Text("网关 \(state.deviceName)")
                        .font(.system(size: 12, weight: .medium))
                        .foregroundStyle(Tokens.fg2)
                        .padding(.horizontal, 10)
                        .padding(.vertical, 3)
                }
            }
        }
        .onReceive(clock) { now = $0
            // 归零 → 记超时（每条只记一次）
            for r in state.pending where r.remainingSeconds(now: now) <= 0 && !expiredIds.contains(r.requestId) {
                expiredIds.insert(r.requestId)
                state.expireRequest(r.requestId)
            }
        }
        .onAppear { state.refreshPending() }
        .onChange(of: state.result) { _, result in
            // 批准后自动关闭：iOS 不能自杀进程，到时间回条目页。
            guard result != nil, state.autoCloseSeconds > 0 else { return }
            Task {
                try? await Task.sleep(nanoseconds: UInt64(state.autoCloseSeconds) * 1_000_000_000)
                if state.result != nil { state.dismissResult() }
            }
        }
        .confirmationDialog(req?.mode == "sign" ? "用哪条 CA 签" : "放出哪一条",
                            isPresented: $picking, titleVisibility: .visible) {
            ForEach(state.items) { item in
                Button(item.name) { state.selectItem(item.id) }
            }
            Button("取消", role: .cancel) {}
        }
    }

    // MARK: - 主体

    @ViewBuilder
    private var pendingContent: some View {
        VStack(spacing: 0) {
            ScrollView {
                if state.offline {
                    Notice(tone: .warn, title: "网关连不上", bodyText: "上次同步 \(state.lastSync.isEmpty ? "—" : state.lastSync)。下拉或点底部按钮重试。")
                }
                if state.loading && req == nil {
                    FootNote(text: "正在从网关取待批准…")
                } else if let req {
                    requestBody(req)
                } else {
                    emptyBody
                }
            }
            .refreshable { state.refreshPending() }

            if let req, !state.loading {
                actionBar(req)
            }
        }
    }

    @ViewBuilder
    private var emptyBody: some View {
        VStack(spacing: 14) {
            Image(systemName: "bell")
                .font(.system(size: 28))
                .foregroundStyle(Tokens.meta)
            if state.timedOut { Chip(text: "已超时", tone: .warn) }
            Text("没有等待你的请求。App 开着时新的请求会自动出现在这里。")
                .font(.system(size: 12))
                .foregroundStyle(Tokens.muted)
                .multilineTextAlignment(.center)
                .padding(.horizontal, 40)
            if !state.paired {
                Text("还没有配对的网关：到「设置 → 网关」加一台，CLI 才能发请求过来。")
                    .font(.system(size: 12))
                    .foregroundStyle(Tokens.muted)
                    .multilineTextAlignment(.center)
                    .padding(.horizontal, 40)
            }
            if state.offline {
                Button("重试") { state.retryPending() }
                    .buttonStyle(TintedButtonStyle())
                    .padding(.horizontal, 80)
            }
        }
        .frame(maxWidth: .infinity)
        .padding(.top, 56)
    }

    // MARK: - 请求卡 + 选项

    @ViewBuilder
    private func requestBody(_ req: PendingRequest) -> some View {
        let listReq = ReservedItem.isListRequest(req.item)
        let signReq = req.mode == "sign"

        RequestCard(
            requester: req.requester,
            subtitle: listReq ? "想看看有哪些条目名" : (signReq ? "想签发一张 SSH 证书" : "正在请求一条凭据"),
            trailing: { TtlRing(remain: req.remainingSeconds(now: now), total: max(1, req.ttl)) }
        ) {
            VStack(alignment: .leading, spacing: 14) {
                Text(listReq ? "条目名名单" : (signReq ? "SSH 证书" : req.item))
                    .font(.mono(30, weight: .semibold))
                    .tracking(-0.9)
                    .foregroundStyle(Tokens.fg)
                    .fixedSize(horizontal: false, vertical: true)

                if !req.purpose.isEmpty {
                    ReqField(label: "用途") {
                        Text(req.purpose)
                            .font(.system(size: 14))
                            .foregroundStyle(Tokens.fg2)
                            .lineSpacing(3)
                    }
                }
                if signReq {
                    ReqField(label: "登录用户") {
                        Text(req.sshUser).font(.mono(13)).foregroundStyle(Tokens.fg2)
                    }
                    ReqField(label: "证书有效") {
                        Text("\(fmtTtl(req.certTtl))（只在登录握手时校验）")
                            .font(.system(size: 14)).foregroundStyle(Tokens.fg2)
                    }
                    ReqField(label: "身份公钥") {
                        Text((try? OpenSshKey.fingerprintOf(req.publicKey)) ?? "解析失败")
                            .font(.mono(12)).foregroundStyle(Tokens.fg2)
                    }
                }
                ReqField(label: "命令") {
                    CommandBlock(text: req.commandLine)
                }
            }
        }

        if listReq {
            listBody(req)
        } else if signReq {
            signBody(req)
        } else {
            valueBody(req)
        }
    }

    /// 「放出这一条」选择行（普通与 sign 共用；label/匹配语义不同）。
    @ViewBuilder
    private func pickRow(_ req: PendingRequest, isCA: Bool) -> some View {
        let chosen = state.items.first { $0.id == req.selectedItemId }
        let exact = chosen.map { namesOf($0).contains(req.item.lowercased()) } ?? false
        KVLabel(isCA ? "用这条 CA 签" : "放出这一条") {
            Text(chosen == nil ? "需要你指定" : (exact ? "完全匹配" : "已手选"))
                .font(.mono(11))
                .foregroundStyle(chosen == nil ? Color(red: 0.55, green: 0.42, blue: 0.02) : (exact ? Tokens.accentActive : Tokens.muted))
        }
        IGroup {
            Button { picking = true } label: {
                HStack {
                    Text(chosen?.name ?? (isCA ? "选择当 CA 的条目" : "没有完全匹配的条目"))
                        .font(.mono(14, weight: .medium))
                        .foregroundStyle(chosen == nil ? Color(red: 0.55, green: 0.42, blue: 0.02) : Tokens.fg)
                        .lineLimit(1)
                    Spacer()
                    Text(chosen == nil ? "选择" : "改")
                        .font(.system(size: 14))
                        .foregroundStyle(Tokens.accent)
                    Image(systemName: "chevron.right")
                        .font(.system(size: 12, weight: .semibold))
                        .foregroundStyle(Tokens.meta.opacity(0.75))
                }
                .padding(.horizontal, 16)
                .frame(minHeight: 48)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
        }
    }

    /// 别名记忆（手选了不同条目时出现）。
    @ViewBuilder
    private func aliasRow(_ req: PendingRequest) -> some View {
        let chosen = state.items.first { $0.id == req.selectedItemId }
        if let chosen, chosen.name.lowercased() != req.item.lowercased() {
            IGroup {
                HStack {
                    Text("把 \(req.item) 记成这个名字的别名")
                        .font(.system(size: 14))
                        .foregroundStyle(Tokens.fg)
                        .fixedSize(horizontal: false, vertical: true)
                    Spacer()
                    Toggle("", isOn: $state.rememberAlias).labelsHidden()
                }
                .padding(.horizontal, 16)
                .frame(minHeight: 48)
            }
            .padding(.top, 10)
        }
    }

    // MARK: 普通请求：选条目 + 选栏 + 预览 + 落盘提示

    @ViewBuilder
    private func valueBody(_ req: PendingRequest) -> some View {
        pickRow(req, isCA: false)
        aliasRow(req)

        let chosen = state.items.first { $0.id == req.selectedItemId }
        if let chosen {
            KVLabel("放出")
            Picker("放出", selection: Binding(
                get: { req.selectedField ?? VaultField.defaultFor(secret: chosen.secret, note: chosen.note) },
                set: { state.selectField($0) }
            )) {
                Text("密码").tag(VaultField.password)
                Text("备注").tag(VaultField.note)
            }
            .pickerStyle(.segmented)
            .padding(.horizontal, 16)

            let field = req.selectedField ?? VaultField.defaultFor(secret: chosen.secret, note: chosen.note)
            let value = field == .note ? chosen.note : chosen.secret
            KVCard {
                Text(value.isEmpty ? "--" : String(value.prefix(240)))
                    .font(.mono(13))
                    .lineSpacing(4)
                    .foregroundStyle(value.isEmpty ? Tokens.meta : Tokens.fg)
                    .lineLimit(4)
                    .truncationMode(.tail)
                    .fixedSize(horizontal: false, vertical: true)
            }
            .padding(.top, 10)
            if !value.isEmpty {
                Text(previewMeta(value))
                    .font(.system(size: 12))
                    .foregroundStyle(Tokens.muted)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.horizontal, 20)
                    .padding(.top, 6)
            }
        }
        if state.items.isEmpty {
            FootNote(text: "库里还没有条目，先去添加一条再回来。", alignLeft: true)
        }
        DeliveryNotice(delivery: req.delivery, target: req.target)
    }

    // MARK: #items 名单请求

    @ViewBuilder
    private func listBody(_ req: PendingRequest) -> some View {
        KVLabel("会告诉它什么") {
            Text("\(state.items.count) 个名字")
                .font(.mono(11))
                .foregroundStyle(Tokens.accentActive)
        }
        KVCard {
            if state.items.isEmpty {
                Text("读不到条目：库可能是空的，也可能还没解锁。")
                    .font(.mono(12))
                    .foregroundStyle(Tokens.muted)
            } else {
                VStack(alignment: .leading, spacing: 6) {
                    ForEach(state.items.prefix(8)) { item in
                        VStack(alignment: .leading, spacing: 1) {
                            Text(item.name).font(.mono(13)).foregroundStyle(Tokens.fg)
                            if !item.aliases.isEmpty {
                                Text("别名 \(item.aliases.joined(separator: ", "))")
                                    .font(.mono(11)).foregroundStyle(Tokens.muted)
                            }
                        }
                    }
                    if state.items.count > 8 {
                        Text("…还有 \(state.items.count - 8) 条")
                            .font(.system(size: 11))
                            .foregroundStyle(Tokens.muted)
                            .padding(.top, 4)
                    }
                }
            }
        }
        Notice(tone: .info, title: "只给名字，不给任何值",
               bodyText: "这台机器会知道有哪几条，但每条取用仍然要你单独批准一次；批准名单本身放不出任何密钥。")
    }

    // MARK: sign 证书请求

    @ViewBuilder
    private func signBody(_ req: PendingRequest) -> some View {
        pickRow(req, isCA: true)
        aliasRow(req)
        let chosen = state.items.first { $0.id == req.selectedItemId }
        if let chosen, caKeyOf(chosen) == nil {
            FootNote(text: "这条的密码/备注里没有 OpenSSH ed25519 私钥，当不了 CA。", alignLeft: true)
        }
        if state.items.isEmpty {
            FootNote(text: "库里还没有条目，先去添加一条 CA 私钥再回来。", alignLeft: true)
        }
        Notice(tone: .info, title: "CA 私钥不出手机",
               bodyText: "对方拿到的只是一张 \(fmtTtl(req.certTtl))内可登录 \(req.sshUser) 的证书；CA 私钥本身不会被放出。证书只在登录那一刻校验，已建立的会话不受影响。")
    }

    // MARK: - 底部操作

    @ViewBuilder
    private func actionBar(_ req: PendingRequest) -> some View {
        let listReq = ReservedItem.isListRequest(req.item)
        let signReq = req.mode == "sign"
        let chosen = state.items.first { $0.id == req.selectedItemId }
        let ready: Bool = {
            if listReq { return !state.items.isEmpty }
            if signReq { return chosen.flatMap(caKeyOf) != nil }
            guard let chosen else { return false }
            let field = req.selectedField ?? VaultField.defaultFor(secret: chosen.secret, note: chosen.note)
            return field == .note ? !chosen.note.isEmpty : !chosen.secret.isEmpty
        }()

        VStack(spacing: 10) {
            Button {
                if bioAvailable() {
                    withAnimation(.easeOut(duration: 0.22)) { bioBusy = true }
                    runBio {
                        bioBusy = false
                        if $0 { state.approveSelected(via: state.biometryKey) }
                    }
                } else {
                    state.approveSelected(via: state.biometryKey)
                }
            } label: {
                Label(approveLabel(req), systemImage: state.biometryIcon)
            }
            .buttonStyle(PillButtonStyle(disabled: !ready))
            .disabled(!ready)
            .accessibilityIdentifier("pending.approve.\(req.id)")

            if passwordMode {
                HStack(spacing: 8) {
                    SecureField("输入解锁密码", text: $password)
                        .focused($pwFocused)
                        .accessibilityIdentifier("approve.pw")
                        .textContentType(.password)
                        .padding(.horizontal, 14)
                        .frame(minHeight: 44)
                        .background(Tokens.bg)
                        .overlay(RoundedRectangle(cornerRadius: 12).stroke(Tokens.border))
                        .clipShape(RoundedRectangle(cornerRadius: 12))
                    Button("确认批准") {
                        dismissKeyboard()
                        state.approveWithPassword(password)
                        password = ""
                    }
                    .font(.system(size: 15, weight: .medium))
                    .foregroundStyle(Tokens.accent)
                    .padding(.horizontal, 18)
                    .frame(minHeight: 44)
                    .background(Tokens.bg)
                    .overlay(RoundedRectangle(cornerRadius: 12).stroke(Tokens.border))
                    .clipShape(RoundedRectangle(cornerRadius: 12))
                }
            }

            HStack(spacing: 10) {
                if state.hasPassword && !passwordMode {
                    Button {
                        passwordMode = true
                    } label: {
                        Label("用密码批准", systemImage: "lock")
                    }
                    .buttonStyle(TintedButtonStyle(disabled: !ready))
                    .disabled(!ready)
                }
                Button("拒绝") { state.denySelected() }
                    .buttonStyle(TintedButtonStyle(danger: true))
                    .accessibilityIdentifier("pending.deny.\(req.id)")
            }
        }
        .padding(.horizontal, 16)
        .padding(.top, 14)
        .padding(.bottom, 16)
        .background(Tokens.surface)
    }

    private func approveLabel(_ req: PendingRequest) -> String {
        if ReservedItem.isListRequest(req.item) { return "允许列出（\(state.items.count) 条）" }
        if req.mode == "sign" { return "签发证书（\(fmtTtl(req.certTtl))）" }
        return req.delivery == Delivery.ephemeral.rawValue ? "放出（不落盘）" : "确认写入磁盘"
    }

    private func bioAvailable() -> Bool {
        var err: NSError?
        return LAContext().canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: &err)
    }

    private func runBio(_ done: @escaping (Bool) -> Void) {
        LAContext().evaluatePolicy(.deviceOwnerAuthenticationWithBiometrics,
                                   localizedReason: "批准这次请求") { ok, _ in
            Task { @MainActor in done(ok) }
        }
    }

    private func previewMeta(_ value: String) -> String {
        var s = "共 \(value.count) 字符"
        let lines = value.filter { $0 == "\n" }.count + 1
        if lines > 1 { s += " · \(lines) 行" }
        if value.count > 240 { s += " · 只显示开头" }
        return s
    }

    private func caKeyOf(_ item: VaultItem) -> OpenSshKey.PrivateKey? {
        OpenSshKey.parseOrNull(item.note) ?? OpenSshKey.parseOrNull(item.secret)
    }

    private func namesOf(_ item: VaultItem) -> [String] {
        [item.name.lowercased()] + item.aliases.map { $0.lowercased() }
    }
}

/// .delivery-notice — 不落盘 / 会落盘提示。
struct DeliveryNotice: View {
    let delivery: String
    let target: String
    var body: some View {
        if delivery == Delivery.ephemeral.rawValue {
            Notice(tone: .info, title: "不落盘，用完即焚",
                   bodyText: "值只交给请求方进程（环境变量 / 匿名 fd），不写文件；进程退出即消失。")
        } else {
            Notice(tone: .warn, title: "会落盘",
                   bodyText: "\(target.isEmpty ? "请求方给的路径" : target) 会被写入文件，内容留在那台机器上，直到你手动删掉。")
        }
    }
}

/// 结果态：已放行 / 已拒绝。
struct ResultView: View {
    let result: ApprovalResult
    let onBack: () -> Void

    var body: some View {
        VStack(spacing: 0) {
            Spacer()
            ZStack {
                Circle()
                    .fill((result.approved ? Tokens.success : Tokens.danger).opacity(result.approved ? 0.10 : 0.08))
                    .frame(width: 96, height: 96)
                Image(systemName: result.approved ? "checkmark" : "xmark")
                    .font(.system(size: 40, weight: .medium))
                    .foregroundStyle(result.approved ? Tokens.success : Tokens.danger)
            }
            .padding(.bottom, 22)
            Text(result.approved ? "已放行" : "已拒绝")
                .font(.system(size: 34, weight: .bold))
                .tracking(-0.5)
            Text(result.approved
                 ? "\(result.isList ? "名单" : result.item) → \(result.requester)"
                 : "\(result.item) ✕ \(result.requester)")
                .font(.mono(13))
                .foregroundStyle(Tokens.muted)
                .padding(.top, 10)
            Text(result.approved
                 ? (result.isList
                    ? "只有条目名，没有任何值离开手机。"
                    : (result.isCert
                       ? "签出去的是一张短时证书，CA 私钥没离开手机。"
                       : "值已用本次 ask 的临时公钥密封，Broker 只转发、解不开。"))
                 : "终端会以非零退出，Agent 不会拿到任何值。")
                .font(.system(size: 14))
                .foregroundStyle(Tokens.muted)
                .lineSpacing(4)
                .multilineTextAlignment(.center)
                .frame(maxWidth: 300)
                .padding(.top, 12)
            Spacer()
            Button("返回条目") { onBack() }
                .buttonStyle(TintedButtonStyle())
                .padding(.horizontal, 16)
                .padding(.bottom, 16)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(Tokens.surface)
    }
}
