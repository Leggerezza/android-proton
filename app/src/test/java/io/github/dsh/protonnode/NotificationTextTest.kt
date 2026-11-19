package io.github.dsh.protonnode

import io.github.dsh.protonnode.core.StatusSnapshot
import io.github.dsh.protonnode.net.NetworkMonitor
import io.github.dsh.protonnode.service.NotificationFactory
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class NotificationTextTest {
    private fun status(
        state: String = StatusSnapshot.STATE_CONNECTED,
        server: String = "JP#390",
        port: Int = 1080,
        rx: Long = 1_572_864,
        tx: Long = 2048,
        uptime: Long = 3725,
        mode: String = "game",
        lastError: String = "",
    ) = StatusSnapshot(
        state = state,
        server = server,
        entryIP = "212.102.51.121",
        sni = "gateway.icloud.com",
        handshakeMs = 250,
        uptimeSec = uptime,
        rxBytes = rx,
        txBytes = tx,
        lastError = lastError,
        udp = true,
        socksPort = port,
        mode = mode,
    )

    @Test
    fun collapsedTextFollowsTheState() {
        assertEquals("connected · JP#390", NotificationFactory.textFor(status()))
        assertEquals("scanning for a server", NotificationFactory.textFor(status(StatusSnapshot.STATE_SCANNING)))
        assertEquals("reconnecting", NotificationFactory.textFor(status(StatusSnapshot.STATE_RECONNECTING)))
        assertEquals("stopped", NotificationFactory.textFor(status(StatusSnapshot.STATE_IDLE)))
    }

    @Test
    fun detailedTextShowsPortTrafficAndUptime() {
        val text = NotificationFactory.detailedFor(status())
        assertTrue(text, text.contains("socks 127.0.0.1:1080"))
        assertTrue(text, text.contains("game"))
        assertTrue(text, text.contains("↓ 1.5 MB"))
        assertTrue(text, text.contains("↑ 2.0 KB"))
        assertTrue(text, text.contains("1h 02m"))
    }

    @Test
    fun detailedTextShowsTheLastErrorWhenNotConnected() {
        val text =
            NotificationFactory.detailedFor(
                status(state = StatusSnapshot.STATE_RECONNECTING, lastError = "proton api: http 401"),
            )
        assertTrue(text, text.contains("401"))
    }

    @Test
    fun byteAndUptimeFormatting() {
        assertEquals("0 B", NotificationFactory.bytes(0))
        assertEquals("512 B", NotificationFactory.bytes(512))
        assertEquals("1.0 KB", NotificationFactory.bytes(1024))
        assertEquals("1.5 MB", NotificationFactory.bytes(1_572_864))
        assertEquals("just started", NotificationFactory.uptime(0))
        assertEquals("45s", NotificationFactory.uptime(45))
        assertEquals("2m 00s", NotificationFactory.uptime(120))
        assertEquals("1h 00m", NotificationFactory.uptime(3600))
    }

    @Test
    fun fingerprintIsTransportPlusHandle() {
        assertEquals("wifi:12345", NetworkMonitor.fingerprintOf("wifi", 12345))
        assertEquals("cellular:7", NetworkMonitor.fingerprintOf("cellular", 7))
    }
}
