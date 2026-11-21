// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.leggerezza.protonnode.service

import android.Manifest
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.IBinder
import android.util.Log
import androidx.core.app.NotificationManagerCompat
import androidx.core.content.ContextCompat
import io.github.leggerezza.protonnode.core.CoreController
import io.github.leggerezza.protonnode.data.Prefs
import io.github.leggerezza.protonnode.data.SessionStore
import io.github.leggerezza.protonnode.net.NetworkMonitor
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeoutOrNull
import kotlin.concurrent.thread

/**
 * Foreground service that owns the node for as long as the user wants it up.
 *
 * Contract (PROMPT.md 5.2 / 6.2):
 *  - `specialUse` foreground service with an ongoing, silent notification;
 *  - every core call happens on the CoreController IO thread, never on the main thread;
 *  - `onDestroy` always calls `Stop()` (bounded to 5s);
 *  - a network change immediately restarts the node (new scan/fingerprint/SNI) instead of
 *    waiting for the 60s reconnect backoff;
 *  - the notification is refreshed every 5s while the service runs.
 */
class ProtonService : Service() {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    private var pollJob: Job? = null

    /** Last fingerprint we acted on; ConnectivityManager repeats callbacks, so
     *  identical values must not restart the tunnel. */
    @Volatile
    private var lastFingerprint: String? = null

    override fun onCreate() {
        super.onCreate()
        NotificationFactory.ensureChannel(this)
        NetworkMonitor.start(applicationContext) { fingerprint ->
            onNetworkChanged(fingerprint)
        }
        running = true
    }

    override fun onStartCommand(
        intent: Intent?,
        flags: Int,
        startId: Int,
    ): Int {
        if (intent?.action == ACTION_STOP) {
            stopNode()
            return START_NOT_STICKY
        }
        startForegroundCompat(CoreController.lastStatus)
        val config = intent?.getStringExtra(EXTRA_CONFIG) ?: Prefs(this).lastConfigJson
        Prefs(this).lastConfigJson = config
        Prefs(this).wantRunning = true
        scope.launch { launchNode(config) }
        return START_NOT_STICKY
    }

    private suspend fun launchNode(config: String) {
        val context = applicationContext
        CoreController.ensureInit(context)
        val result = CoreController.start(config)
        if (result.isFailure) {
            Log.w(TAG, "start rejected: " + result.exceptionOrNull()?.message)
        }
        startPolling()
    }

    /** Refreshes the notification every 5s; stops when the service stops. */
    private fun startPolling() {
        pollJob?.cancel()
        pollJob =
            scope.launch {
                while (isActive) {
                    val status = CoreController.status()
                    updateNotification(status)
                    // Proton refresh tokens are single-use: write back every rotation.
                    CoreController.persistSession(SessionStore(applicationContext))
                    delay(POLL_INTERVAL_MS)
                }
            }
    }

    private fun updateNotification(status: io.github.leggerezza.protonnode.core.StatusSnapshot) {
        // POST_NOTIFICATIONS is a runtime permission from API 33. The foreground
        // notification itself is posted by startForeground (exempt); this refresh
        // is skipped when the user has not granted it (P5 asks for it).
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
            ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) !=
            PackageManager.PERMISSION_GRANTED
        ) {
            return
        }
        runCatching {
            NotificationManagerCompat
                .from(this)
                .notify(NotificationFactory.NOTIFICATION_ID, NotificationFactory.build(this, status))
        }
    }

    /**
     * Immediate reconnect on a network switch (PROMPT.md 6.2): the tunnel is
     * bound to the old path, so it is stopped and started again with the new
     * network fingerprint and a fresh server scan.
     */
    private fun onNetworkChanged(fingerprint: String) {
        if (!running) return
        if (fingerprint == lastFingerprint) return
        val changed = lastFingerprint != null
        lastFingerprint = fingerprint
        val config = Prefs(this).lastConfigJson
        Prefs(this).networkFingerprint = fingerprint
        scope.launch {
            CoreController.setNetworkFingerprint(fingerprint)
            // The first observation is just the current network; only a real
            // change invalidates the tunnel (PROMPT.md 5.5 / 6.2).
            if (changed && CoreController.lastStatus.isRunning) {
                Log.i(TAG, "network changed to " + fingerprint + ", restarting the tunnel")
                withTimeoutOrNull(STOP_BUDGET_MS) { CoreController.stop() }
                CoreController.start(config)
            }
        }
    }

    private fun startForegroundCompat(status: io.github.leggerezza.protonnode.core.StatusSnapshot) {
        val notification = NotificationFactory.build(this, status)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
            startForeground(
                NotificationFactory.NOTIFICATION_ID,
                notification,
                ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE,
            )
        } else {
            startForeground(NotificationFactory.NOTIFICATION_ID, notification)
        }
    }

    private fun stopNode() {
        Prefs(this).wantRunning = false
        scope.launch {
            withTimeoutOrNull(STOP_BUDGET_MS) { CoreController.stop() }
            stopSelf()
        }
    }

    override fun onDestroy() {
        running = false
        pollJob?.cancel()
        NetworkMonitor.stop()
        // onDestroy must not return before the tunnel is gone, but it must not
        // block the main thread for the whole budget either: stop on a worker
        // and wait at most STOP_BUDGET_MS.
        val worker =
            thread(name = "node-stop") {
                runBlocking { withTimeoutOrNull(STOP_BUDGET_MS) { CoreController.stop() } }
            }
        worker.join(STOP_BUDGET_MS + 500)
        stopForeground(STOP_FOREGROUND_REMOVE)
        scope.cancel()
        super.onDestroy()
    }

    override fun onBind(intent: Intent?): IBinder? = null

    companion object {
        private const val TAG = "ProtonService"
        private const val POLL_INTERVAL_MS = 5_000L
        private const val STOP_BUDGET_MS = 5_000L

        const val ACTION_START = "io.github.leggerezza.protonnode.START"
        const val ACTION_STOP = "io.github.leggerezza.protonnode.STOP"
        const val EXTRA_CONFIG = "config"

        @Volatile
        var running: Boolean = false
            private set

        fun start(
            context: Context,
            configJson: String? = null,
        ) {
            val intent =
                Intent(context, ProtonService::class.java)
                    .setAction(ACTION_START)
                    .putExtra(EXTRA_CONFIG, configJson)
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                context.startForegroundService(intent)
            } else {
                context.startService(intent)
            }
        }

        fun stop(context: Context) {
            context.startService(Intent(context, ProtonService::class.java).setAction(ACTION_STOP))
        }
    }
}
