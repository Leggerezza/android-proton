// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.leggerezza.protonnode.core

import kotlin.math.min

/**
 * Client-side guard for the "Sign in" button.
 *
 * Users press it repeatedly when feedback is unclear; Proton answers with
 * "Too many requests" and the account gets throttled. This gate enforces a
 * cooldown after a failure (longer when the server said it is rate limiting)
 * and is bypassed entirely when a verified session already exists.
 */
class LoginGate(
    private val baseCooldownMs: Long = 30_000,
    private val rateLimitCooldownMs: Long = 120_000,
    private val maxCooldownMs: Long = 600_000,
) {
    private var cooldownUntil = 0L
    private var failures = 0

    fun remainingMs(nowMs: Long): Long = (cooldownUntil - nowMs).coerceAtLeast(0L)

    fun canAttempt(nowMs: Long): Boolean = remainingMs(nowMs) == 0L

    fun onSuccess() {
        failures = 0
        cooldownUntil = 0L
    }

    /** Records a failed attempt and returns the cooldown that now applies. */
    fun onFailure(
        nowMs: Long,
        rateLimited: Boolean,
    ): Long {
        failures += 1
        val base = if (rateLimited) rateLimitCooldownMs else baseCooldownMs
        val backoff = base * (1L shl min(failures - 1, 3))
        val wait = min(backoff, maxCooldownMs)
        cooldownUntil = nowMs + wait
        return wait
    }

    /** Test/diagnostic hook. */
    fun failureCount(): Int = failures
}

/** Proton's rate-limit answer, in the shapes the Go core renders it. */
fun looksRateLimited(message: String): Boolean {
    val m = message.lowercase()
    return m.contains("too many") || m.contains("429") || m.contains("rate limit")
}

/** What the "Sign in" button should do right now. */
enum class LoginDecision { ALREADY_SIGNED_IN, COOLDOWN, MISSING_FIELDS, PROCEED }

/**
 * The rule the user reported: a session that is still valid must never trigger
 * a Proton request — the button just says "already signed in".
 */
fun decideLogin(
    info: SessionInfo,
    busy: Boolean,
    cooldownRemainingMs: Long,
    email: String,
    password: String,
): LoginDecision =
    when {
        info.loggedIn && !info.isExpired -> LoginDecision.ALREADY_SIGNED_IN
        busy -> LoginDecision.COOLDOWN
        cooldownRemainingMs > 0 -> LoginDecision.COOLDOWN
        email.isBlank() || password.isBlank() -> LoginDecision.MISSING_FIELDS
        else -> LoginDecision.PROCEED
    }
