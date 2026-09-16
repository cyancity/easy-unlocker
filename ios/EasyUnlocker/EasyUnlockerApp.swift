import SwiftUI
import UserNotifications

@main
struct EasyUnlockerApp: App {
    @UIApplicationDelegateAdaptor(AppDelegate.self) var appDelegate
    @StateObject private var state = AppState()
    @Environment(\.scenePhase) private var scenePhase

    var body: some Scene {
        WindowGroup {
            RootView()
                .environmentObject(state)
                .preferredColorScheme(.light)
                .tint(Tokens.accent)
                .onChange(of: scenePhase) { _, phase in
                    switch phase {
                    case .active:
                        state.startPolling()
                        appDelegate.registerPush(state: state)
                    case .background:
                        state.stopPolling()
                        if !AppState.uiTesting { state.lock() }
                    default:
                        break
                    }
                }
        }
    }
}

/// APNs 注册（对应安卓 Push.fetch）。模拟器拿不到 token，轮询兜底，行为不变。
final class AppDelegate: NSObject, UIApplicationDelegate, UNUserNotificationCenterDelegate {
    private weak var state: AppState?

    func application(_ application: UIApplication,
                     didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]? = nil) -> Bool {
        UNUserNotificationCenter.current().delegate = self
        return true
    }

    func registerPush(state: AppState) {
        self.state = state
        UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound, .badge]) { granted, _ in
            guard granted else { return }
            DispatchQueue.main.async {
                UIApplication.shared.registerForRemoteNotifications()
            }
        }
    }

    func application(_ application: UIApplication, didRegisterForRemoteNotificationsWithDeviceToken deviceToken: Data) {
        let token = deviceToken.map { String(format: "%02x", $0) }.joined()
        Task { @MainActor in state?.registerPushToken(token) }
    }

    /// 推送点进来：切到「待批准」。
    func userNotificationCenter(_ center: UNUserNotificationCenter,
                                didReceive response: UNNotificationResponse,
                                withCompletionHandler completionHandler: @escaping () -> Void) {
        let gateway = response.notification.request.content.userInfo["gateway"] as? String
        Task { @MainActor in
            if let state {
                if let gateway, !gateway.isEmpty,
                   let target = state.pairings.first(where: { normalizeGatewayUrl($0.url) == normalizeGatewayUrl(gateway) }) {
                    state.switchPairing(target.id)
                }
                state.selectedTab = 1
                state.refreshPending()
            }
        }
        completionHandler()
    }

    /// 前台也弹推送样式（横幅），与安卓的 Notifications 一致。
    func userNotificationCenter(_ center: UNUserNotificationCenter,
                                willPresent notification: UNNotification,
                                withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void) {
        completionHandler([.banner, .sound])
    }
}
