// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.leggerezza.protonnode.core

import android.content.Context
import io.github.leggerezza.protonnode.data.Prefs
import io.github.leggerezza.protonnode.data.SessionStore
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

    suspend fun login(
        user: String,
        pass: String,
        totp: String,
    ): Result<String> = withContext(io) { runCatching { Protonnode.login(user, pass, totp) } }

    suspend fun logout(): Result<Unit> = withContext(io) { runCatching { Protonnode.logout() }.map { } }

    /**
     * Persists a rotated session. Proton refresh tokens are single-use, so the
     * app must write back every rotation or the next start fails with 2001
     * (the same bug the CLI had in P2).
     */
    suspend fun persistSession(store: io.github.leggerezza.protonnode.data.SessionStore): Boolean =
        withContext(io) {
            val json = runCatching { Protonnode.sessionJSON() }.getOrNull() ?: return@withContext false
            if (json.isEmpty() || json == lastSavedSession) return@withContext false
            store.save(json)
            lastSavedSession = json
            true
        }

    @Volatile
    private var lastSavedSession: String? = null

    suspend fun verifySession(): Result<String> = withContext(io) { runCatching { Protonnode.verifySession() } }

    suspend fun listServers(
        country: String,
        secureCore: Boolean,
    ): String =
        withContext(io) {
            runCatching { Protonnode.listServers(country, secureCore) }.getOrDefault("[]")
        }

    suspend fun startScan(
        scope: String,
        concurrency: Int,
    ): String = withContext(io) { Protonnode.startScan(scope, concurrency.toLong()) }

    suspend fun scanProgress(scanId: String): String =
        withContext(io) {
            runCatching { Protonnode.scanProgress(scanId) }.getOrDefault("{}")
        }

    suspend fun scanResults(scanId: String): String =
        withContext(io) {
            runCatching { Protonnode.scanResults(scanId) }.getOrDefault("{}")
        }

    suspend fun measureLatencies(scope: String): Int =
        withContext(io) { runCatching { Protonnode.measureLatencies(scope).toInt() }.getOrDefault(0) }

    suspend fun cancelScan(scanId: String): Boolean =
        withContext(io) {
            runCatching { Protonnode.cancelScan(scanId) }.getOrDefault(false)
        }

    suspend fun lastScanSummary(): String =
        withContext(io) {
            runCatching { Protonnode.lastScanSummary() }.getOrDefault("{}")
        }

    /** Called when the process is about to go away (PROMPT.md 5.2). */
    suspend fun shutdown() =
        withContext(io) {
            runCatching { Protonnode.shutdown() }
            initialised = false
        }
}
