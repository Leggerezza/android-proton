// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.leggerezza.protonnode.data

/**
 * Country codes come from the user, so they must be normalised and validated
 * before they reach the core (which filters servers by ISO-3166 alpha-2).
 *
 * Returns "" for "automatic", the upper-case code for a valid input, or null
 * when the input is not two letters — the UI shows an error instead of sending
 * a query that can never match.
 */
fun normalizeCountryCode(raw: String): String? {
    val code = raw.trim().uppercase()
    if (code.isEmpty()) return ""
    if (code.length != 2) return null
    if (!code.all { it in 'A'..'Z' }) return null
    return code
}

/** True when [code] appears in the loaded server list (used for a soft warning). */
fun countryExists(
    code: String,
    known: Collection<String>,
): Boolean = code.isEmpty() || known.any { it.equals(code, ignoreCase = true) }
