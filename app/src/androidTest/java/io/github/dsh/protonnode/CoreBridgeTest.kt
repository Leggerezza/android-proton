package io.github.dsh.protonnode

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import protonnode.Protonnode

/**
 * P1 smoke test for the gomobile bridge.
 *
 * It answers the question the AAR build cannot: does the Go runtime (utls +
 * wireguard-go + gVisor netstack) actually load and run inside an Android app
 * process? No network is touched here; the tunnel itself is covered by the
 * device CLI evidence in docs/TEST-EVIDENCE.md.
 */
@RunWith(AndroidJUnit4::class)
class CoreBridgeTest {
    @Test
    fun initStatusAndShutdownWork() {
        val ctx = InstrumentationRegistry.getInstrumentation().targetContext
        val dir = ctx.filesDir.absolutePath + "/core"

        Protonnode.init(dir)
        val status = Protonnode.status()
        assertTrue("status should be JSON with an idle state, got: $status", status.contains("\"state\":\"idle\""))

        val logs = Protonnode.recentLogs(20L)
        assertTrue("the core should log its initialisation, got: $logs", logs.contains("initialized"))

        val session = Protonnode.sessionState()
        assertTrue("sessionState should be JSON, got: $session", session.contains("loggedIn"))

        Protonnode.shutdown()
    }
}
