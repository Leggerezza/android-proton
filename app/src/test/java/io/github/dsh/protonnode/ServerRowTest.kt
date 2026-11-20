package io.github.dsh.protonnode

import io.github.dsh.protonnode.ui.screens.parseServers
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ServerRowTest {
    @Test
    fun featuresAreParsed() {
        val json =
            """
            [
              {"name":"JP#1","exitCountry":"JP","load":10,"score":1.0,"secureCore":false,"p2p":true,"tor":false,"streaming":false,"entryIP":"1.2.3.4"},
              {"name":"SE#2","exitCountry":"SE","load":20,"score":2.0,"secureCore":true,"p2p":false,"tor":true,"streaming":true}
            ]
            """.trimIndent()
        val rows = parseServers(json)
        assertEquals(2, rows.size)
        assertTrue(rows[0].p2p)
        assertFalse(rows[0].tor)
        assertTrue(rows[1].secureCore)
        assertTrue(rows[1].tor)
        assertTrue(rows[1].streaming)
    }

    @Test
    fun missingFeatureFieldsDefaultToFalse() {
        val rows = parseServers("""[{"name":"US#1","exitCountry":"US"}]""")
        assertEquals(1, rows.size)
        assertFalse(rows[0].p2p || rows[0].tor || rows[0].streaming || rows[0].secureCore)
    }

    @Test
    fun garbageIsEmpty() {
        assertTrue(parseServers("not json").isEmpty())
        assertTrue(parseServers("").isEmpty())
    }
}
