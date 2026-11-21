// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.leggerezza.protonnode.ui.theme

import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.Locale

/** Human-readable byte count for the UI (binary units, one decimal). */
fun formatBytes(value: Long): String {
    if (value < 1024) return value.toString() + " B"
    val units = arrayOf("KB", "MB", "GB", "TB")
    var v = value.toDouble() / 1024.0
    var unit = 0
    while (v >= 1024 && unit < units.size - 1) {
        v /= 1024.0
        unit++
    }
    return String.format(Locale.US, "%.1f %s", v, units[unit])
}

/** "1h 02m" / "2m 05s" / "42s". */
fun formatUptime(seconds: Long): String {
    if (seconds <= 0) return "—"
    val h = seconds / 3600
    val m = (seconds % 3600) / 60
    val s = seconds % 60
    return when {
        h > 0 -> String.format(Locale.US, "%dh %02dm", h, m)
        m > 0 -> String.format(Locale.US, "%dm %02ds", m, s)
        else -> String.format(Locale.US, "%ds", s)
    }
}

/** RFC3339 from the core -> local "yyyy-MM-dd HH:mm"; falls back to the raw value. */
fun formatTimestamp(iso: String): String {
    if (iso.isEmpty()) return "—"
    return runCatching {
        DateTimeFormatter
            .ofPattern("yyyy-MM-dd HH:mm")
            .withZone(ZoneId.systemDefault())
            .format(Instant.parse(iso))
    }.getOrDefault(iso)
}
