// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.leggerezza.protonnode

import io.github.leggerezza.protonnode.core.SessionInfo
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SessionInfoTest {
    @Test
    fun validSessionIsUsable() {
        val info =
            SessionInfo.from(
                """{"loggedIn":true,"email":"a***@x.com","authState":"valid","lastVerifiedAt":"2026-11-20T04:14:24Z"}""",
            )
        assertTrue(info.loggedIn)
        assertTrue(info.isVerified)
        assertTrue(info.isUsable)
        assertFalse(info.isExpired)
    }

    @Test
    fun expiredSessionIsNotUsable() {
        val info =
            SessionInfo.from(
                """{"loggedIn":true,"authState":"expired","authError":"proton api: http 401"}""",
            )
        assertTrue(info.isExpired)
        assertFalse(info.isUsable)
        assertEquals("proton api: http 401", info.authError)
    }

    @Test
    fun restoredSessionStartsUnverified() {
        val info = SessionInfo.from("""{"loggedIn":true,"authState":"unknown"}""")
        assertTrue(info.loggedIn)
        assertFalse(info.isExpired)
        assertFalse(info.isVerified)
        // Unknown sessions may still be used; the first API call settles it.
        assertTrue(info.isUsable)
    }

    @Test
    fun missingOrGarbagePayloadIsSignedOut() {
        assertFalse(SessionInfo.from("").loggedIn)
        assertFalse(SessionInfo.from("not json").loggedIn)
        assertFalse(SessionInfo.from("{}").loggedIn)
    }
}
