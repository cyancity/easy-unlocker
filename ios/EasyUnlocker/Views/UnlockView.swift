import SwiftUI

/// 首次使用：展示恢复码，确认后建库。恢复码是保险库唯一钥匙——丢了就打不开。
struct SetupView: View {
    @EnvironmentObject var state: AppState
    @State private var copied = false

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Text("easy-unlocker")
                    .font(.system(size: 34, weight: .bold))
                    .tracking(-0.5)
                Spacer()
            }
            .padding(.horizontal, 16)
            .padding(.top, 10)

            ScrollView {
                VStack(alignment: .leading, spacing: 0) {
                    KVLabel("恢复码 · 只出现这一次") {
                        CopyButton(text: state.recoveryPreview, copied: $copied)
                    }
                    KVCard {
                        Text(state.recoveryPreview)
                            .font(.mono(14))
                            .lineSpacing(5)
                            .foregroundStyle(Tokens.fg)
                            .textSelection(.enabled)
                    }
                    FootNote(text: "保险库只能用恢复码解开。把它抄到别处收好——它也写进了 \(state.recoveryPath)，但那台手机丢了就两样都没了。", alignLeft: true)
                    FootNote(text: "批准请求时用 \(state.biometryName) 确认本人；也可以在「设置 → 解锁密码」设一个密码兜底。", alignLeft: true)
                }
            }

            VStack(spacing: 10) {
                Button {
                    state.createVault()
                } label: {
                    Label("创建保险库", systemImage: "lock.shield")
                }
                .buttonStyle(PillButtonStyle())
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 14)
        }
        .background(Tokens.surface)
    }
}

/// 解锁：中间是 app 无背景 logo；底部主按钮走生物识别（自动弹出），
/// 「解锁密码 / 恢复码」降级入口同一行，点谁才展开谁的输入框。
struct UnlockView: View {
    @EnvironmentObject var state: AppState
    @State private var input = ""
    @State private var mode: Mode? = nil
    @State private var attempted = false

    enum Mode { case password, recovery }

    var body: some View {
        VStack(spacing: 0) {
            if !state.message.isEmpty {
                Notice(tone: .info, title: "有请求待批准", bodyText: "解锁后会直接带你过去。")
                    .padding(.horizontal, 16)
                    .padding(.top, 10)
            }

            Spacer()

            Image("logo-mark")
                .resizable()
                .scaledToFit()
                .frame(width: 128, height: 128)

            Spacer()

            VStack(spacing: 10) {
                // 降级输入框：点了「用解锁密码/恢复码」才出现
                if mode == .password {
                    HStack(spacing: 8) {
                        SecureField("解锁密码", text: $input)
                            .accessibilityIdentifier("unlock.pw")
                            .textContentType(.password)
                            .padding(.horizontal, 14)
                            .frame(minHeight: 44)
                            .background(Tokens.bg)
                            .overlay(RoundedRectangle(cornerRadius: 12).stroke(Tokens.border))
                            .clipShape(RoundedRectangle(cornerRadius: 12))
                        Button("确认") { state.unlockWithPassword(input); input = "" }
                            .buttonStyle(QuietButtonStyle())
                            .frame(width: 86)
                            .background(Tokens.bg)
                            .overlay(RoundedRectangle(cornerRadius: 12).stroke(Tokens.border))
                            .clipShape(RoundedRectangle(cornerRadius: 12))
                    }
                    .padding(.horizontal, 16)
                    .transition(.move(edge: .bottom).combined(with: .opacity))
                } else if mode == .recovery {
                    HStack(spacing: 8) {
                        TextField("xxxx-xxxx-xxxx-…", text: $input)
                            .accessibilityIdentifier("unlock.recovery")
                            .font(.mono(15))
                            .autocorrectionDisabled()
                            .textInputAutocapitalization(.never)
                            .padding(.horizontal, 14)
                            .frame(minHeight: 44)
                            .background(Tokens.bg)
                            .overlay(RoundedRectangle(cornerRadius: 12).stroke(Tokens.border))
                            .clipShape(RoundedRectangle(cornerRadius: 12))
                        Button("解锁") { state.unlock(recoveryCode: input); input = "" }
                            .buttonStyle(QuietButtonStyle())
                            .frame(width: 86)
                            .background(Tokens.bg)
                            .overlay(RoundedRectangle(cornerRadius: 12).stroke(Tokens.border))
                            .clipShape(RoundedRectangle(cornerRadius: 12))
                    }
                    .padding(.horizontal, 16)
                    .transition(.move(edge: .bottom).combined(with: .opacity))
                }

                // 主解锁按钮：生物识别优先，80% 宽居中
                if state.canFingerprint {
                    Button {
                        attempted = true
                        state.unlockWithFingerprint()
                    } label: {
                        Label("用 \(state.biometryName) 解锁", systemImage: state.biometryIcon)
                    }
                    .buttonStyle(PillButtonStyle())
                    .containerRelativeFrame(.horizontal) { length, _ in length * 0.8 }
                }

                // 降级入口同一行：与主按钮同宽（80%），小字矮钮
                HStack(spacing: 10) {
                    if state.hasPassword {
                        Button(mode == .password ? "收起密码" : "用解锁密码") {
                            withAnimation(.easeOut(duration: 0.18)) {
                                mode = mode == .password ? nil : .password
                                input = ""
                            }
                        }
                        .buttonStyle(TintedButtonStyle(size: 13, minHeight: 38))
                        .frame(maxWidth: .infinity)
                    }
                    Button(mode == .recovery ? "收起恢复码" : "用恢复码") {
                        withAnimation(.easeOut(duration: 0.18)) {
                            mode = mode == .recovery ? nil : .recovery
                            input = ""
                        }
                    }
                    .buttonStyle(TintedButtonStyle(size: 13, minHeight: 38))
                    .frame(maxWidth: .infinity)
                }
                .containerRelativeFrame(.horizontal) { length, _ in length * 0.8 }
            }
            .padding(.bottom, 14)

            FootNote(text: "认证只在本机完成，解锁的是本机保险库。")
                .padding(.bottom, 8)
        }
        .background(Tokens.surface)
        .onAppear {
            if state.canFingerprint && !attempted {
                attempted = true
                state.unlockWithFingerprint()
            }
        }
    }
}
