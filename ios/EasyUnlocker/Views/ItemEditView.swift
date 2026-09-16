import SwiftUI

/// 添加/编辑条目（screens/item-edit.html）：sheet 模态、取消/保存入导航栏、
/// 必填校验、备注识别 SSH CA 私钥自动提示、生成 CA 私钥；导入留 disabled stub。
struct ItemEditView: View {
    @EnvironmentObject var state: AppState
    let editing: VaultItem?
    @Environment(\.dismiss) private var dismiss

    @State private var name = ""
    @State private var secret = ""
    @State private var note = ""
    @State private var error = ""
    // sheet 消失时不清焦点会让键盘残留在下一屏盖住 tab bar——保存/取消前必须先释放
    @FocusState private var focused: Focused?
    private enum Focused { case name, secret, note }

    private var isCA: Bool { OpenSshKey.looksLikePrivateKey(note) }

    var body: some View {
        ScrollView {
            FormLabel("名称")
            IGroup {
                TextField("OPENAI_API_KEY", text: $name)
                    .focused($focused, equals: .name)
                    .accessibilityIdentifier("edit.name")
                    .font(.mono(17))
                    .autocorrectionDisabled()
                    .textInputAutocapitalization(.characters)
                    .padding(.horizontal, 16)
                    .frame(minHeight: 46)
                    .onChange(of: name) { _, _ in error = "" }
            }

            FormLabel("密码 · 短值").padding(.top, 18)
            IGroup {
                TextField("粘贴 token / 密码", text: $secret)
                    .focused($focused, equals: .secret)
                    .accessibilityIdentifier("edit.secret")
                    .font(.mono(17))
                    .autocorrectionDisabled()
                    .textInputAutocapitalization(.never)
                    .padding(.horizontal, 16)
                    .frame(minHeight: 46)
                    .onChange(of: secret) { _, _ in error = "" }
            }

            FormLabel("备注 · 长文本（可空）").padding(.top, 18)
            IGroup {
                TextField("SSH 私钥、整段 .env 放这里", text: $note, axis: .vertical)
                    .focused($focused, equals: .note)
                    .accessibilityIdentifier("edit.note")
                    .font(.mono(17))
                    .lineLimit(4...10)
                    .autocorrectionDisabled()
                    .textInputAutocapitalization(.never)
                    .padding(.horizontal, 16)
                    .padding(.vertical, 10)
                    .onChange(of: note) { _, _ in error = "" }
            }

            if isCA {
                Text("备注里是一把 SSH 私钥——这条可以当 CA 给 easyGet ssh 签证书。")
                    .font(.system(size: 14))
                    .foregroundStyle(Tokens.accentActive)
                    .lineSpacing(3)
                    .padding(.horizontal, 32)
                    .padding(.top, 10)
            }

            if !isCA {
                FormLabel("SSH CA 私钥（可空）").padding(.top, 14)
                IGroup {
                    Button {
                        note = OpenSshKey.generate().toPem()
                    } label: {
                        HStack {
                            Text("生成一把新私钥")
                                .font(.system(size: 17))
                                .foregroundStyle(Tokens.accent)
                            Spacer()
                        }
                        .padding(.horizontal, 16)
                        .frame(minHeight: 46)
                        .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                    RowSeparator()
                    // TODO: 导入已有私钥——设计稿标记「即将推出」，首批不实现。
                    HStack {
                        Text("导入已有私钥")
                            .font(.system(size: 17))
                            .foregroundStyle(Tokens.meta)
                        Spacer()
                        Chip(text: "即将推出")
                    }
                    .padding(.horizontal, 16)
                    .frame(minHeight: 46)
                }
                FootNote(text: "生成的私钥会直接写进上面的备注栏，只存在本机保险库。", alignLeft: true)
                    .padding(.top, -12)
            }

            if !error.isEmpty {
                Text(error)
                    .font(.system(size: 14, weight: .medium))
                    .foregroundStyle(Tokens.danger)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.horizontal, 32)
                    .padding(.top, 10)
            }

            FootNote(text: "和 easyGet 里的请求名可以不同，审批时手选对齐。")
        }
        .background(Tokens.surface)
        .navigationTitle(editing == nil ? "添加条目" : "编辑条目")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .cancellationAction) {
                Button("取消") { dismissKeyboard(); dismiss() }
            }
            ToolbarItem(placement: .confirmationAction) {
                Button("保存") { save() }
                    .fontWeight(.semibold)
            }
        }
        .onAppear {
            if let editing {
                name = editing.name
                secret = editing.secret
                note = editing.note
            }
        }
    }

    private func save() {
        if let err = state.saveItem(editingId: editing?.id, name: name, secret: secret, note: note) {
            error = err
        } else {
            dismissKeyboard()
            state.showToast(editing == nil ? "已添加" : "已保存")
            dismiss()
        }
    }
}

/// item-edit 用的表单小标签（.form-label）。
struct FormLabel: View {
    let text: String
    init(_ text: String) { self.text = text }
    var body: some View {
        Text(text)
            .font(.system(size: 12))
            .tracking(0.24)
            .foregroundStyle(Tokens.muted)
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.horizontal, 32)
            .padding(.top, 8)
            .padding(.bottom, 6)
    }
}
