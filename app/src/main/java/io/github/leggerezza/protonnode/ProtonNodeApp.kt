// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.leggerezza.protonnode

import android.app.Application
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import io.github.leggerezza.protonnode.service.NotificationFactory
import io.github.leggerezza.protonnode.work.HealthWorker
import java.util.concurrent.TimeUnit

/**
 * Process entry point.
 *
 * P4: creates the notification channel up front and schedules the best-effort
 * health check; the node itself lives in [io.github.leggerezza.protonnode.service.ProtonService].
 */
class ProtonNodeApp : Application() {
    override fun onCreate() {
        super.onCreate()
        NotificationFactory.ensureChannel(this)
        WorkManager.getInstance(this).enqueueUniquePeriodicWork(
            HEALTH_WORK,
            ExistingPeriodicWorkPolicy.KEEP,
            PeriodicWorkRequestBuilder<HealthWorker>(15, TimeUnit.MINUTES).build(),
        )
    }

    private companion object {
        const val HEALTH_WORK = "node-health"
    }
}
