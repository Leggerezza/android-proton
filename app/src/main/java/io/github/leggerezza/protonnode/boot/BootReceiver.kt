// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.leggerezza.protonnode.boot

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.util.Log
import io.github.leggerezza.protonnode.data.Prefs
import io.github.leggerezza.protonnode.service.ProtonService

/**
 * Restarts the node after a reboot or an app update when the user opted in.
 *
 * A foreground-service start from BOOT_COMPLETED is not always allowed, so the
 * attempt is best effort and never crashes the boot.
 */
class BootReceiver : BroadcastReceiver() {
    override fun onReceive(
        context: Context,
        intent: Intent,
    ) {
        val action = intent.action ?: return
        if (action != Intent.ACTION_BOOT_COMPLETED && action != Intent.ACTION_MY_PACKAGE_REPLACED) return
        val prefs = Prefs(context)
        if (!prefs.autoStart || !prefs.wantRunning) return
        runCatching { ProtonService.start(context) }
            .onFailure { Log.w("BootReceiver", "cannot start the node after boot: " + it.message) }
    }
}
