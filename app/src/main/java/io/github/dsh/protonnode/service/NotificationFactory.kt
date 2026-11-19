package io.github.dsh.protonnode.service

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import androidx.core.app.NotificationCompat
import io.github.dsh.protonnode.R
import io.github.dsh.protonnode.core.StatusSnapshot

/**
 * Builds the ongoing notification. It is the only UI in P4 (PROMPT.md 6.2):
 * state, server, traffic and a Stop action.
 */
object NotificationFactory {
    const val CHANNEL_ID = "node"
    const val NOTIFICATION_ID = 1001

    fun ensureChannel(context: Context) {
        val manager = context.getSystemService(NotificationManager::class.java) ?: return
        if (manager.getNotificationChannel(CHANNEL_ID) != null) return
        val channel =
            NotificationChannel(
                CHANNEL_ID,
                context.getString(R.string.channel_name),
                NotificationManager.IMPORTANCE_LOW,
            ).apply {
                description = context.getString(R.string.channel_description)
                setShowBadge(false)
                enableVibration(false)
                setSound(null, null)
            }
        manager.createNotificationChannel(channel)
    }

    fun build(
        context: Context,
        status: StatusSnapshot,
    ): Notification {
        val title = context.getString(R.string.app_name)
        val stopIntent = Intent(context, ProtonService::class.java).setAction(ProtonService.ACTION_STOP)
        val stopPending =
            PendingIntent.getService(
                context,
                1,
                stopIntent,
                PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
            )
        return NotificationCompat
            .Builder(context, CHANNEL_ID)
            .setSmallIcon(android.R.drawable.stat_sys_upload_done)
            .setContentTitle(title)
            .setContentText(textFor(status))
            .setStyle(NotificationCompat.BigTextStyle().bigText(detailedFor(status)))
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setShowWhen(false)
            .setSilent(true)
            .setCategory(NotificationCompat.CATEGORY_SERVICE)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .addAction(0, context.getString(R.string.action_stop), stopPending)
            .build()
    }

    /** One line for the collapsed notification. */
    fun textFor(status: StatusSnapshot): String =
        when (status.state) {
            StatusSnapshot.STATE_CONNECTED -> "connected · " + status.server
            StatusSnapshot.STATE_SCANNING -> "scanning for a server"
            StatusSnapshot.STATE_CONNECTING -> "connecting · " + status.server.ifEmpty { "…" }
            StatusSnapshot.STATE_RECONNECTING -> "reconnecting"
            StatusSnapshot.STATE_IDLE -> "stopped"
            else -> status.state
        }

    /** Two lines for the expanded notification. */
    fun detailedFor(status: StatusSnapshot): String {
        val parts = mutableListOf(textFor(status))
        if (status.state == StatusSnapshot.STATE_CONNECTED) {
            parts.add("socks 127.0.0.1:" + status.socksPort + " · " + modeLabel(status.mode))
            parts.add("↓ " + bytes(status.rxBytes) + "  ↑ " + bytes(status.txBytes) + " · " + uptime(status.uptimeSec))
        }
        if (status.lastError.isNotEmpty() && !status.isConnected) {
            parts.add(status.lastError)
        }
        return parts.joinToString("\n")
    }

    private fun modeLabel(mode: String): String = if (mode == "game") "game" else "normal"

    fun bytes(value: Long): String {
        if (value < 1024) return value.toString() + " B"
        val units = arrayOf("KB", "MB", "GB", "TB")
        var v = value.toDouble() / 1024.0
        var unit = 0
        while (v >= 1024 && unit < units.size - 1) {
            v /= 1024.0
            unit++
        }
        return String.format("%.1f %s", v, units[unit])
    }

    fun uptime(seconds: Long): String {
        if (seconds <= 0) return "just started"
        val h = seconds / 3600
        val m = (seconds % 3600) / 60
        val s = seconds % 60
        return when {
            h > 0 -> String.format("%dh %02dm", h, m)
            m > 0 -> String.format("%dm %02ds", m, s)
            else -> String.format("%ds", s)
        }
    }
}
