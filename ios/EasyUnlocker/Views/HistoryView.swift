import SwiftUI

/// 批准记录（screens/history.html）：分段筛选 + 点行 push 详情。
struct HistoryView: View {
    @EnvironmentObject var state: AppState
    @State private var filter = "all"

    private var filtered: [HistoryEntry] {
        if filter == "all" { return state.history }
        return state.history.filter { $0.decision == filter }
    }

    var body: some View {
        VStack(spacing: 0) {
            Picker("筛选", selection: $filter) {
                Text("全部").tag("all")
                Text("已放行").tag("approved")
                Text("已拒绝").tag("denied")
                Text("已超时").tag("expired")
            }
            .pickerStyle(.segmented)
            .padding(.horizontal, 16)
            .padding(.top, 4)
            .padding(.bottom, 6)

            ScrollView {
                if filtered.isEmpty {
                    FootNote(text: "这个状态下还没有记录。")
                }
                IGroup {
                    ForEach(Array(filtered.enumerated()), id: \.element.id) { idx, entry in
                        if idx > 0 { RowSeparator() }
                        NavigationLink(value: entry.requestId) {
                            HStack {
                                VStack(alignment: .leading, spacing: 1) {
                                    Text(entry.item)
                                        .font(.mono(13, weight: .medium))
                                        .foregroundStyle(Tokens.fg)
                                        .lineLimit(1)
                                    Text("\(entry.requester) · \(fmtDate(entry.at))")
                                        .font(.mono(12))
                                        .foregroundStyle(Tokens.muted)
                                        .lineLimit(1)
                                }
                                Spacer()
                                DecisionChip(decision: entry.decision)
                            }
                            .padding(.horizontal, 16)
                            .padding(.vertical, 10)
                        }
                    }
                }
                FootNote(text: "放行、拒绝和超时会留在这台手机上，不上传。")
            }
        }
        .background(Tokens.surface)
        .navigationTitle("批准记录")
        .navigationBarTitleDisplayMode(.inline)
        .navigationDestination(for: String.self) { id in
            if let entry = state.history.first(where: { $0.requestId == id }) {
                HistoryDetailView(entry: entry)
            }
        }
    }

    private func fmtDate(_ ms: Int64) -> String {
        let d = Date(timeIntervalSince1970: TimeInterval(ms) / 1000)
        return d.formatted(.dateTime.month(.wide).day().hour().minute().locale(Locale(identifier: "zh_CN")))
    }
}

/// 决策 chip：已放行绿 / 已拒绝红 / 已超时黄。
struct DecisionChip: View {
    let decision: String
    var body: some View {
        switch decision {
        case "approved": Chip(text: "已放行", tone: .ok)
        case "denied": Chip(text: "已拒绝", tone: .bad)
        default: Chip(text: "已超时", tone: .warn)
        }
    }
}

/// 记录详情（push）：请求快照卡 + 处理结果分组列表。
struct HistoryDetailView: View {
    let entry: HistoryEntry

    var body: some View {
        ScrollView {
            RequestCard(
                requester: entry.requester,
                subtitle: "\(fmtDate(entry.at)) 发起",
                trailing: { DecisionChip(decision: entry.decision) }
            ) {
                VStack(alignment: .leading, spacing: 14) {
                    Text(entry.item)
                        .font(.mono(24, weight: .semibold))
                        .tracking(-0.7)
                        .foregroundStyle(Tokens.fg)
                        .fixedSize(horizontal: false, vertical: true)
                    if !entry.purpose.isEmpty {
                        ReqField(label: "用途") {
                            Text(entry.purpose)
                                .font(.system(size: 14))
                                .foregroundStyle(Tokens.fg2)
                                .lineSpacing(3)
                        }
                    }
                    if !entry.commandLine.isEmpty {
                        ReqField(label: "命令") {
                            CommandBlock(text: entry.commandLine)
                        }
                    }
                }
            }

            Text("处理结果")
                .font(.system(size: 12))
                .tracking(0.48)
                .textCase(.uppercase)
                .foregroundStyle(Tokens.muted)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.horizontal, 32)
                .padding(.top, 22)
                .padding(.bottom, 6)

            IGroup {
                DetailRow(label: "结果", value: resultText, color: resultColor)
                RowSeparator()
                DetailRow(label: "批准方式", value: entry.via.isEmpty ? "—" : viaText)
                RowSeparator()
                DetailRow(label: "交付字段", value: entry.field.isEmpty ? "—" : entry.field)
                RowSeparator()
                DetailRow(label: "耗时", value: tookText)
            }

            FootNote(text: "这是当时那条请求的完整快照，值本身不留痕。")
        }
        .background(Tokens.surface)
        .navigationTitle("记录详情")
        .navigationBarTitleDisplayMode(.inline)
    }

    private var resultText: String {
        switch entry.decision {
        case "approved": return "已放行"
        case "denied": return "已拒绝"
        default: return "已超时"
        }
    }
    private var resultColor: Color {
        switch entry.decision {
        case "approved": return Tokens.success
        case "denied": return Tokens.danger
        default: return Color(red: 0.55, green: 0.42, blue: 0.02)
        }
    }
    private var viaText: String {
        switch entry.via {
        case "faceid": return "Face ID"
        case "touchid": return "Touch ID"
        case "opticid": return "Optic ID"
        case "biometric": return "生物识别"
        case "password": return "密码"
        default: return entry.via
        }
    }
    private var tookText: String {
        if entry.tookMs <= 0 { return "—" }
        let s = Int(entry.tookMs / 1000)
        if entry.decision == "expired" { return "\(s) 秒无响应" }
        if entry.decision == "denied" { return "\(s) 秒后拒绝" }
        return "\(s) 秒"
    }

    private func fmtDate(_ ms: Int64) -> String {
        let d = Date(timeIntervalSince1970: TimeInterval(ms) / 1000)
        return d.formatted(.dateTime.month(.wide).day().hour().minute().locale(Locale(identifier: "zh_CN")))
    }
}

private struct DetailRow: View {
    let label: String
    let value: String
    var color: Color = Tokens.muted
    var body: some View {
        HStack {
            Text(label).font(.system(size: 15, weight: .medium)).foregroundStyle(Tokens.fg)
            Spacer()
            Text(value)
                .font(.system(size: 14))
                .foregroundStyle(color)
                .lineLimit(1)
        }
        .padding(.horizontal, 16)
        .frame(minHeight: 48)
    }
}
