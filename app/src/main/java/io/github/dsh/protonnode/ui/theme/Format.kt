package io.github.dsh.protonnode.ui.theme

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
