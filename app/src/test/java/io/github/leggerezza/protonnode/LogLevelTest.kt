// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.leggerezza.protonnode

import io.github.leggerezza.protonnode.core.LogLevel
import io.github.leggerezza.protonnode.core.classifyLogLine
import io.github.leggerezza.protonnode.core.logLineMatches
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class LogLevelTest {
    @Test
    fun connectionFailuresAreErrors() {
        assertEquals(
            LogLevel.ERROR,
            classifyLogLine("socks5: CONNECT failed (rep 0x4, family=name, dns)"),
        )
        assertEquals(LogLevel.ERROR, classifyLogLine("connect failed: no entry passed L3 verification"))
        assertEquals(LogLevel.ERROR, classifyLogLine("tunnel lost: read tcp: connection reset"))
    }

    @Test
    fun aDeadTunnelIsAnErrorButASingleHealthMissIsAWarning() {
        assertEquals(LogLevel.WARN, classifyLogLine("health: check failed (1/2)"))
        assertEquals(LogLevel.ERROR, classifyLogLine("health: tunnel judged dead"))
        assertEquals(LogLevel.WARN, classifyLogLine("health: recovered after 1 failure(s)"))
    }

    @Test
    fun retriesAndFallbacksAreWarnings() {
        assertEquals(LogLevel.WARN, classifyLogLine("reconnect: attempt 3 in 1m0s"))
        assertEquals(LogLevel.WARN, classifyLogLine("retry 2 after transport error: EOF"))
        assertEquals(
            LogLevel.WARN,
            classifyLogLine("servers: API unavailable (EOF); using 2784 cached servers"),
        )
    }

    @Test
    fun ordinaryProgressIsInfo() {
        assertEquals(LogLevel.INFO, classifyLogLine("tunnel up in 251ms (sni www.gstatic.com)"))
        assertEquals(LogLevel.INFO, classifyLogLine("servers: 2784 logical servers from the API"))
        assertEquals(LogLevel.INFO, classifyLogLine("scan scan-1: l1 200/200 reachable"))
    }

    @Test
    fun theFilterKeepsOnlyTheChosenLevel() {
        val line = "socks5: CONNECT failed (rep 0x4, family=name, dns)"
        assertTrue(logLineMatches(line, null))
        assertTrue(logLineMatches(line, LogLevel.ERROR))
        assertFalse(logLineMatches(line, LogLevel.INFO))
    }
}
