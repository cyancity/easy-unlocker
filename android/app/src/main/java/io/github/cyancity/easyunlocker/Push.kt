package io.github.cyancity.easyunlocker

import android.content.Context
import com.google.firebase.FirebaseApp
import com.google.firebase.messaging.FirebaseMessaging

object Push {
    fun fetch(context: Context, onToken: (String) -> Unit) {
        runCatching {
            if (FirebaseApp.getApps(context).isEmpty()) return
            FirebaseMessaging.getInstance().token.addOnSuccessListener { token ->
                if (!token.isNullOrBlank()) onToken(token)
            }
        }
    }
}
