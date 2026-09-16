import SwiftUI
import UIKit

/// 网关列表：配对/切换/改名（本地标签）/删除。
struct GatewaysView: View {
    @EnvironmentObject var state: AppState
    @State private var adding = false
    @State private var removing: Pairing? = nil
    @State private var renaming: Pairing? = nil
    @State private var renameText = ""

    var body: some View {
        ScrollView {
            if state.pairings.isEmpty {
                FootNote(text: "还没有网关。配一台，CLI 才能把请求推到这台手机。")
            }
            IGroup {
                ForEach(Array(state.pairings.enumerated()), id: \.element.id) { idx, p in
                    if idx > 0 { RowSeparator() }
                    Button { state.switchPairing(p.id) } label: {
                        HStack {
                            VStack(alignment: .leading, spacing: 1) {
                                Text(p.name).font(.system(size: 15, weight: .medium)).foregroundStyle(Tokens.fg)
                                Text(p.url).font(.mono(12)).foregroundStyle(Tokens.muted).lineLimit(1)
                            }
                            Spacer()
                            if p.id == state.activePairingId {
                                Chip(text: "当前", tone: .info)
                            }
                            Button { renaming = p; renameText = p.name } label: {
                                Image(systemName: "square.and.pencil")
                                    .font(.system(size: 15))
                                    .foregroundStyle(Tokens.muted)
                            }
                            .buttonStyle(.plain)
                            Button(role: .destructive) { removing = p } label: {
                                Image(systemName: "trash")
                                    .font(.system(size: 15))
                                    .foregroundStyle(Tokens.danger)
                            }
                            .buttonStyle(.plain)
                        }
                        .padding(.horizontal, 16)
                        .frame(minHeight: 48)
                        .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                }
            }
            IGroup {
                Button { adding = true } label: {
                    HStack {
                        Text("添加网关").font(.system(size: 15)).foregroundStyle(Tokens.accent)
                        Spacer()
                    }
                    .padding(.horizontal, 16)
                    .frame(minHeight: 48)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
            }
            .padding(.top, 10)
            FootNote(text: "网关只决定「往哪儿发待批准 / 决定」，跟保险库没有关系；切换网关不会动条目。")
        }
        .background(Tokens.surface)
        .navigationTitle("网关")
        .navigationBarTitleDisplayMode(.inline)
        .sheet(isPresented: $adding) {
            NavigationStack { PairGatewayView() }
        }
        .confirmationDialog("删除网关 \(removing?.name ?? "")？", isPresented: Binding(
            get: { removing != nil }, set: { if !$0 { removing = nil } }
        ), titleVisibility: .visible) {
            Button("删除", role: .destructive) {
                if let r = removing { state.removePairing(r.id) }
                removing = nil
            }
            Button("取消", role: .cancel) { removing = nil }
        }
        .alert("网关改名", isPresented: Binding(
            get: { renaming != nil }, set: { if !$0 { renaming = nil } })) {
            TextField("网关名", text: $renameText)
            Button("保存") {
                if let r = renaming { state.renamePairing(r.id, name: renameText) }
                renaming = nil
            }
            Button("取消", role: .cancel) { renaming = nil }
        }
    }
}

/// 配对表单：Broker 地址 + pairing token + 显示名。
struct PairGatewayView: View {
    @EnvironmentObject var state: AppState
    @Environment(\.dismiss) private var dismiss
    @State private var url = ""
    @State private var token = ""
    @State private var name = ""
    @State private var error = ""
    @State private var busy = false
    @FocusState private var focused: Focused? // sheet 消失前必须释放，否则键盘残留盖 tab bar
    private enum Focused { case url, token, name }

    var body: some View {
        ScrollView {
            FormLabel("Broker 地址")
            IGroup {
                TextField("https://broker.example.com", text: $url)
                    .focused($focused, equals: .url)
                    .accessibilityIdentifier("pair.url")
                    .font(.mono(13))
                    .autocorrectionDisabled()
                    .textInputAutocapitalization(.never)
                    .keyboardType(.URL)
                    .padding(.horizontal, 16)
                    .frame(minHeight: 46)
            }
            FormLabel("Pairing token").padding(.top, 18)
            IGroup {
                SecureField("easyGet pair 给的 token", text: $token)
                    .focused($focused, equals: .token)
                    .accessibilityIdentifier("pair.token")
                    .font(.mono(13))
                    .autocorrectionDisabled()
                    .textInputAutocapitalization(.never)
                    .padding(.horizontal, 16)
                    .frame(minHeight: 46)
            }
            FormLabel("显示名（可空）").padding(.top, 18)
            IGroup {
                TextField("默认取地址里的 host", text: $name)
                    .focused($focused, equals: .name)
                    .accessibilityIdentifier("pair.name")
                    .font(.system(size: 15))
                    .autocorrectionDisabled()
                    .padding(.horizontal, 16)
                    .frame(minHeight: 46)
            }
            if !error.isEmpty {
                Text(error)
                    .font(.system(size: 14, weight: .medium))
                    .foregroundStyle(Tokens.danger)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.horizontal, 32)
                    .padding(.top, 10)
            }
            FootNote(text: "token 只用来换这台设备的 device_token，存在本机，不进保险库。")
        }
        .background(Tokens.surface)
        .navigationTitle("添加网关")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .cancellationAction) {
                Button("取消") { dismissKeyboard(); dismiss() }
            }
            ToolbarItem(placement: .confirmationAction) {
                Button(busy ? "配对中…" : "配对") {
                    dismissKeyboard()
                    busy = true
                    Task {
                        if let err = await state.pair(url: url, pairingToken: token, name: name) {
                            error = err
                            busy = false
                        } else {
                            dismiss()
                        }
                    }
                }
                .fontWeight(.semibold)
                .disabled(busy)
            }
        }
    }
}

