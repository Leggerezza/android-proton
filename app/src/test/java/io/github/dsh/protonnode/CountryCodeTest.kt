package io.github.dsh.protonnode

import io.github.dsh.protonnode.data.countryExists
import io.github.dsh.protonnode.data.normalizeCountryCode
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class CountryCodeTest {
    @Test
    fun emptyMeansAutomatic() {
        assertEquals("", normalizeCountryCode(""))
        assertEquals("", normalizeCountryCode("   "))
    }

    @Test
    fun codesAreTrimmedAndUppercased() {
        assertEquals("JP", normalizeCountryCode("jp"))
        assertEquals("US", normalizeCountryCode(" uS "))
        assertEquals("DE", normalizeCountryCode("De"))
    }

    @Test
    fun invalidInputIsRejected() {
        assertNull(normalizeCountryCode("J"))
        assertNull(normalizeCountryCode("JPN"))
        assertNull(normalizeCountryCode("1A"))
        assertNull(normalizeCountryCode("J P"))
        assertNull(normalizeCountryCode("日本"))
    }

    @Test
    fun unknownCodesAreReportedButAllowed() {
        val known = listOf("JP", "US", "DE")
        assertTrue(countryExists("JP", known))
        assertTrue(countryExists("jp", known))
        assertFalse(countryExists("ZZ", known))
        // "" (automatic) never warns
        assertTrue(countryExists("", known))
    }
}
