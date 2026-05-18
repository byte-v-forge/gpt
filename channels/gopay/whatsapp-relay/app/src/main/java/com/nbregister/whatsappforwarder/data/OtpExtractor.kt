package com.nbregister.whatsappforwarder.data

object OtpExtractor {
    private val otpRegex = Regex("""(?:^|\D)(\d{4,8})(?!\d)""")

    fun extractOtp(text: String): String? {
        val matches = otpRegex.findAll(text).mapNotNull { it.groupValues.getOrNull(1) }.toList()
        return matches.firstOrNull { it.length == 6 } ?: matches.firstOrNull()
    }

    fun hasKeyword(text: String, keywords: Set<String>): Boolean {
        if (keywords.isEmpty()) {
            return true
        }
        val haystack = text.lowercase()
        return keywords.any { keyword -> keyword.isNotBlank() && haystack.contains(keyword.lowercase()) }
    }
}
