// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.leggerezza.protonnode.ui

import android.Manifest
import android.content.pm.PackageManager
import android.os.Build
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Article
import androidx.compose.material.icons.outlined.Dns
import androidx.compose.material.icons.outlined.Home
import androidx.compose.material.icons.outlined.Radar
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.material3.Icon
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.NavigationRail
import androidx.compose.material3.NavigationRailItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.core.content.ContextCompat
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.leggerezza.protonnode.R
import io.github.leggerezza.protonnode.ui.screens.DisclaimerDialog
import io.github.leggerezza.protonnode.ui.screens.HomeScreen
import io.github.leggerezza.protonnode.ui.screens.LogsScreen
import io.github.leggerezza.protonnode.ui.screens.ScanScreen
import io.github.leggerezza.protonnode.ui.screens.ServersScreen
import io.github.leggerezza.protonnode.ui.screens.SettingsScreen

/** Destinations of the left navigation (PROMPT.md 6.3). */
enum class Dest(
    val icon: ImageVector,
    val labelRes: Int,
) {
    HOME(Icons.Outlined.Home, R.string.nav_home),
    SERVERS(Icons.Outlined.Dns, R.string.nav_servers),
    SCAN(Icons.Outlined.Radar, R.string.nav_scan),
    SETTINGS(Icons.Outlined.Settings, R.string.nav_settings),
    LOGS(Icons.Outlined.Article, R.string.nav_logs),
}

/**
 * Window-size classes from PROMPT.md 6.3:
 *  - compact  (< 600dp): single column + bottom navigation;
 *  - medium   (600..839dp): navigation rail + one pane;
 *  - expanded (>= 840dp): navigation rail + wide content (two columns where it helps).
 */
object Layout {
    const val MEDIUM = 600
    const val EXPANDED = 840

    fun widthClass(widthDp: Int): Int =
        when {
            widthDp < MEDIUM -> 0
            widthDp < EXPANDED -> 1
            else -> 2
        }
}

@Composable
fun ProtonApp(vm: NodeViewModel) {
    var dest by remember { mutableStateOf(Dest.HOME) }
    val wide = Layout.widthClass(LocalConfiguration.current.screenWidthDp) > 0

    val disclaimerAccepted by vm.disclaimerAccepted.collectAsStateWithLifecycle()

    // Android 13+ hides notifications (including this app's foreground service
    // notification) until the user grants POST_NOTIFICATIONS. Ask once the notice
    // is out of the way, so a dropped tunnel is actually visible.
    val appContext = LocalContext.current
    val notificationPermission =
        rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { }
    LaunchedEffect(disclaimerAccepted) {
        if (!disclaimerAccepted) return@LaunchedEffect
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) return@LaunchedEffect
        val granted =
            ContextCompat.checkSelfPermission(appContext, Manifest.permission.POST_NOTIFICATIONS) ==
                PackageManager.PERMISSION_GRANTED
        if (!granted) notificationPermission.launch(Manifest.permission.POST_NOTIFICATIONS)
    }
    if (!disclaimerAccepted) {
        DisclaimerDialog(onAccept = { vm.acceptDisclaimer() })
    }
    Scaffold(
        bottomBar = {
            if (!wide) {
                NavigationBar {
                    Dest.entries.forEach { item ->
                        NavigationBarItem(
                            selected = dest == item,
                            onClick = { dest = item },
                            icon = { Icon(item.icon, contentDescription = null) },
                            label = { Text(stringResource(item.labelRes)) },
                        )
                    }
                }
            }
        },
    ) { padding ->
        Row(Modifier.fillMaxSize().padding(padding)) {
            if (wide) {
                NavigationRail {
                    Dest.entries.forEach { item ->
                        NavigationRailItem(
                            selected = dest == item,
                            onClick = { dest = item },
                            icon = { Icon(item.icon, contentDescription = null) },
                            label = { Text(stringResource(item.labelRes)) },
                        )
                    }
                }
            }
            Box(Modifier.fillMaxSize()) {
                when (dest) {
                    Dest.HOME ->
                        HomeScreen(
                            vm,
                            expanded =
                                Layout.widthClass(LocalConfiguration.current.screenWidthDp) == 2,
                        )
                    Dest.SERVERS -> ServersScreen(vm)
                    Dest.SCAN ->
                        ScanScreen(
                            vm = vm,
                            onUseServer = { name, country ->
                                vm.selectServer(name, country)
                                vm.connect()
                                dest = Dest.HOME
                            },
                        )
                    Dest.SETTINGS -> SettingsScreen(vm)
                    Dest.LOGS -> LogsScreen(vm)
                }
            }
        }
    }
}
