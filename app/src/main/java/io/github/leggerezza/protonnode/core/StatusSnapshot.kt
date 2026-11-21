// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.leggerezza.protonnode.core

import org.json.JSONObject

/**
 * Parsed form of the JSON that `protonnode.Protonnode.status()` returns.
 *
 * The Go side is the single source of truth (PROMPT.md 5.1); this class only
 * reads the fields the notification needs and never invents values.
 */
data class StatusSnapshot(
    val state: String,
    val server: String,
    val entryIP: String,
    val sni: String,
    val handshakeMs: Long,
    val uptimeSec: Long,
    val rxBytes: Long,
    val txBytes: Long,
    val lastError: String,
    val udp: Boolean,
    val socksPort: Int,
    val mode: String,
) {
    val isRunning: Boolean get() = state != STATE_IDLE
    val isConnected: Boolean get() = state == STATE_CONNECTED

    companion object {
        const val STATE_IDLE = "idle"
        const val STATE_SCANNING = "scanning"
        const val STATE_CONNECTING = "connecting"
        const val STATE_CONNECTED = "connected"
        const val STATE_RECONNECTING = "reconnecting"

        val idle =
            StatusSnapshot(
                state = STATE_IDLE,
                server = "",
                entryIP = "",
                sni = "",
                handshakeMs = 0,
                uptimeSec = 0,
                rxBytes = 0,
                txBytes = 0,
                lastError = "",
                udp = true,
                socksPort = 0,
                mode = "",
            )

        /** Parses the Go payload; a malformed payload degrades to [idle]. */
        fun from(json: String): StatusSnapshot {
            if (json.isBlank()) return idle
            return try {
                val o = JSONObject(json)
                StatusSnapshot(
                    state = o.optString("state", STATE_IDLE),
                    server = o.optString("server", ""),
                    entryIP = o.optString("entryIP", ""),
                    sni = o.optString("sni", ""),
                    handshakeMs = o.optLong("handshakeMs", 0),
                    uptimeSec = o.optLong("uptimeSec", 0),
                    rxBytes = o.optLong("rxBytes", 0),
                    txBytes = o.optLong("txBytes", 0),
                    lastError = o.optString("lastError", ""),
                    udp = o.optBoolean("udp", true),
                    socksPort = o.optInt("socksPort", 0),
                    mode = o.optString("mode", ""),
                )
            } catch (e: Exception) {
                idle.copy(lastError = "unparseable status: " + e.message)
            }
        }
    }
}
