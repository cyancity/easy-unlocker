import SwiftUI

/// Bitwarden 备份导入勾选页（对齐安卓 ImportPane）：搜索 + 勾选 + 全选可见项。
/// 明文候选只活在这页的生命周期里，关页即释放。
struct ImportView: View {
    let candidates: [ImportCandidate]
    let notice: String
    let onCancel: () -> Void
    let onConfirm: ([ImportCandidate]) -> Void

    @State private var query = ""
    @State private var selected: Set<Int> = []

    private var visible: [ImportCandidate] {
        let q = query.trimmingCharacters(in: .whitespaces).lowercased()
        if q.isEmpty { return candidates }
        return candidates.filter {
            $0.name.lowercased().contains(q)
                || $0.aliases.contains { $0.lowercased().contains(q) }
        }
    }
    private var visibleIds: [Int] { visible.map(\.id) }
    private var allOn: Bool { !visibleIds.isEmpty && visibleIds.allSatisfy { selected.contains($0) } }

    var body: some View {
        VStack(spacing: 0) {
            Text(notice)
                .font(.system(size: 12))
                .foregroundStyle(Tokens.muted)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.horizontal, 16)
                .padding(.top, 10)

            ScrollView {
                if candidates.isEmpty {
                    FootNote(text: "没有可导入的条目。")
                } else if visible.isEmpty {
                    FootNote(text: "没有匹配的条目。")
                } else {
                    IGroup {
                        ForEach(Array(visible.enumerated()), id: \.element.id) { idx, c in
                            if idx > 0 { RowSeparator() }
                            Button {
                                if selected.contains(c.id) { selected.remove(c.id) } else { selected.insert(c.id) }
                            } label: {
                                HStack(spacing: 10) {
                                    Image(systemName: selected.contains(c.id) ? "checkmark.circle.fill" : "circle")
                                        .font(.system(size: 18))
                                        .foregroundStyle(selected.contains(c.id) ? Tokens.accent : Tokens.meta)
                                    VStack(alignment: .leading, spacing: 1) {
                                        Text(c.name)
                                            .font(.mono(13, weight: .medium))
                                            .foregroundStyle(Tokens.fg)
                                            .lineLimit(1)
                                        if let u = c.aliases.first, !u.isEmpty {
                                            Text(u)
                                                .font(.mono(12))
                                                .foregroundStyle(Tokens.muted)
                                                .lineLimit(1)
                                        }
                                    }
                                    Spacer()
                                }
                                .padding(.horizontal, 16)
                                .padding(.vertical, 10)
                                .contentShape(Rectangle())
                            }
                            .buttonStyle(.plain)
                        }
                    }
                    .padding(.top, 6)
                }
            }

            VStack(spacing: 10) {
                Button {
                    onConfirm(candidates.filter { selected.contains($0.id) })
                } label: {
                    Text("导入 \(selected.count) 条")
                }
                .buttonStyle(PillButtonStyle(disabled: selected.isEmpty))
                .disabled(selected.isEmpty)
                .containerRelativeFrame(.horizontal) { l, _ in l * 0.8 }
            }
            .padding(.vertical, 14)
        }
        .background(Tokens.surface)
        .navigationTitle("导入 Bitwarden")
        .navigationBarTitleDisplayMode(.inline)
        .searchable(text: $query, placement: .navigationBarDrawer(displayMode: .always), prompt: "条目名或用户名")
        .toolbar {
            ToolbarItem(placement: .topBarLeading) {
                Button("取消") { onCancel() }
            }
            ToolbarItem(placement: .topBarTrailing) {
                if !visibleIds.isEmpty {
                    Button(allOn ? "取消全选" : "全选") {
                        if allOn { selected.subtract(visibleIds) } else { selected.formUnion(visibleIds) }
                    }
                }
            }
        }
    }
}
