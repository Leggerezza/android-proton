package io.github.dsh.protonnode.data

import android.content.Context

/**
 * Small preference store for the node.
 *
 * P4 keeps this on SharedPreferences; the security document lists DataStore as
 * the target. The migration is tracked for P5 (it must happen before the
 * settings screen is written) — the API here is already suspend-friendly so the
 * swap stays local.
 */
class Prefs(
    context: Context,
) {
    private val prefs = context.applicationContext.getSharedPreferences("node", Context.MODE_PRIVATE)

    var autoStart: Boolean
        get() = prefs.getBoolean(KEY_AUTOSTART, false)
        set(value) = prefs.edit().putBoolean(KEY_AUTOSTART, value).apply()

    var lastConfigJson: String
        get() = prefs.getString(KEY_LAST_CONFIG, DEFAULT_CONFIG) ?: DEFAULT_CONFIG
        set(value) = prefs.edit().putString(KEY_LAST_CONFIG, value).apply()

    var apiDns: String
        get() = prefs.getString(KEY_API_DNS, "") ?: ""
        set(value) = prefs.edit().putString(KEY_API_DNS, value).apply()

    var networkFingerprint: String
        get() = prefs.getString(KEY_FINGERPRINT, "") ?: ""
        set(value) = prefs.edit().putString(KEY_FINGERPRINT, value).apply()

    var wantRunning: Boolean
        get() = prefs.getBoolean(KEY_WANT_RUNNING, false)
        set(value) = prefs.edit().putBoolean(KEY_WANT_RUNNING, value).apply()

    companion object {
        /** Game mode by default: the honest choice for the target device (PROMPT.md 7). */
        const val DEFAULT_CONFIG = "{\"mode\":\"game\",\"socksPort\":1080}"

        private const val KEY_AUTOSTART = "auto_start"
        private const val KEY_LAST_CONFIG = "last_config"
        private const val KEY_API_DNS = "api_dns"
        private const val KEY_FINGERPRINT = "network_fingerprint"
        private const val KEY_WANT_RUNNING = "want_running"
    }
}
