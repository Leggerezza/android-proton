package io.github.dsh.protonnode.work

import android.content.Context
import androidx.work.Worker
import androidx.work.WorkerParameters
import io.github.dsh.protonnode.core.CoreController
import io.github.dsh.protonnode.data.Prefs
import io.github.dsh.protonnode.service.ProtonService
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeoutOrNull

/**
 * Periodic best-effort health check (PROMPT.md 6.2: WorkManager is best effort,
 * never a precision timer). If the user wants the node running but the last
 * status is not connected, the service is asked to restart it.
 */
class HealthWorker(
    context: Context,
    params: WorkerParameters,
) : Worker(context, params) {
    override fun doWork(): Result {
        val prefs = Prefs(applicationContext)
        if (!prefs.wantRunning && !prefs.autoStart) return Result.success()
        val status =
            runBlocking { withTimeoutOrNull(5_000) { CoreController.status() } }
                ?: return Result.retry()
        if (status.isConnected) return Result.success()
        if (ProtonService.running) {
            ProtonService.start(applicationContext, prefs.lastConfigJson)
        }
        return Result.success()
    }
}
