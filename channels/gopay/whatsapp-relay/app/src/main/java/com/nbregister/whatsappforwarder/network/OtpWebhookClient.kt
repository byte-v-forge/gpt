package com.nbregister.whatsappforwarder.network

import com.google.gson.Gson
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import java.util.concurrent.TimeUnit

data class SendResult(
    val success: Boolean,
    val permanentFailure: Boolean,
    val message: String,
)

class OtpWebhookClient {
    private val gson = Gson()
    private val client = OkHttpClient.Builder()
        .connectTimeout(15, TimeUnit.SECONDS)
        .readTimeout(20, TimeUnit.SECONDS)
        .writeTimeout(20, TimeUnit.SECONDS)
        .build()

    fun send(url: String, otp: String): SendResult {
        return try {
            val payload = mapOf(
                "otp" to otp,
                "source" to "whatsapp",
            )
            val body = gson.toJson(payload).toRequestBody(JSON.toMediaType())
            val request = Request.Builder()
                .url(url)
                .post(body)
                .header("Content-Type", JSON)
                .build()

            client.newCall(request).execute().use { response ->
                if (response.isSuccessful) {
                    SendResult(success = true, permanentFailure = false, message = "OK")
                } else {
                    val permanent = response.code in 400..499 && response.code != 408 && response.code != 429
                    SendResult(
                        success = false,
                        permanentFailure = permanent,
                        message = "HTTP ${response.code}",
                    )
                }
            }
        } catch (exc: Exception) {
            SendResult(success = false, permanentFailure = false, message = exc.message ?: "network error")
        }
    }

    companion object {
        private const val JSON = "application/json; charset=utf-8"
    }
}
