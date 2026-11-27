// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.leggerezza.protonnode.core

import android.content.ContentValues
import android.content.Context
import android.content.Intent
import android.net.ConnectivityManager
import android.net.NetworkCapabilities
import android.os.Build
import android.os.Environment
import android.provider.MediaStore
import androidx.core.content.FileProvider
import java.io.File
import java.text.SimpleDateFormat
import java.time.Instant
import java.util.Date
import java.util.Locale
import java.util.TimeZone

/**
 * Turns the ring log into a diagnostic report and gets it off the device.
 *
 * The header matters as much as the lines: when somebody sends a log later, the
 * first question is always "which build, which settings, what was the session
 * state" - so every export starts with that block, the way Proton's own client
 * does.
 */
object LogExport {
    private const val FILE_PREFIX = "proton-node-"
    private const val MIME = "text/plain"

    // MediaStore appends ".txt" to text/plain names it thinks lack an extension;
    // a log is opaque bytes, so it keeps the exact name we give it.
    private const val SAVE_MIME = "application/octet-stream"

    fun stamp(): String =
        SimpleDateFormat("yyyyMMdd-HHmmss", Locale.US)
            .apply {
                timeZone = TimeZone.getDefault()
            }.format(Date())

    /** File name with the wall clock time, e.g. proton-node-20261125-193000.log. */
    fun fileName(): String = FILE_PREFIX + stamp() + ".log"

    /** Builds the full report: context header, current status, then the ring log. */
    fun build(
        context: Context,
        status: StatusSnapshot,
        session: SessionInfo,
        settings: List<Pair<String, String>>,
        log: String,
    ): String =
        buildString {
            appendLine("# Proton Node diagnostic log")
            appendLine("# generated: " + Instant.now().toString())
            appendLine("# app: " + appVersion(context))
            appendLine("# device: " + Build.MANUFACTURER + " " + Build.MODEL + " (" + Build.DEVICE + ")")
            appendLine("# android: " + Build.VERSION.RELEASE + " (API " + Build.VERSION.SDK_INT + ")")
            appendLine("# timezone: " + TimeZone.getDefault().id)
            appendLine("# network: " + networkSummary(context))
            appendLine("# session: " + sessionSummary(session))
            appendLine("# state: " + status.state)
            appendLine("# server: " + status.server.ifEmpty { "-" } + " entry=" + status.entryIP.ifEmpty { "-" })
            appendLine(
                "# handshake: " + status.handshakeMs + " ms uptime=" + status.uptimeSec + " s" +
                    " rx=" + status.rxBytes + " tx=" + status.txBytes,
            )
            if (status.lastError.isNotEmpty()) appendLine("# lastError: " + status.lastError)
            settings.forEach { (key, value) -> appendLine("# " + key + ": " + value) }
            appendLine("# ---- node log (" + log.lines().count { it.isNotBlank() } + " lines) ----")
            log
        }

    /** Writes [content] into the cache, where the FileProvider can serve it. */
    fun writeCacheFile(
        context: Context,
        content: String,
        name: String = fileName(),
    ): File {
        val dir = File(context.cacheDir, "logs").apply { mkdirs() }
        return File(dir, name).apply { writeText(content) }
    }

    /** Shares the report as a real .log file (not a wall of text in the message body). */
    fun share(
        context: Context,
        file: File,
    ) {
        val uri = FileProvider.getUriForFile(context, context.packageName + ".fileprovider", file)
        val send =
            Intent(Intent.ACTION_SEND).apply {
                type = MIME
                putExtra(Intent.EXTRA_STREAM, uri)
                putExtra(Intent.EXTRA_SUBJECT, file.name)
                addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
            }
        context.startActivity(Intent.createChooser(send, null))
    }

    /**
     * Saves the report into the public Downloads folder via MediaStore, so no
     * storage permission is needed. Returns the display path on success.
     */
    fun saveToDownloads(
        context: Context,
        content: String,
        name: String = fileName(),
    ): String? {
        val resolver = context.contentResolver
        return runCatching {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                val values =
                    ContentValues().apply {
                        put(MediaStore.Downloads.DISPLAY_NAME, name)
                        put(MediaStore.Downloads.MIME_TYPE, SAVE_MIME)
                        put(MediaStore.Downloads.RELATIVE_PATH, Environment.DIRECTORY_DOWNLOADS)
                        put(MediaStore.Downloads.IS_PENDING, 1)
                    }
                val uri = resolver.insert(MediaStore.Downloads.EXTERNAL_CONTENT_URI, values) ?: return null
                val bytes = content.toByteArray()
                // "wt" truncates: "w" alone can leave a partially written file.
                resolver.openOutputStream(uri, "wt")?.use { out ->
                    out.write(bytes)
                    out.flush()
                }
                values.clear()
                values.put(MediaStore.Downloads.IS_PENDING, 0)
                resolver.update(uri, values, null, null)
                "Download/" + name
            } else {
                // API 26-28: app-specific external dir, no permission required.
                val dir = File(context.getExternalFilesDir(Environment.DIRECTORY_DOWNLOADS), "")
                dir.mkdirs()
                val file = File(dir, name).apply { writeText(content) }
                file.absolutePath
            }
        }.getOrNull()
    }

    @Suppress("DEPRECATION")
    private fun appVersion(context: Context): String =
        runCatching {
            val info = context.packageManager.getPackageInfo(context.packageName, 0)
            val code =
                if (Build.VERSION.SDK_INT >=
                    Build.VERSION_CODES.P
                ) {
                    info.longVersionCode
                } else {
                    info.versionCode.toLong()
                }
            info.versionName + " (" + code + ")"
        }.getOrDefault("unknown")

    private fun sessionSummary(session: SessionInfo): String =
        if (!session.loggedIn) {
            "signed out"
        } else {
            session.email + " plan=" + session.plan + " state=" + session.authState +
                " verified=" + session.lastVerifiedAt +
                (if (session.authError.isNotEmpty()) " authError=" + session.authError else "")
        }

    private fun networkSummary(context: Context): String {
        val cm = context.getSystemService(ConnectivityManager::class.java) ?: return "unknown"
        val caps = cm.getNetworkCapabilities(cm.activeNetwork) ?: return "offline"
        val transport =
            when {
                caps.hasTransport(NetworkCapabilities.TRANSPORT_WIFI) -> "wifi"
                caps.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR) -> "cellular"
                caps.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET) -> "ethernet"
                else -> "other"
            }
        return transport +
            " validated=" + caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_VALIDATED) +
            " captivePortal=" + caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_CAPTIVE_PORTAL) +
            " internet=" + caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
    }
}
