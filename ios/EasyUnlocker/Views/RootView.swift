import SwiftUI

/// 根路由：建库 → 解锁 → 主界面（三个 tab）。覆盖 toast 与切后台遮罩。
struct RootView: View {
    @EnvironmentObject var state: AppState
    @Environment(\.scenePhase) private var scenePhase

    var body: some View {
        ZStack {
            if !state.vaultExists {
                SetupView()
            } else if !state.unlocked {
                UnlockView()
            } else {
                MainTabView()
            }

            // toast（.toast：深色胶囊，底部浮层）
            if !state.toast.isEmpty {
                VStack {
                    Spacer()
                    Text(state.toast)
                        .font(.system(size: 14, weight: .medium))
                        .foregroundStyle(.white)
                        .padding(.horizontal, 18)
                        .padding(.vertical, 10)
                        .background(Tokens.fg)
                        .clipShape(Capsule())
                        .shadow(color: .black.opacity(0.12), radius: 16, y: 6)
                        .padding(.bottom, 96)
                        .transition(.move(edge: .bottom).combined(with: .opacity))
                }
                .animation(.easeOut(duration: 0.22), value: state.toast)
            }
        }
        // 切后台/切多任务：盖住内容（对应安卓 FLAG_SECURE + 自动上锁）。
        .overlay {
            if scenePhase != .active {
                Tokens.surface.ignoresSafeArea()
            }
        }
    }
}
