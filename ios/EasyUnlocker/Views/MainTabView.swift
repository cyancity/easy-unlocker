import SwiftUI

/// 三个 tab：条目 / 待批准（badge = 等待数）/ 设置。
/// 详情、编辑、记录都从 tab 内 push/sheet 出去，不带 tab bar。
struct MainTabView: View {
    @EnvironmentObject var state: AppState

    var body: some View {
        TabView(selection: $state.selectedTab) {
            NavigationStack {
                VaultView()
            }
            .tabItem {
                Label("条目", systemImage: "key")
            }
            .tag(0)

            NavigationStack {
                PendingView()
            }
            .tabItem {
                Label("待批准", systemImage: "bell")
            }
            .badge(state.pending.isEmpty ? 0 : state.pending.count)
            .tag(1)

            NavigationStack {
                SettingsView()
            }
            .tabItem {
                Label("设置", systemImage: "gearshape")
            }
            .tag(2)
        }
        .tint(Tokens.accent)
    }
}
