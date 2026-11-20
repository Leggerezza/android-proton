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

    /** Tunnel mode: "game" (default) or "normal" (PROMPT.md 7). */
    var mode: String
        get() = prefs.getString(KEY_MODE, "game") ?: "game"
        set(value) = prefs.edit().putString(KEY_MODE, value).apply()

    /** Lock the server while in game mode (PROMPT.md 7, default on). */
    var gameLock: Boolean
        get() = prefs.getBoolean(KEY_GAME_LOCK, true)
        set(value) = prefs.edit().putBoolean(KEY_GAME_LOCK, value).apply()

    var themeMode: String
        get() = prefs.getString(KEY_THEME, "system") ?: "system"
        set(value) = prefs.edit().putString(KEY_THEME, value).apply()

    var dynamicColor: Boolean
        get() = prefs.getBoolean(KEY_DYNAMIC, true)
        set(value) = prefs.edit().putBoolean(KEY_DYNAMIC, value).apply()

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
        private const val KEY_THEME = "theme_mode"
        private const val KEY_MODE = "tunnel_mode"
        private const val KEY_GAME_LOCK = "game_lock"
        private const val KEY_DYNAMIC = "dynamic_color"
    }
}
