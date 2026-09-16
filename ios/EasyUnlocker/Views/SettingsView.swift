import SwiftUI
import UniformTypeIdentifiers

/// 设置（screens/settings.html）：三组 inset-grouped 列表 + iOS 开关 + APNs + 恢复码。
struct SettingsView: View {
    @EnvironmentObject var state: AppState
    @State private var exporting = false
    @State private var exportDoc: VaultFileDocument? = nil
    @State private var pickingImport = false
    @State private var importSession: ImportSession? = nil

    /// sheet 的 item 需要 Identifiable——数组不行，包一层。
    private struct ImportSession: Identifiable {
        let id = UUID()
        let candidates: [ImportCandidate]
        let notice: String
    }

    var body: some View {
        ScrollView {
            // ─── 连接与账户 ───
            IGroup {
                NavigationLink {
                    GatewaysView().toolbar(.hidden, for: .tabBar)
                } label: {
                    SettingRow(title: "网关", value: gatewayValue, chevron: true)
                }
                RowSeparator()
                NavigationLink {
                    DevicesView().toolbar(.hidden, for: .tabBar)
                } label: {
                    SettingRow(title: "设备", value: "", chevron: true)
                }
                RowSeparator()
                NavigationLink {
                    PasswordView().toolbar(.hidden, for: .tabBar)
                } label: {
                    SettingRow(title: "解锁密码", value: state.hasPassword ? "已设置" : "未设置", chevron: true)
                }
                RowSeparator()
                NavigationLink {
                    HistoryView().toolbar(.hidden, for: .tabBar)
                } label: {
                    SettingRow(title: "批准记录", value: "\(state.history.count) 条", chevron: true)
                }
            }
            .padding(.top, 4)

            // ─── 备份与行为 ───
            IGroup {
                NavigationLink {
                    AutoCloseView().toolbar(.hidden, for: .tabBar)
                } label: {
                    SettingRow(title: "批准后自动关闭", value: state.autoCloseSeconds == 0 ? "不关闭" : "\(state.autoCloseSeconds) 秒", chevron: true)
                }
                RowSeparator()
                Button { exportBackup() } label: {
                    SettingRow(title: "导出密文备份", value: state.lastExportAt.isEmpty ? "" : "上次 \(state.lastExportAt)", chevron: true)
                }
                .buttonStyle(.plain)
                RowSeparator()
                Button { pickingImport = true } label: {
                    SettingRow(title: "导入 Bitwarden 备份",
                               value: state.lastImportAt.isEmpty ? "仅支持 JSON" : "仅支持 JSON · 上次 \(state.lastImportAt)",
                               chevron: true)
                }
                .buttonStyle(.plain)
            }
            .padding(.top, 22)

            // ─── 反馈与推送 ───
            IGroup {
                HStack {
                    Text("放行时震动").font(.system(size: 15, weight: .medium)).foregroundStyle(Tokens.fg)
                    Spacer()
                    Toggle("", isOn: Binding(get: { state.haptics }, set: { state.setHaptics($0) })).labelsHidden()
                }
                .padding(.horizontal, 16).frame(minHeight: 48)
                RowSeparator()
                HStack {
                    Text("放行时提示音").font(.system(size: 15, weight: .medium)).foregroundStyle(Tokens.fg)
                    Spacer()
                    Toggle("", isOn: Binding(get: { state.sound }, set: { state.setSound($0) })).labelsHidden()
                }
                .padding(.horizontal, 16).frame(minHeight: 48)
                RowSeparator()
                SettingRow(title: "推送", value: "APNs", chevron: false)
            }
            .padding(.top, 22)

            FootNote(text: "恢复码 \(state.recoveryPath)", mono: true)
        }
        .background(Tokens.surface)
        .navigationTitle("设置")
        .fileExporter(isPresented: $exporting, document: exportDoc, contentType: .data, defaultFilename: "easy-unlocker-backup.eu1") { _ in }
        .fileImporter(isPresented: $pickingImport, allowedContentTypes: [.json]) { result in
            switch result {
            case .success(let url):
                handleImportFile(url)
            case .failure(let e):
                state.showToast(e.localizedDescription)
            }
        }
        .sheet(item: $importSession) { session in
            NavigationStack {
                ImportView(candidates: session.candidates, notice: session.notice,
                           onCancel: { importSession = nil }) { picked in
                    importSession = nil
                    if state.importEntries(picked) != nil {
                        state.selectedTab = 0
                    }
                }
            }
        }
    }

    private func handleImportFile(_ url: URL) {
        guard state.unlocked else {
            state.showToast("库已锁上，请先解锁再导入。")
            return
        }
        // 安全域文件要先申请访问权。
        let secured = url.startAccessingSecurityScopedResource()
        defer { if secured { url.stopAccessingSecurityScopedResource() } }
        do {
            let data = try Data(contentsOf: url)
            guard data.count <= BitwardenAdapter.maxBytes else {
                state.showToast("文件太大了，先在 Bitwarden 里导出成 JSON。")
                return
            }
            let result = try BitwardenAdapter.parse(String(decoding: data, as: UTF8.self))
            guard state.unlocked else {
                state.showToast("库已锁上，请重新解锁后再导入。")
                return
            }
            if result.entries.isEmpty {
                state.showToast("没有可导入的条目。\(state.importNotice(result))")
            } else {
                importSession = ImportSession(candidates: result.entries, notice: state.importNotice(result))
            }
        } catch {
            state.showToast(error.localizedDescription)
        }
    }

    private var gatewayValue: String {
        if state.pairings.isEmpty { return "未配对" }
        let active = state.activePairing?.name ?? ""
        return state.pairings.count > 1 ? "\(active) · 共 \(state.pairings.count) 个" : active
    }

    private func exportBackup() {
        guard let bytes = state.exportVault() else { return }
        exportDoc = VaultFileDocument(data: bytes)
        exporting = true
    }
}

/// 导出用文档壳子。
struct VaultFileDocument: FileDocument {
    static var readableContentTypes: [UTType] { [.data] }
    var data: Data
    init(data: Data) { self.data = data }
    init(configuration: ReadConfiguration) throws { data = configuration.file.regularFileContents ?? Data() }
    func fileWrapper(configuration: WriteConfiguration) throws -> FileWrapper { .init(regularFileWithContents: data) }
}

/// .irow 形状的一行：标题 + 右侧值 + 可选 chevron。
struct SettingRow: View {
    let title: String
    let value: String
    var chevron = true
    var body: some View {
        HStack {
            Text(title).font(.system(size: 15, weight: .medium)).foregroundStyle(Tokens.fg)
            Spacer()
            if !value.isEmpty {
                Text(value)
                    .font(.system(size: 14))
                    .foregroundStyle(Tokens.muted)
                    .lineLimit(1)
            }
            if chevron {
                Image(systemName: "chevron.right")
                    .font(.system(size: 12, weight: .semibold))
                    .foregroundStyle(Tokens.meta.opacity(0.75))
            }
        }
        .padding(.horizontal, 16)
        .frame(minHeight: 48)
        .contentShape(Rectangle())
    }
}