/// 设备管理：配对码生成 + 本机卡（续期/改名/登出）+ 外部设备行（改名/撤销），对齐安卓。
struct DevicesView: View {
    @EnvironmentObject var state: AppState
    @State private var devices: [PairedDevice] = []
    @State private var loading = true
    @State private var error = ""
    @State private var pairCode = ""
    @State private var pairCodeExpiresAt: Int64 = 0
    @State private var renaming: PairedDevice? = nil
    @State private var renameText = ""
    @State private var revoking: PairedDevice? = nil
    @State private var logoutArmed = false
    @State private var now = Date()
    private let clock = Timer.publish(every: 1, on: .main, in: .common).autoconnect()

    private var client: BrokerClient? {
        state.activePairing.map { BrokerClient(brokerUrl: $0.url, deviceToken: $0.deviceToken) }
    }
    private var selfDevice: PairedDevice? { devices.first { $0.current } }
    private var externalDevices: [PairedDevice] { devices.filter { !$0.current } }
    private var codeLeft: Int {
        max(0, Int((pairCodeExpiresAt - Int64(now.timeIntervalSince1970 * 1000)) / 1000))
    }

    private static func isoDate(_ iso: String) -> String {
        if let d = ISO8601DateFormatter().date(from: iso) {
            let f = DateFormatter()
            f.dateFormat = "yyyy-MM-dd"
            return f.string(from: d)
        }
        return String(iso.prefix(10))
    }

    private static func deviceMeta(_ d: PairedDevice) -> String {
        let used = d.lastUsedAt.isEmpty ? "从未使用" : "最后使用 \(isoDate(d.lastUsedAt))"
        return "\(used) · 有效期至 \(isoDate(d.expiresAt))"
    }

