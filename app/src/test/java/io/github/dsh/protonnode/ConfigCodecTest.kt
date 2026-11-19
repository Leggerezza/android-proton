package io.github.dsh.protonnode

import io.github.dsh.protonnode.data.ConfigCodec
import io.github.dsh.protonnode.data.NodeProfile
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class ConfigCodecTest {
    private val profile =
        NodeProfile(
            mode = "game",
            socksPort = 1080,
            theme = "dark",
            dynamicColor = false,
            autostart = true,
            apiDns = "172.26.26.3",
        )

    @Test
    fun roundTripRestoresTheProfile() {
        val json = ConfigCodec.export(profile, "{}")
        assertFalse("the export must not carry credentials", json.contains("token", ignoreCase = true))
        assertFalse(json.contains("password", ignoreCase = true))
        val parsed = ConfigCodec.parse(json)
        assertNotNull(parsed)
        assertEquals(profile, parsed)
    }

    @Test
    fun tamperedPayloadIsRejected() {
        val json = ConfigCodec.export(profile, "{}")
        val tampered = json.replace("\"dark\"", "\"light\"")
        assertTrue(tampered != json)
        assertNull(ConfigCodec.parse(tampered))
    }

    @Test
    fun wrongVersionOrGarbageIsRejected() {
        assertNull(ConfigCodec.parse("not json"))
        val json = ConfigCodec.export(profile, "{}")
        val other = json.replace("\"version\":1", "\"version\":99")
        assertNull(ConfigCodec.parse(other))
    }
}
