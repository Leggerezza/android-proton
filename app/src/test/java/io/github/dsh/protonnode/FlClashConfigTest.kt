package io.github.dsh.protonnode

import io.github.dsh.protonnode.data.FlClashConfig
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class FlClashConfigTest {
    private val self = "io.github.dsh.protonnode"

    @Test
    fun generatedConfigPassesItsOwnValidation() {
        val yaml = FlClashConfig.generate(self)
        assertEquals(emptyList<String>(), FlClashConfig.validate(yaml, self))
    }

    @Test
    fun loopGuardIsTheFirstRule() {
        val yaml = FlClashConfig.generate(self)
        val rules =
            yaml
                .substringAfter(
                    "rules:",
                ).lineSequence()
                .map { it.trim() }
                .filter { it.startsWith("- ") }
                .toList()
        assertEquals("- PROCESS-NAME,$self,DIRECT", rules.first())
        assertTrue(yaml.contains("PROCESS-NAME," + FlClashConfig.GAME_PACKAGE + ",GAME"))
    }

    @Test
    fun socks5HasUdpForUdpAssociate() {
        val yaml = FlClashConfig.generate(self)
        assertTrue(yaml.contains("type: socks5"))
        assertTrue(yaml.contains("udp: true"))
        assertTrue(yaml.contains("port: 1080"))
    }

    @Test
    fun aConfigWithoutTheGuardIsRejected() {
        val yaml = FlClashConfig.generate(self).replace("- PROCESS-NAME,$self,DIRECT\n", "")
        val problems = FlClashConfig.validate(yaml, self)
        assertTrue(problems.toString(), problems.any { it.contains("loop guard") })
    }

    @Test
    fun aConfigWithTheGuardAfterMatchIsRejected() {
        val yaml =
            FlClashConfig
                .generate(self)
                .replace("- PROCESS-NAME,$self,DIRECT\n", "")
                .replace("  - MATCH,PROXY", "  - MATCH,PROXY\n  - PROCESS-NAME,$self,DIRECT")
        val problems = FlClashConfig.validate(yaml, self)
        assertTrue(problems.toString(), problems.any { it.contains("after the loop guard") })
    }

    @Test
    fun udpDisabledIsReported() {
        val yaml = FlClashConfig.generate(self).replace("udp: true", "udp: false")
        val problems = FlClashConfig.validate(yaml, self)
        assertTrue(problems.toString(), problems.any { it.contains("udp must be true") })
    }
}