    var body: some View {
        ScrollView {
            // 配对码卡
            if !pairCode.isEmpty && codeLeft > 0 {
                IGroup {
                    VStack(alignment: .leading, spacing: 8) {
                        Text("一次性配对码")
                            .font(.mono(11))
                            .tracking(1.1)
                            .foregroundStyle(Tokens.muted)
                        Text(pairCode)
                            .font(.mono(26, weight: .semibold))
                            .foregroundStyle(Tokens.fg)
                            .textSelection(.enabled)
                            .accessibilityIdentifier("device.paircode")
                        Text("在另一台机器上跑：")
                            .font(.system(size: 12))
                            .foregroundStyle(Tokens.muted)
                        Text("easyGet pair --broker \(state.activePairing?.url ?? "") --code \(pairCode)")
                            .font(.mono(12))
                            .foregroundStyle(Tokens.fg)
                            .textSelection(.enabled)
                        HStack {
                            Text("\(codeLeft) 秒后过期")
                                .font(.system(size: 12))
                                .foregroundStyle(Tokens.muted)
                            Spacer()
                            Button {
                                UIPasteboard.general.string = pairCode
                                state.showToast("配对码已复制")
                            } label: {
                                Label("复制", systemImage: "doc.on.doc")
                                    .font(.system(size: 12, weight: .medium))
                                    .foregroundStyle(Tokens.accent)
                            }
                        }
                    }
                    .padding(.horizontal, 16)
                    .padding(.vertical, 14)
                    .frame(maxWidth: .infinity, alignment: .leading)
                }
                .padding(.top, 4)
            }

            if loading {
                FootNote(text: "正在从网关取设备列表…")
            } else if !error.isEmpty {
                Notice(tone: .warn, title: "取不到设备列表", bodyText: error)
            } else {
                // 本机（审批端）
                if let d = selfDevice {
                    IGroup {
                        VStack(alignment: .leading, spacing: 0) {
                            HStack(spacing: 6) {
                                Text(d.name.isEmpty ? "这台手机" : d.name)
                                    .font(.mono(15, weight: .medium))
                                    .foregroundStyle(Tokens.fg)
                                Spacer()
                                Chip(text: "审批端", tone: .ok)
                            }
                            Text(Self.deviceMeta(d))
                                .font(.system(size: 11))
                                .foregroundStyle(Tokens.muted)
                                .padding(.top, 6)
                            HStack(spacing: 8) {
                                devicePill("续期 180 天") { renew() }
                                devicePill("改名") { renaming = d; renameText = d.name }
                                devicePill(logoutArmed ? "再点一次确认登出" : "登出这台", danger: logoutArmed) {
                                    if logoutArmed { revoke(d) } else { logoutArmed = true }
                                }
                            }
                            .padding(.top, 12)
                        }
                        .padding(.horizontal, 16)
                        .padding(.vertical, 14)
                        .frame(maxWidth: .infinity, alignment: .leading)
                    }
                    .padding(.top, pairCode.isEmpty || codeLeft <= 0 ? 4 : 10)
                }

                // 外部设备（请求端）
                if !externalDevices.isEmpty {
                    KVLabel("外部设备")
                    IGroup {
                        ForEach(Array(externalDevices.enumerated()), id: \.element.id) { idx, d in
                            if idx > 0 { RowSeparator() }
                            HStack(spacing: 10) {
                                VStack(alignment: .leading, spacing: 1) {
                                    Text(d.name.isEmpty ? "未命名设备" : d.name)
                                        .font(.mono(15, weight: .medium))
                                        .foregroundStyle(Tokens.fg)
                                    Text(Self.deviceMeta(d))
                                        .font(.system(size: 11))
                                        .foregroundStyle(Tokens.muted)
                                }
                                Spacer()
                                Button("改名") { renaming = d; renameText = d.name }
                                    .font(.mono(12, weight: .medium))
                                    .foregroundStyle(Tokens.accentActive)
                                Button("撤销") { revoking = d }
                                    .font(.mono(12, weight: .medium))
                                    .foregroundStyle(Tokens.danger)
                            }
                            .padding(.horizontal, 16)
                            .frame(minHeight: 52)
                        }
                    }
                } else if pairCode.isEmpty {
                    FootNote(text: "还没有外部设备。生成配对码，在新机器上运行 easyGet pair。")
                }
            }

            // 生成配对码入口
            if !loading && error.isEmpty {
                if !pairCode.isEmpty && codeLeft <= 0 {
                    Button("重新生成配对码") { createCode() }
                        .buttonStyle(PillButtonStyle())
                        .containerRelativeFrame(.horizontal) { l, _ in l * 0.8 }
                        .padding(.top, 14)
                } else if pairCode.isEmpty {
                    Button("添加设备（生成配对码）") { createCode() }
                        .buttonStyle(PillButtonStyle())
                        .containerRelativeFrame(.horizontal) { l, _ in l * 0.8 }
                        .padding(.top, 14)
                } else {
                    Button("收起配对码") { pairCode = "" }
                        .buttonStyle(QuietButtonStyle())
                }
            }
        }
        .background(Tokens.surface)
        .navigationTitle("设备")
        .navigationBarTitleDisplayMode(.inline)
        .task { await reload() }
        .onReceive(clock) { now = $0 }
        .alert("设备改名", isPresented: Binding(get: { renaming != nil }, set: { if !$0 { renaming = nil } })) {
            TextField("设备名", text: $renameText)
            Button("保存") { if let d = renaming { rename(d) } ; renaming = nil }
            Button("取消", role: .cancel) { renaming = nil }
        }
        .confirmationDialog("撤销设备 \(revoking?.name ?? "")？", isPresented: Binding(
            get: { revoking != nil }, set: { if !$0 { revoking = nil } }), titleVisibility: .visible) {
            Button("撤销", role: .destructive) { if let d = revoking { revoke(d) } ; revoking = nil }
            Button("取消", role: .cancel) { revoking = nil }
        } message: {
            Text("撤销后那台机器要重新配对才能再发请求。")
        }
    }

