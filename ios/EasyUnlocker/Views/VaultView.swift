import SwiftUI

/// 条目首页（screens/vault.html）：大标题 + 实时搜索 + 掩码列表 + tab。
struct VaultView: View {
    @EnvironmentObject var state: AppState
    @State private var query = ""
    @State private var editing: VaultItem? = nil
    @State private var adding = false

    private var filtered: [VaultItem] {
        let q = query.trimmingCharacters(in: .whitespaces).lowercased()
        if q.isEmpty { return state.items }
        return state.items.filter {
            $0.name.lowercased().contains(q) || $0.aliases.contains { $0.lowercased().contains(q) }
        }
    }

    var body: some View {
        ScrollView {
            if filtered.isEmpty && !query.isEmpty {
                FootNote(text: "没有匹配的条目。")
            }
            if state.items.isEmpty {
                FootNote(text: "库里还没有条目，点右上角加一条。")
            }
            IGroup {
                ForEach(Array(filtered.enumerated()), id: \.element.id) { idx, item in
                    if idx > 0 { RowSeparator() }
                    NavigationLink(value: item.id) {
                        VStack(alignment: .leading, spacing: 1) {
                            Text(item.name)
                                .font(.mono(15, weight: .medium))
                                .foregroundStyle(Tokens.fg)
                                .lineLimit(1)
                            Text(subtitle(item))
                                .font(.mono(13))
                                .foregroundStyle(Tokens.muted)
                        }
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(.horizontal, 16)
                        .padding(.vertical, 12)
                    }
                }
            }
            FootNote(text: "密钥明文只在这台手机上，批准一次才放出去一次。")
        }
        .background(Tokens.surface)
        .navigationTitle("条目")
        .searchable(text: $query, placement: .navigationBarDrawer(displayMode: .always), prompt: "搜索")
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Button {
                    adding = true
                } label: {
                    Image(systemName: "plus")
                        .font(.system(size: 22, weight: .regular))
                }
                .accessibilityLabel("添加条目")
            }
        }
        .navigationDestination(for: String.self) { id in
            ItemDetailView(itemID: id)
                .toolbar(.hidden, for: .tabBar)
        }
        .sheet(isPresented: $adding) {
            NavigationStack {
                ItemEditView(editing: nil)
            }
        }
    }

    private func subtitle(_ item: VaultItem) -> String {
        var s = "••••••••"
        if let first = item.aliases.first { s += " · 别名 \(first)" }
        return s
    }
}
