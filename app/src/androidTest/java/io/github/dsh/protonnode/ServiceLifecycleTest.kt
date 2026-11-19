package io.github.dsh.protonnode

import android.app.NotificationManager
import android.content.Context
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import io.github.dsh.protonnode.core.CoreController
import io.github.dsh.protonnode.service.NotificationFactory
import io.github.dsh.protonnode.data.SessionStore
import io.github.dsh.protonnode.service.ProtonService
import kotlinx.coroutines.runBlocking
import java.net.InetSocketAddress
import java.net.Socket
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Foreground-service lifecycle (PROMPT.md 5.2 / 6.2). Runs on a device or the
 * 16 KB emulator; needs no Proton account: the assertions are about the service
 * contract, not about reaching Proton.
 */
@RunWith(AndroidJUnit4::class)
class ServiceLifecycleTest {
    private val context: Context = InstrumentationRegistry.getInstrumentation().targetContext

    @After
    fun cleanUp() {
        SessionStore(context).clear()
        ProtonService.stop(context)
        awaitCondition(5_000) { !ProtonService.running }
    }

    @Test
    fun startBringsTheServiceUpAndStopTakesItDown() {
        // A syntactically valid session makes the state machine actually run: the
        // API rejects the fake token, so we expect scanning/reconnecting, never a
        // silent idle.
        SessionStore(context).save(
            """{"uid":"test","access_token":"fake","refresh_token":"fake","device_name":"test-device","ed25519_seed":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}""",
        )

        ProtonService.start(context)
        awaitCondition(10_000) { ProtonService.running }
        assertTrue("the service should be running", ProtonService.running)

        awaitCondition(15_000) {
            runBlocking { CoreController.status().state != "idle" }
        }
        val running = runBlocking { CoreController.status() }
        assertTrue("expected a non-idle state while running, got " + running.state, running.state != "idle")

        ProtonService.stop(context)
        awaitCondition(10_000) { !ProtonService.running }
        assertFalse("the service should stop within 10s", ProtonService.running)

        awaitCondition(5_000) { runBlocking { CoreController.status().state == "idle" } }
        val status = runBlocking { CoreController.status() }
        assertEquals("idle", status.state)

        // No listener left behind after Stop (PROMPT.md 5.2).
        val refused = runCatching {
            Socket().use { it.connect(InetSocketAddress("127.0.0.1", 1080), 500) }
        }.isFailure
        assertTrue("127.0.0.1:1080 should be closed after Stop", refused)
    }

    @Test
    fun notificationChannelExists() {
        NotificationFactory.ensureChannel(context)
        val manager = context.getSystemService(NotificationManager::class.java)
        assertNotNull(manager.getNotificationChannel(NotificationFactory.CHANNEL_ID))
    }

    private fun awaitCondition(
        timeoutMs: Long,
        condition: () -> Boolean,
    ) {
        val deadline = System.currentTimeMillis() + timeoutMs
        while (System.currentTimeMillis() < deadline) {
            if (condition()) return
            Thread.sleep(100)
        }
    }
}
