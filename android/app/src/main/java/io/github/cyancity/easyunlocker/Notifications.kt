package io.github.cyancity.easyunlocker

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.Context
import android.media.AudioAttributes
import android.media.RingtoneManager
import android.os.Build
import androidx.core.app.NotificationManagerCompat

object Notifications {
    const val CHANNEL_ID = "unlock_v2"

    /**
     * 把已经没意义的通知收掉（请求批过了 / 过期了 / 撤销了）。
     * 推送是 FCM 自动展示的，我们拿不到它的 id，所以整批取消——本 App 的通知只有这一类。
     */
    fun clearAll(context: Context) {
        NotificationManagerCompat.from(context).cancelAll()
    }

    fun ensureChannel(context: Context) {
        if (Build.VERSION.SDK_INT < 26) return
        val nm = context.getSystemService(NotificationManager::class.java)
        nm.deleteNotificationChannel("unlock")
        if (nm.getNotificationChannel(CHANNEL_ID) != null) return
        val sound = RingtoneManager.getDefaultUri(RingtoneManager.TYPE_NOTIFICATION)
        val audio = AudioAttributes.Builder()
            .setUsage(AudioAttributes.USAGE_NOTIFICATION)
            .setContentType(AudioAttributes.CONTENT_TYPE_SONIFICATION)
            .build()
        val channel = NotificationChannel(CHANNEL_ID, "批准请求", NotificationManager.IMPORTANCE_HIGH).apply {
            description = "Agent 请求批准"
            lockscreenVisibility = Notification.VISIBILITY_PUBLIC
            setShowBadge(true)
            enableLights(true)
            enableVibration(true)
            vibrationPattern = longArrayOf(0, 160, 80, 160)
            setSound(sound, audio)
        }
        nm.createNotificationChannel(channel)
    }
}
