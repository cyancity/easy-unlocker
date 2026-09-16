import Foundation

/// 轻量偏好（UserDefaults）。明文密钥绝不进这里。
final class Prefs {
    private let sp = UserDefaults.standard

    var activePairingId: String {
        get { sp.string(forKey: "active_pairing_id") ?? "" }
        set { sp.set(newValue, forKey: "active_pairing_id") }
    }
    var haptics: Bool {
        get { sp.object(forKey: "haptics") as? Bool ?? true }
        set { sp.set(newValue, forKey: "haptics") }
    }
    var sound: Bool {
        get { sp.object(forKey: "sound") as? Bool ?? true }
        set { sp.set(newValue, forKey: "sound") }
    }
    var lastExportAt: String {
        get { sp.string(forKey: "last_export_at") ?? "" }
        set { sp.set(newValue, forKey: "last_export_at") }
    }
    var lastImportAt: String {
        get { sp.string(forKey: "last_import_at") ?? "" }
        set { sp.set(newValue, forKey: "last_import_at") }
    }
    /// 0 = 不自动关闭；默认 3 秒后回到条目页。
    var autoCloseSeconds: Int {
        get { sp.integer(forKey: "auto_close_seconds") }
        set { sp.set(min(60, max(0, newValue)), forKey: "auto_close_seconds") }
    }
    /// 用户没有动过这个设置时给默认 3。
    var autoCloseSecondsOrDefault: Int {
        sp.object(forKey: "auto_close_seconds") == nil ? 3 : autoCloseSeconds
    }
}
