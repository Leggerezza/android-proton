// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.leggerezza.protonnode.data

import org.json.JSONObject
import java.security.MessageDigest

/** The user-visible settings that travel in an export (PROMPT.md 6.4). */
data class NodeProfile(
    val mode: String,
    val socksPort: Int,
    val theme: String,
    val dynamicColor: Boolean,
    val autostart: Boolean,
    val apiDns: String,
)

/**
 * Export/import of the configuration. The payload is
 * {version, profile, preferences, lastScan} and carries a SHA-256 checksum; no
 * token, password or key ever enters the document.
 */
object ConfigCodec {
    const val VERSION = 1

    fun export(
        profile: NodeProfile,
        lastScanJson: String,
    ): String {
        val payload =
            JSONObject()
                .put("version", VERSION)
                .put(
                    "profile",
                    JSONObject()
                        .put("mode", profile.mode)
                        .put("socksPort", profile.socksPort),
                ).put(
                    "preferences",
                    JSONObject()
                        .put("theme", profile.theme)
                        .put("dynamicColor", profile.dynamicColor)
                        .put("autostart", profile.autostart)
                        .put("apiDns", profile.apiDns),
                ).put("lastScan", JSONObject(lastScanJson.ifBlank { "{}" }))
        return JSONObject()
            .put("payload", payload)
            .put("sha256", checksum(payload.toString()))
            .toString()
    }

    /** Parses and validates an export; null means "reject, do not touch state". */
    fun parse(raw: String): NodeProfile? =
        runCatching {
            val root = JSONObject(raw)
            val payload = root.getJSONObject("payload")
            if (root.getString("sha256") != checksum(payload.toString())) return null
            if (payload.getInt("version") != VERSION) return null
            val prefs = payload.optJSONObject("preferences") ?: JSONObject()
            val profile = payload.optJSONObject("profile") ?: JSONObject()
            val mode = profile.optString("mode", "game")
            val theme = prefs.optString("theme", "system")
            if (mode != "normal" && mode != "game") return null
            if (theme !in setOf("system", "light", "dark")) return null
            NodeProfile(
                mode = mode,
                socksPort = profile.optInt("socksPort", 1080).takeIf { it in 1..65535 } ?: 1080,
                theme = theme,
                dynamicColor = prefs.optBoolean("dynamicColor", true),
                autostart = prefs.optBoolean("autostart", false),
                apiDns = prefs.optString("apiDns", ""),
            )
        }.getOrNull()

    fun checksum(body: String): String =
        MessageDigest
            .getInstance("SHA-256")
            .digest(body.toByteArray())
            .joinToString("") { "%02x".format(it) }
}

/**
 * Start configuration sent to the Go core. Game mode locks the server across
 * reconnects by default (PROMPT.md 7: changing it mid-game changes the exit IP
 * and drops every session).
 */
fun buildStartConfig(
    mode: String,
    gameLock: Boolean,
    socksPort: Int = 1080,
    server: String = "",
    country: String = "",
): String {
    val effectiveMode = if (mode == "normal") "normal" else "game"
    val json =
        JSONObject()
            .put("mode", effectiveMode)
            .put("socksPort", socksPort)
            .put("lockServer", effectiveMode == "game" && gameLock)
    // A pinned server wins over a country; the Go core still re-verifies it with L3.
    if (server.isNotBlank()) {
        json.put("server", server.trim())
    } else if (country.isNotBlank()) {
        json.put("country", country.trim().uppercase())
    }
    return json.toString()
}
