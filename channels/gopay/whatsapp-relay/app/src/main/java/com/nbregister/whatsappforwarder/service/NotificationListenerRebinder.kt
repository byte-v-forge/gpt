package com.nbregister.whatsappforwarder.service

import android.content.ComponentName
import android.content.Context
import android.content.pm.PackageManager
import android.os.SystemClock
import android.service.notification.NotificationListenerService
import android.util.Log

object NotificationListenerRebinder {
    private var lastForcedRebindAtMillis = 0L

    fun request(context: Context, force: Boolean = false) {
        val appContext = context.applicationContext
        val component = ComponentName(
            appContext,
            WhatsAppNotificationListenerService::class.java,
        )
        runCatching {
            if (force || shouldForceRebind()) {
                forceComponentRebind(appContext, component)
            }
            NotificationListenerService.requestRebind(component)
            Log.d(TAG, "Notification listener rebind requested force=$force")
        }.onFailure { exc ->
            Log.w(TAG, "Failed to request notification listener rebind: ${exc.message}")
        }
    }

    private fun shouldForceRebind(): Boolean {
        val now = SystemClock.elapsedRealtime()
        return now - lastForcedRebindAtMillis >= FORCE_REBIND_INTERVAL_MS
    }

    private fun forceComponentRebind(context: Context, component: ComponentName) {
        val packageManager = context.packageManager
        packageManager.setComponentEnabledSetting(
            component,
            PackageManager.COMPONENT_ENABLED_STATE_DISABLED,
            PackageManager.DONT_KILL_APP,
        )
        packageManager.setComponentEnabledSetting(
            component,
            PackageManager.COMPONENT_ENABLED_STATE_ENABLED,
            PackageManager.DONT_KILL_APP,
        )
        lastForcedRebindAtMillis = SystemClock.elapsedRealtime()
        Log.i(TAG, "Notification listener component rebind kicked")
    }

    private const val TAG = "WhatsAppForwarder"
    private const val FORCE_REBIND_INTERVAL_MS = 10 * 60 * 1000L
}
