package io.github.cyancity.easyunlocker

import android.content.Context

class Prefs(context: Context) {
    private val sp = context.getSharedPreferences("easy_unlocker", Context.MODE_PRIVATE)

    /** 当前网关（Pairing.id）；空 = 没配过任何网关。 */
    var activePairingId: String
        get() = sp.getString("active_pairing_id", "") ?: ""
        set(value) { sp.edit().putString("active_pairing_id", value).apply() }

    // 以下三个是「单网关时代」的字段：只用于一次性迁移进 pairings.json，迁移完就清空。
    var brokerUrl: String
        get() = sp.getString("broker_url", "") ?: ""
        set(value) { sp.edit().putString("broker_url", value.trim().trimEnd('/')).apply() }

    var deviceToken: String
        get() = sp.getString("device_token", "") ?: ""
        set(value) { sp.edit().putString("device_token", value).apply() }

    var deviceName: String
        get() = sp.getString("device_name", "android") ?: "android"
        set(value) { sp.edit().putString("device_name", value).apply() }

    fun clearLegacyPairing() {
        sp.edit().remove("broker_url").remove("device_token").remove("device_name").apply()
    }

    var haptics: Boolean
        get() = sp.getBoolean("haptics", true)
        set(value) { sp.edit().putBoolean("haptics", value).apply() }

    var sound: Boolean
        get() = sp.getBoolean("sound", true)
        set(value) { sp.edit().putBoolean("sound", value).apply() }

    var lastExportAt: String
        get() = sp.getString("last_export_at", "") ?: ""
        set(value) { sp.edit().putString("last_export_at", value).apply() }

    var lastImportAt: String
        get() = sp.getString("last_import_at", "") ?: ""
        set(value) { sp.edit().putString("last_import_at", value).apply() }

    /** 0 = 不自动关闭；默认 3 秒后关掉 App。 */
    var autoCloseSeconds: Int
        get() = sp.getInt("auto_close_seconds", 3)
        set(value) { sp.edit().putInt("auto_close_seconds", value.coerceIn(0, 60)).apply() }
}
