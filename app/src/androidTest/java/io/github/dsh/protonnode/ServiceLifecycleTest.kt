package io.github.dsh.protonnode

import android.app.NotificationManager
import android.content.Context
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import io.github.dsh.protonnode.core.CoreController
import io.github.dsh.protonnode.service.NotificationFactory
import io.github.dsh.protonnode.service.ProtonService
import kotlinx.coroutines.runBlocking
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

    @Test
    fun startBringsTheServiceUpAndStopTakesItDown() {
        ProtonService.start(context)
        awaitCondition(10_000) { ProtonService.running }
        assertTrue("the service should be running", ProtonService.running)

        ProtonService.stop(context)
        awaitCondition(10_000) { !ProtonService.running }
        assertFalse("the service should stop within 10s", ProtonService.running)

        val status = runBlocking { CoreController.status() }
        assertEquals("idle", status.state)
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