    private func devicePill(_ title: String, danger: Bool = false, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Text(title)
                .font(.mono(11, weight: .medium))
                .foregroundStyle(danger ? .white : (title.contains("登出") ? Tokens.danger : Tokens.accentActive))
                .padding(.horizontal, 12)
                .padding(.vertical, 6)
                .background(danger ? Tokens.danger : Tokens.bg)
                .overlay(Capsule().stroke(danger ? Tokens.danger : Tokens.border, lineWidth: 1))
                .clipShape(Capsule())
        }
        .buttonStyle(.plain)
    }

    private func reload() async {
        guard let c = client else { loading = false; return }
        do {
            devices = try await c.devices()
            error = ""
        } catch {
            self.error = error.localizedDescription
        }
        loading = false
    }

    private func createCode() {
        guard let c = client else { return }
        Task {
            do {
                let (code, exp) = try await c.createPairCode()
                pairCode = code
                pairCodeExpiresAt = exp
            } catch {
                state.showToast(error.localizedDescription)
            }
        }
    }

    private func renew() {
        guard let c = client else { return }
        Task {
            do {
                _ = try await c.renewDevice()
                state.showToast("已续期 180 天")
                await reload()
            } catch {
                state.showToast(error.localizedDescription)
            }
        }
    }

    private func rename(_ d: PairedDevice) {
        guard let c = client, !renameText.trimmingCharacters(in: .whitespaces).isEmpty else { return }
        Task {
            do {
                try await c.renameDevice(id: d.id, name: renameText.trimmingCharacters(in: .whitespaces))
                state.showToast("已改名")
                await reload()
            } catch {
                state.showToast(error.localizedDescription)
            }
        }
    }

    private func revoke(_ d: PairedDevice) {
        guard let c = client else { return }
        Task {
            do {
                try await c.revokeDevice(id: d.id)
                if d.current, let p = state.activePairing {
                    state.removePairing(p.id)
                } else {
                    state.showToast("已撤销")
                    await reload()
                }
            } catch {
                state.showToast(error.localizedDescription)
            }
        }
    }
}

/// 解锁密码页：设置 / 更换 / 清除。
struct PasswordView: View {
    @EnvironmentObject var state: AppState
    @State private var pw1 = ""
    @State private var pw2 = ""
    @State private var error = ""
    @State private var confirmClear = false
    @FocusState private var focused: Bool // 保存后释放，返回时键盘不残留

    var body: some View {
        ScrollView {
            IGroup {
                SecureField("新密码", text: $pw1)
                    .focused($focused)
                    .accessibilityIdentifier("pw.new")
                    .textContentType(.newPassword)
                    .padding(.horizontal, 16)
                    .frame(minHeight: 46)
                RowSeparator()
                SecureField("再输一遍", text: $pw2)
                    .focused($focused)
                    .accessibilityIdentifier("pw.repeat")
                    .textContentType(.newPassword)
                    .padding(.horizontal, 16)
                    .frame(minHeight: 46)
            }
            .padding(.top, 4)
            if !error.isEmpty {
                Text(error)
                    .font(.system(size: 14, weight: .medium))
                    .foregroundStyle(Tokens.danger)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.horizontal, 32)
                    .padding(.top, 10)
            }
            Button("保存") { save() }
                .buttonStyle(PillButtonStyle())
                .padding(.horizontal, 16)
                .padding(.top, 14)
            if state.hasPassword {
                Button("清除解锁密码", role: .destructive) { confirmClear = true }
                    .buttonStyle(QuietButtonStyle(danger: true))
                    .padding(.horizontal, 16)
            }
            FootNote(text: "密码只在本机当批准的兜底认证；换掉保险库后旧密码自动作废。")
        }
        .background(Tokens.surface)
        .navigationTitle("解锁密码")
        .navigationBarTitleDisplayMode(.inline)
        .confirmationDialog("清除解锁密码？批准时就只能用 \(state.biometryName) 了。",
                            isPresented: $confirmClear, titleVisibility: .visible) {
            Button("清除", role: .destructive) { state.clearPassword() }
            Button("取消", role: .cancel) {}
        }
    }

    private func save() {
        if pw1.isEmpty { error = "密码不能为空"; return }
        if pw1 != pw2 { error = "两遍输的不一样"; return }
        state.setPassword(pw1)
        pw1 = ""; pw2 = ""; error = ""
        dismissKeyboard()
    }
}

/// 批准后自动关闭：iOS 不能自杀进程，等价行为是倒计时结束回条目页。
struct AutoCloseView: View {
    @EnvironmentObject var state: AppState
    private let options = [0, 3, 5, 10, 30]

    var body: some View {
        ScrollView {
            IGroup {
                ForEach(Array(options.enumerated()), id: \.element) { idx, s in
                    if idx > 0 { RowSeparator() }
                    Button { state.setAutoCloseSeconds(s) } label: {
                        HStack {
                            Text(s == 0 ? "不自动关闭" : "\(s) 秒后回条目页")
                                .font(.system(size: 15, weight: .medium))
                                .foregroundStyle(Tokens.fg)
                            Spacer()
                            if state.autoCloseSeconds == s {
                                Image(systemName: "checkmark")
                                    .foregroundStyle(Tokens.accent)
                            }
                        }
                        .padding(.horizontal, 16)
                        .frame(minHeight: 48)
                        .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                }
            }
            .padding(.top, 4)
            FootNote(text: "iOS 不允许应用自己退出；到时间会从结果页自动回到「条目」。")
        }
        .background(Tokens.surface)
        .navigationTitle("批准后自动关闭")
        .navigationBarTitleDisplayMode(.inline)
    }
}
