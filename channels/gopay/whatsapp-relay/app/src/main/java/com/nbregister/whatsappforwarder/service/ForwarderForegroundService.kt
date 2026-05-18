package com.nbregister.whatsappforwarder.service

import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import android.util.Log
import androidx.core.app.NotificationCompat
import androidx.core.content.ContextCompat
import com.nbregister.whatsappforwarder.MainActivity
import com.nbregister.whatsappforwarder.R

class ForwarderForegroundService : Service() {
    private val rebindHandler = Handler(Looper.getMainLooper())
    private var rebindLoopStarted = false
    private val rebindRunnable = object : Runnable {
        override fun run() {
            NotificationListenerRebinder.request(this@ForwarderForegroundService)
            rebindHandler.postDelayed(this, REBIND_INTERVAL_MS)
        }
    }

    override fun onCreate() {
        super.onCreate()
        startInForeground()
        startRebindLoop()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        startInForeground()
        startRebindLoop()
        return START_STICKY
    }

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onDestroy() {
        rebindHandler.removeCallbacks(rebindRunnable)
        rebindLoopStarted = false
        super.onDestroy()
    }

    private fun startRebindLoop() {
        if (rebindLoopStarted) {
            return
        }
        rebindLoopStarted = true
        NotificationListenerRebinder.request(this, force = true)
        rebindHandler.postDelayed(rebindRunnable, REBIND_INTERVAL_MS)
    }

    private fun startInForeground() {
        createNotificationChannel()
        val notification = buildNotification()
        runCatching {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
                startForeground(
                    NOTIFICATION_ID,
                    notification,
                    ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE,
                )
            } else {
                startForeground(NOTIFICATION_ID, notification)
            }
        }.onFailure { exc ->
            Log.w(TAG, "Failed to enter foreground: ${exc.message}")
            stopSelf()
        }
    }

    private fun buildNotification() =
        NotificationCompat.Builder(this, CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_stat_forwarder)
            .setContentTitle("WhatsApp Forwarder")
            .setContentText("Listening for WhatsApp OTP notifications")
            .setContentIntent(
                PendingIntent.getActivity(
                    this,
                    0,
                    Intent(this, MainActivity::class.java),
                    PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
                ),
            )
            .setOngoing(true)
            .setSilent(true)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .build()

    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) {
            return
        }
        val channel = NotificationChannel(
            CHANNEL_ID,
            "WhatsApp Forwarder",
            NotificationManager.IMPORTANCE_LOW,
        ).apply {
            description = "Keeps WhatsApp OTP forwarding active"
            setShowBadge(false)
        }
        getSystemService(NotificationManager::class.java)
            ?.createNotificationChannel(channel)
    }

    companion object {
        private const val TAG = "WhatsAppForwarder"
        private const val CHANNEL_ID = "whatsapp_forwarder_keep_alive"
        private const val NOTIFICATION_ID = 1001
        private const val REBIND_INTERVAL_MS = 60_000L

        fun start(context: Context) {
            val appContext = context.applicationContext
            val intent = Intent(appContext, ForwarderForegroundService::class.java)
            runCatching {
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                    ContextCompat.startForegroundService(appContext, intent)
                } else {
                    appContext.startService(intent)
                }
            }.onFailure { exc ->
                Log.w(TAG, "Foreground keep-alive not started: ${exc.message}")
            }
        }
    }
}
