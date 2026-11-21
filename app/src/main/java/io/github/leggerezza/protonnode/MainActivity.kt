// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.leggerezza.protonnode

import android.os.Bundle
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.viewModels
import androidx.appcompat.app.AppCompatActivity
import androidx.compose.runtime.getValue
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.leggerezza.protonnode.ui.NodeViewModel
import io.github.leggerezza.protonnode.ui.ProtonApp
import io.github.leggerezza.protonnode.ui.theme.ProtonTheme
import io.github.leggerezza.protonnode.ui.theme.ThemeMode

class MainActivity : AppCompatActivity() {
    private val vm: NodeViewModel by viewModels()

    override fun onNewIntent(intent: android.content.Intent) {
        super.onNewIntent(intent)
        handleDebugIntent(intent)
    }

    /**
     * Debug builds only: lets the developer inject a session from adb so the
     * real UI can be exercised without typing a password on the device:
     *   adb shell am start -n <pkg>/.MainActivity --es session_json '<json>'
     */
    private fun handleDebugIntent(intent: android.content.Intent?) {
        if (!BuildConfig.DEBUG) return
        val encoded = intent?.getStringExtra("session_b64") ?: return
        val json = String(android.util.Base64.decode(encoded, android.util.Base64.NO_WRAP), Charsets.UTF_8)
        val result = vm.setSession(json)
        android.util.Log.i("MainActivity", "debug session injected: " + result.isSuccess)
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        handleDebugIntent(intent)
        enableEdgeToEdge()
        setContent {
            val themeMode by vm.themeMode.collectAsStateWithLifecycle()
            val dynamic by vm.dynamicColor.collectAsStateWithLifecycle()
            ProtonTheme(
                mode = runCatching { ThemeMode.valueOf(themeMode.uppercase()) }.getOrDefault(ThemeMode.SYSTEM),
                dynamicColor = dynamic,
            ) {
                ProtonApp(vm)
            }
        }
    }
}
