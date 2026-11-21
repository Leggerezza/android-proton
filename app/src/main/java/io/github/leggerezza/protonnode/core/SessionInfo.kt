// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.leggerezza.protonnode.core

import org.json.JSONObject

/**
 * Parsed form of the Go `SessionState()` payload.
 *
 * [authState] is the verdict of the last authenticated call:
 * - "unknown": tokens are stored but nobody has confirmed them yet (app restart);
 * - "valid":   Proton accepted them;
 * - "expired": Proton rejected them (401/2001) — the UI must say so.
 */
data class SessionInfo(
    val loggedIn: Boolean,
    val email: String,
    val plan: String,
    val authState: String,
    val lastVerifiedAt: String,
    val authError: String,
    val deviceName: String,
) {
    val isExpired: Boolean get() = authState == STATE_EXPIRED
    val isVerified: Boolean get() = authState == STATE_VALID

    /** A session that can be used without asking Proton again. */
    val isUsable: Boolean get() = loggedIn && !isExpired

    companion object {
        const val STATE_UNKNOWN = "unknown"
        const val STATE_VALID = "valid"
        const val STATE_EXPIRED = "expired"

        val signedOut =
            SessionInfo(
                loggedIn = false,
                email = "",
                plan = "",
                authState = STATE_UNKNOWN,
                lastVerifiedAt = "",
                authError = "",
                deviceName = "",
            )

        fun from(json: String): SessionInfo {
            if (json.isBlank()) return signedOut
            return runCatching {
                val o = JSONObject(json)
                SessionInfo(
                    loggedIn = o.optBoolean("loggedIn", false),
                    email = o.optString("email", ""),
                    plan = o.optString("plan", ""),
                    authState = o.optString("authState", STATE_UNKNOWN).ifEmpty { STATE_UNKNOWN },
                    lastVerifiedAt = o.optString("lastVerifiedAt", ""),
                    authError = o.optString("authError", ""),
                    deviceName = o.optString("deviceName", ""),
                )
            }.getOrDefault(signedOut)
        }
    }
}
