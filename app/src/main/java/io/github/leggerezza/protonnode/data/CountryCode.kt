package io.github.leggerezza.protonnode.data

/**
 * Country codes come from the user, so they must be normalised and validated
 * before they reach the core (which filters servers by ISO-3166 alpha-2).
 *
 * A region form is accepted too — [A-Z]{2}-[A-Z]{2}, e.g. US-CA — because the
 * logical server names carry the state (US-CA#12) and a west-coast exit is
 * ~60 ms closer to Asia than US-CO/US-NY.
 *
 * Returns "" for "automatic", the normalised code, or null when the input is
 * not a country or region — the UI shows an error instead of sending a query
 * that can never match.
 */
fun normalizeCountryCode(raw: String): String? {
    val code = raw.trim().uppercase()
    if (code.isEmpty()) return ""
    val parts = code.split('-')
    if (parts.size > 2) return null
    for (part in parts) {
        if (part.length != 2) return null
        if (part.any { it !in 'A'..'Z' }) return null
    }
    return parts.joinToString("-")
}

/** True when [code] appears in the loaded server list (used for a soft warning). */
fun countryExists(
    code: String,
    known: Collection<String>,
): Boolean {
    val country = code.substringBefore('-')
    return country.isEmpty() || known.any { it.equals(country, ignoreCase = true) }
}
