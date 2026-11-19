package io.github.dsh.protonnode.core

import android.content.Context
import io.github.dsh.protonnode.data.Prefs
import io.github.dsh.protonnode.data.SessionStore
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import protonnode.Protonnode
import java.io.File

/**
 * The only place that talks to the gomobile bridge.
 *
 * Rules (PROMPT.md 5.2): every blocking call runs on a single IO thread so the
 * Go side sees a serialised command stream, `Status()` stays cheap, and nothing
 * here is ever called from the main thread.
 */
object CoreController {
    private val io = Dispatchers.IO.limitedParallelism(1)

    @Volatile
    private var initialised = false

    @Volatile
    var lastStatus: StatusSnapshot = StatusSnapshot.idle
        private set

    /** Initialises the Go core once and restores the encrypted session. */
    suspend fun ensureInit(context: Context) =
        withContext(io) {
            if (initialised) return@withContext
            val dir = File(context.filesDir, "core").apply { mkdirs() }
            Protonnode.init(dir.absolutePath)
            val prefs = Prefs(context)
            val dns = prefs.apiDns
            if (dns.isNotEmpty()) {
                Protonnode.setAPIDNS(dns)
            }
            SessionStore(context).load()?.let { session ->
                runCatching { Protonnode.setSession(session) }
            }
            Protonnode.setNetworkFingerprint(prefs.networkFingerprint)
            initialised = true
            lastStatus = StatusSnapshot.from(Protonnode.status())
        }

    suspend fun start(configJson: String): Result<Unit> =
        withContext(io) {
            runCatching { Protonnode.start(configJson) }.map { }
        }

    suspend fun stop(): Result<Unit> =
        withContext(io) {
            runCatching { Protonnode.stop() }.map { }
        }

    suspend fun status(): StatusSnapshot =
        withContext(io) {
            val snapshot = StatusSnapshot.from(Protonnode.status())
            lastStatus = snapshot
            snapshot
        }

    suspend fun recentLogs(lines: Int): String =
        withContext(io) {
            runCatching { Protonnode.recentLogs(lines.toLong()) }.getOrDefault("")
        }

    suspend fun sessionState(): String =
        withContext(io) {
            runCatching { Protonnode.sessionState() }.getOrDefault("{}")
        }

    suspend fun sessionJson(): String? =
        withContext(io) {
            runCatching { Protonnode.sessionJSON() }.getOrNull()
        }

    suspend fun setSession(json: String): Result<Unit> =
        withContext(io) {
            runCatching { Protonnode.setSession(json) }.map { }
        }

    suspend fun setNetworkFingerprint(fingerprint: String) =
        withContext(io) {
            runCatching { Protonnode.setNetworkFingerprint(fingerprint) }
        }

    suspend fun setApiDns(server: String) =
        withContext(io) {
            runCatching { Protonnode.setAPIDNS(server) }
        }

    /** Called when the process is about to go away (PROMPT.md 5.2). */
    suspend fun shutdown() =
        withContext(io) {
            runCatching { Protonnode.shutdown() }
            initialised = false
        }
}
