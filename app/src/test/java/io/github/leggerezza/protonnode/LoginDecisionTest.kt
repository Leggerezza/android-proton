// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.leggerezza.protonnode

import io.github.leggerezza.protonnode.core.LoginDecision
import io.github.leggerezza.protonnode.core.SessionInfo
import io.github.leggerezza.protonnode.core.decideLogin
import org.junit.Assert.assertEquals
import org.junit.Test

/** The rule the user reported: a valid session must not hit Proton again. */
class LoginDecisionTest {
    private fun info(
        state: String,
        loggedIn: Boolean = true,
    ) = SessionInfo.from(
        "{\"loggedIn\":" + loggedIn + ",\"authState\":\"" + state + "\"}",
    )

    @Test
    fun aValidSessionNeverReachesProton() {
        assertEquals(
            LoginDecision.ALREADY_SIGNED_IN,
            decideLogin(info("valid"), busy = false, cooldownRemainingMs = 0, email = "a@b.c", password = "x"),
        )
    }

    @Test
    fun anUnknownSessionIsTreatedAsSignedIn() {
        // Restored from disk: tokens exist, so no new login should be sent.
        assertEquals(
            LoginDecision.ALREADY_SIGNED_IN,
            decideLogin(info("unknown"), busy = false, cooldownRemainingMs = 0, email = "a@b.c", password = "x"),
        )
    }

    @Test
    fun anExpiredSessionMaySignInAgain() {
        assertEquals(
            LoginDecision.PROCEED,
            decideLogin(info("expired"), busy = false, cooldownRemainingMs = 0, email = "a@b.c", password = "x"),
        )
    }

    @Test
    fun cooldownBusyAndMissingFieldsBlockTheRequest() {
        val out = SessionInfo.signedOut
        assertEquals(LoginDecision.COOLDOWN, decideLogin(out, false, 5_000, "a", "b"))
        assertEquals(LoginDecision.COOLDOWN, decideLogin(out, true, 0, "a", "b"))
        assertEquals(LoginDecision.MISSING_FIELDS, decideLogin(out, false, 0, "", "b"))
        assertEquals(LoginDecision.PROCEED, decideLogin(out, false, 0, "a", "b"))
    }
}
