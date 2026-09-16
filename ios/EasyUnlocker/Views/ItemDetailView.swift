import SwiftUI

/// 条目详情（screens/item-detail.html）：别名 chips + 密码/备注/CA 公钥卡 +
/// 更多操作（编辑/删除）+ 删除二次确认。
struct ItemDetailView: View {
    @EnvironmentObject var state: AppState
    let itemID: String
    @State private var menu = false
    @State private var confirmDelete = false
    @State private var editing = false
    @State private var copiedSecret = false
    @State private var copiedNote = false
    @State private var copiedCA = false
    @Environment(\.dismiss) private var dismiss

    private var item: VaultItem? {
        state.items.first { $0.id == itemID }
    }

    private var ca: OpenSshKey.PrivateKey? {
        guard let item else { return nil }
        return OpenSshKey.parseOrNull(item.note) ?? OpenSshKey.parseOrNull(item.secret)
    }

    var body: some View {
        ScrollView {
            if let item {
                detailBody(item)
            }
        }
        .background(Tokens.surface)
        .navigationTitle(item?.name ?? "")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .principal) {
                Text(item?.name ?? "").font(.mono(13, weight: .semibold))
            }
            ToolbarItem(placement: .topBarTrailing) {
                Button { menu = true } label: {
                    Image(systemName: "ellipsis.circle")
                        .font(.system(size: 20))
                }
                .accessibilityLabel("更多操作")
            }
        }
        .confirmationDialog("条目操作", isPresented: $menu, titleVisibility: .visible) {
            Button("编辑名称、密码与备注") { editing = true }
            Button("删除这条凭据", role: .destructive) { confirmDelete = true }
            Button("取消", role: .cancel) {}
        }
        .confirmationDialog("删除 \(item?.name ?? "")？\n删除后无法撤销。恢复只能靠密文备份 + 恢复码。",
                            isPresented: $confirmDelete, titleVisibility: .visible) {
            Button("删除", role: .destructive) {
                state.deleteItem(itemID)
                dismiss()
            }
            Button("取消", role: .cancel) {}
        }
        .sheet(isPresented: $editing) {
            NavigationStack {
                ItemEditView(editing: item)
            }
        }
    }

    @ViewBuilder
    private func detailBody(_ item: VaultItem) -> some View {
        Group {
            if !item.aliases.isEmpty {
                HStack(spacing: 6) {
                    ForEach(item.aliases, id: \.self) { Chip(text: "别名 \($0)") }
                    Spacer()
                }
                .padding(.horizontal, 16)
                .padding(.top, 6)
            }

            KVLabel("密码") {
                CopyButton(text: item.secret, copied: $copiedSecret)
            }
            KVCard {
                Text(item.secret.isEmpty ? "--" : item.secret)
                    .font(.mono(13))
                    .lineSpacing(4)
                    .foregroundStyle(item.secret.isEmpty ? Tokens.meta : Tokens.fg)
                    .fixedSize(horizontal: false, vertical: true)
                    .textSelection(.enabled)
            }

            KVLabel("备注") {
                CopyButton(text: item.note, copied: $copiedNote)
            }
            KVCard {
                Text(item.note.isEmpty ? "--" : item.note)
                    .font(.mono(13))
                    .lineSpacing(4)
                    .foregroundStyle(item.note.isEmpty ? Tokens.meta : Tokens.fg)
                    .fixedSize(horizontal: false, vertical: true)
                    .textSelection(.enabled)
            }

            if let ca {
                KVLabel("SSH CA 公钥") {
                    CopyButton(text: ca.authorizedLine(), copied: $copiedCA)
                }
                KVCard(accent: true) {
                    Text(ca.authorizedLine())
                        .font(.mono(12))
                        .lineSpacing(3)
                        .foregroundStyle(Tokens.fg)
                        .fixedSize(horizontal: false, vertical: true)
                        .textSelection(.enabled)
                    Text(ca.fingerprint())
                        .font(.mono(12))
                        .foregroundStyle(Tokens.muted)
                        .padding(.top, 8)
                    Text("把上面那行放进目标机的 TrustedUserCAKeys 文件，这条目就能给 easyGet ssh 签证书。")
                        .font(.system(size: 12))
                        .foregroundStyle(Tokens.muted)
                        .lineSpacing(3)
                        .padding(.top, 8)
                }
            }

            FootNote(text: "切到后台会自动上锁 · 已屏蔽截屏")
        }
    }
}
