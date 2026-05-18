package com.nbregister.whatsappforwarder.service

import android.app.Notification
import android.os.Build
import android.service.notification.NotificationListenerService
import android.service.notification.StatusBarNotification
import android.util.Log
import com.nbregister.whatsappforwarder.data.OtpExtractor
import com.nbregister.whatsappforwarder.settings.SettingsStore
import com.nbregister.whatsappforwarder.worker.OtpForwardWorker

class WhatsAppNotificationListenerService : NotificationListenerService() {
    private val forwardedFingerprints = LinkedHashSet<String>()

    override fun onListenerConnected() {
        super.onListenerConnected()
        ForwarderForegroundService.start(applicationContext)
        activeNotifications.orEmpty().forEach { item ->
            handleNotification(item, "active")
        }
    }

    override fun onNotificationPosted(sbn: StatusBarNotification?) {
        val item = sbn ?: return
        ForwarderForegroundService.start(applicationContext)
        handleNotification(item, "posted")
    }

    private fun handleNotification(item: StatusBarNotification, event: String) {
        val settings = SettingsStore(applicationContext)
        val appSettings = settings.readAll()
        if (
            appSettings.webhookUrl.isBlank() ||
            item.packageName !in SettingsStore.WATCHED_PACKAGES
        ) {
            Log.d(TAG, "Ignored notification event=$event package=${item.packageName} webhook=${appSettings.webhookUrl.isNotBlank()}")
            return
        }

        val appName = resolveAppName(item.packageName)
        val candidates = extractCandidates(item.notification)
        if (candidates.isEmpty()) {
            Log.d(TAG, "No text candidates event=$event app=$appName key=${item.key}")
            return
        }
        Log.d(TAG, "Inspecting notification event=$event app=$appName key=${item.key} candidates=${candidates.size}")

        for (candidate in candidates) {
            val otp = OtpExtractor.extractOtp(candidate.body)
            val hasKeyword = OtpExtractor.hasKeyword(candidate.context, SettingsStore.OTP_KEYWORDS)
            val hasIssuer = OtpExtractor.hasKeyword(candidate.context, SettingsStore.GOPAY_ISSUER_KEYWORDS)
            if (otp == null || !hasKeyword || !hasIssuer) {
                Log.d(
                    TAG,
                    "Candidate ignored event=$event otp=${otp != null} keyword=$hasKeyword issuer=$hasIssuer text=${candidate.context.debugPreview()}",
                )
                continue
            }
            val fingerprint = "${item.packageName}:$otp:${candidate.body.hashCode()}"
            if (!rememberForwardedFingerprint(fingerprint)) {
                Log.d(TAG, "Duplicate OTP candidate skipped event=$event key=${item.key} otp_len=${otp.length}")
                continue
            }

            OtpForwardWorker.enqueue(applicationContext, appSettings.webhookUrl, otp)
            Log.i(TAG, "Queued WhatsApp OTP forward event=$event app=$appName key=${item.key} otp_len=${otp.length}")
        }
    }

    override fun onListenerDisconnected() {
        super.onListenerDisconnected()
        NotificationListenerRebinder.request(applicationContext)
    }

    private fun resolveAppName(packageName: String): String {
        return runCatching {
            val appInfo = packageManager.getApplicationInfo(packageName, 0)
            packageManager.getApplicationLabel(appInfo).toString()
        }.getOrDefault(packageName)
    }

    @Suppress("DEPRECATION")
    private fun extractCandidates(notification: Notification): List<MessageCandidate> {
        val extras = notification.extras ?: return emptyList()
        val title = extras.getCharSequence(Notification.EXTRA_TITLE)?.toString().orEmpty()
        val subText = extras.getCharSequence(Notification.EXTRA_SUB_TEXT)?.toString().orEmpty()
        val summary = extras.getCharSequence(Notification.EXTRA_SUMMARY_TEXT)?.toString().orEmpty()
        val candidates = linkedSetOf<MessageCandidate>()

        fun add(candidateTitle: String, body: CharSequence?) {
            val text = body?.toString()?.trim().orEmpty()
            if (text.isBlank()) {
                return
            }
            val mergedTitle = candidateTitle.ifBlank { title }.trim()
            val mergedText = listOf(mergedTitle, subText, summary, text)
                .filter { it.isNotBlank() }
                .joinToString("\n")
            candidates += MessageCandidate(title = mergedTitle, body = text, context = mergedText)
        }

        add(title, extras.getCharSequence(Notification.EXTRA_TEXT))
        add(title, extras.getCharSequence(Notification.EXTRA_BIG_TEXT))
        extras.getCharSequenceArray(Notification.EXTRA_TEXT_LINES)
            ?.forEach { line -> add(title, line) }

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            val messages = Notification.MessagingStyle.Message.getMessagesFromBundleArray(
                extras.getParcelableArray(Notification.EXTRA_MESSAGES),
            )
            messages.forEach { message ->
                val sender = message.sender?.toString().orEmpty()
                add(sender.ifBlank { title }, message.text)
            }
        }

        return candidates.toList()
    }

    private fun rememberForwardedFingerprint(value: String): Boolean {
        if (!forwardedFingerprints.add(value)) {
            return false
        }
        while (forwardedFingerprints.size > MAX_FORWARDED_FINGERPRINTS) {
            val first = forwardedFingerprints.firstOrNull() ?: break
            forwardedFingerprints.remove(first)
        }
        return true
    }

    private fun String.debugPreview(): String {
        return replace(Regex("""\d"""), "#")
            .replace("\n", " ")
            .take(160)
    }

    private data class MessageCandidate(
        val title: String,
        val body: String,
        val context: String,
    )

    companion object {
        private const val TAG = "WhatsAppForwarder"
        private const val MAX_FORWARDED_FINGERPRINTS = 200
    }
}
