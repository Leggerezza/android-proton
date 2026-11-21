// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.leggerezza.protonnode

import io.github.leggerezza.protonnode.data.buildStartConfig
import io.github.leggerezza.protonnode.ui.theme.formatTimestamp
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class StartConfigTest {
    @Test
    fun gameModeLocksTheServerByDefault() {
        val cfg = JSONObject(buildStartConfig("game", gameLock = true))
        assertEquals("game", cfg.getString("mode"))
        assertEquals(1080, cfg.getInt("socksPort"))
        assertTrue(cfg.getBoolean("lockServer"))
    }

    @Test
    fun normalModeNeverLocks() {
        val cfg = JSONObject(buildStartConfig("normal", gameLock = true))
        assertEquals("normal", cfg.getString("mode"))
        assertFalse(cfg.getBoolean("lockServer"))
    }

    @Test
    fun gameLockCanBeDisabled() {
        assertFalse(JSONObject(buildStartConfig("game", gameLock = false)).getBoolean("lockServer"))
    }

    @Test
    fun aPinnedServerWinsOverACountry() {
        val cfg = JSONObject(buildStartConfig("game", true, 1080, server = "JP#377", country = "us"))
        assertEquals("JP#377", cfg.getString("server"))
        assertFalse(cfg.has("country"))
    }

    @Test
    fun aCountryIsUppercased() {
        val cfg = JSONObject(buildStartConfig("normal", false, 1080, country = "jp"))
        assertEquals("JP", cfg.getString("country"))
        assertFalse(cfg.has("server"))
    }

    @Test
    fun autoAddsNeitherField() {
        val cfg = JSONObject(buildStartConfig("game", true))
        assertFalse(cfg.has("server"))
        assertFalse(cfg.has("country"))
    }

    @Test
    fun unknownModeFallsBackToGame() {
        assertEquals("game", JSONObject(buildStartConfig("weird", gameLock = false)).getString("mode"))
    }

    @Test
    fun verifiedTimestampIsHumanReadable() {
        // The exact clock depends on the zone, but the shape must be stable.
        val out = formatTimestamp("2026-11-20T04:43:31Z")
        assertTrue(out, Regex("\\d{4}-\\d{2}-\\d{2} \\d{2}:\\d{2}").matches(out))
        assertEquals("—", formatTimestamp(""))
    }
}
