// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.leggerezza.protonnode.core

/** Severity attached to a node log line. */
enum class LogLevel(
    val label: String,
) {
    INFO("INFO"),
    WARN("WARN"),
    ERROR("ERROR"),
}

/**
 * Classifies one core log line.
 *
 * The Go core writes plain sentences, so the level is derived from what the line
 * says: a lost tunnel or a refused connect is an error, a retry or a degraded
 * fallback is a warning, everything else is informational. The mapping is
 * deliberately explicit and covered by tests so the Logs screen cannot quietly
 * change its colour semantics.
 */
fun classifyLogLine(line: String): LogLevel {
    val text = line.lowercase()
    if (text.contains("judged dead") || text.contains("tunnel lost") || text.contains("panic")) {
        return LogLevel.ERROR
    }
    if (text.contains("health:")) {
        // "health: check failed (1/2)" is a warning; two in a row trigger the
        // "judged dead" error handled above.
        return LogLevel.WARN
    }
    // Retries and degraded fallbacks mention "error" in their text ("retry after
    // transport error"), so they must be classified before the error keywords.
    if (
        text.contains("reconnect") ||
        text.contains("retry") ||
        text.contains("skipped") ||
        text.contains("unavailable") ||
        text.contains("cached") ||
        text.contains("degraded") ||
        text.contains("fallback")
    ) {
        return LogLevel.WARN
    }
    if (
        text.contains("connect failed") ||
        text.contains(" failed") ||
        text.contains("failure") ||
        text.contains("no entry") ||
        text.contains("refused") ||
        text.contains("unreachable") ||
        text.contains("error")
    ) {
        return LogLevel.ERROR
    }
    return LogLevel.INFO
}

/** True when [line] should be shown under [filter] (null = every level). */
fun logLineMatches(
    line: String,
    filter: LogLevel?,
): Boolean = filter == null || classifyLogLine(line) == filter
