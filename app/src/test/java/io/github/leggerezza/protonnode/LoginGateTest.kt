// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.leggerezza.protonnode

import io.github.leggerezza.protonnode.core.LoginGate
import io.github.leggerezza.protonnode.core.looksRateLimited
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class LoginGateTest {
    @Test
    fun firstAttemptIsAllowed() {
        val gate = LoginGate()
        assertTrue(gate.canAttempt(0))
        assertEquals(0, gate.remainingMs(0))
    }

    @Test
    fun failureStartsACooldownThatExpires() {
        val gate = LoginGate(baseCooldownMs = 30_000)
        val wait = gate.onFailure(1_000, rateLimited = false)
        assertEquals(30_000, wait)
        assertFalse(gate.canAttempt(10_000))
        assertEquals(21_000, gate.remainingMs(10_000))
        assertTrue(gate.canAttempt(31_000))
    }

    @Test
    fun rateLimitingCoolsDownLonger() {
        val gate = LoginGate(baseCooldownMs = 30_000, rateLimitCooldownMs = 120_000)
        assertEquals(120_000, gate.onFailure(0, rateLimited = true))
    }

    @Test
    fun repeatedFailuresBackOffUpToTheCap() {
        val gate = LoginGate(baseCooldownMs = 30_000, maxCooldownMs = 120_000)
        assertEquals(30_000, gate.onFailure(0, false))
        assertEquals(60_000, gate.onFailure(0, false))
        assertEquals(120_000, gate.onFailure(0, false))
        assertEquals(120_000, gate.onFailure(0, false))
        assertEquals(4, gate.failureCount())
    }

    @Test
    fun successClearsTheCooldown() {
        val gate = LoginGate()
        gate.onFailure(0, true)
        gate.onSuccess()
        assertTrue(gate.canAttempt(1))
    }

    @Test
    fun rateLimitDetectionMatchesProtonsWording() {
        assertTrue(looksRateLimited("proton api: http 429, code 429: Too many requests"))
        assertTrue(looksRateLimited("Too Many Requests"))
        assertTrue(looksRateLimited("rate limited, try later"))
        assertFalse(looksRateLimited("proton api: http 401, code 401: Invalid access token"))
    }
}
