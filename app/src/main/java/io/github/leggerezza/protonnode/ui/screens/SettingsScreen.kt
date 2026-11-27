// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.leggerezza.protonnode.ui.screens

import android.Manifest
import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.provider.Settings
import androidx.appcompat.app.AppCompatDelegate
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.core.content.ContextCompat
import androidx.core.os.LocaleListCompat
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.leggerezza.protonnode.BuildConfig
import io.github.leggerezza.protonnode.R
import io.github.leggerezza.protonnode.core.SessionInfo
import io.github.leggerezza.protonnode.data.ConfigCodec
import io.github.leggerezza.protonnode.data.NodeProfile
import io.github.leggerezza.protonnode.ui.LoginMessageKind
import io.github.leggerezza.protonnode.ui.LoginUi
import io.github.leggerezza.protonnode.ui.NodeViewModel
import io.github.leggerezza.protonnode.ui.theme.formatTimestamp

/** One entry of the settings root list. */
private enum class SettingsPage {
    ACCOUNT,
    THEME,
    LANGUAGE,
    NOTIFICATIONS,
    BEHAVIOR,
    FLCLASH,
    ABOUT,
}

@Composable
fun SettingsScreen(vm: NodeViewModel) {
    var open by remember { mutableStateOf<SettingsPage?>(null) }
    val current = open
    if (current == null) {
        SettingsRoot(vm, onOpen = { open = it })
    } else {
        SettingsDetail(vm, current, onBack = { open = null })
    }
}

@Composable
private fun SettingsRoot(
    vm: NodeViewModel,
    onOpen: (SettingsPage) -> Unit,
) {
    val sessionInfo by vm.session.collectAsStateWithLifecycle()
    val themeMode by vm.themeMode.collectAsStateWithLifecycle()
    val mode by vm.mode.collectAsStateWithLifecycle()
    val transport by vm.transport.collectAsStateWithLifecycle()
    val activeLanguage = currentLanguage()

    Column(
        modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(24.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        Column(Modifier.widthIn(max = 760.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text(stringResource(R.string.settings_title), style = MaterialTheme.typography.headlineSmall)
            SessionBanner(sessionInfo)
            CategoryRow(
                title = stringResource(R.string.settings_account),
                summary = accountSummary(sessionInfo),
                onClick = { onOpen(SettingsPage.ACCOUNT) },
            )
            CategoryRow(
                title = stringResource(R.string.settings_section_theme),
                summary = themeMode,
                onClick = { onOpen(SettingsPage.THEME) },
            )
            CategoryRow(
                title = stringResource(R.string.settings_language),
                summary = activeLanguage.ifEmpty { stringResource(R.string.settings_language_system) },
                onClick = { onOpen(SettingsPage.LANGUAGE) },
            )
            CategoryRow(
                title = stringResource(R.string.settings_section_notifications),
                summary = notificationSummary(),
                onClick = { onOpen(SettingsPage.NOTIFICATIONS) },
            )
            CategoryRow(
                title = stringResource(R.string.settings_behavior),
                summary = mode + " · " + transport,
                onClick = { onOpen(SettingsPage.BEHAVIOR) },
            )
            CategoryRow(
                title = stringResource(R.string.settings_flclash),
                summary = stringResource(R.string.settings_flclash_hint_short),
                onClick = { onOpen(SettingsPage.FLCLASH) },
            )
            CategoryRow(
                title = stringResource(R.string.settings_section_about),
                summary = stringResource(R.string.about_version_short, BuildConfig.VERSION_NAME),
                onClick = { onOpen(SettingsPage.ABOUT) },
            )
        }
    }
}

@Composable
private fun CategoryRow(
    title: String,
    summary: String,
    onClick: () -> Unit,
) {
    Card(
        modifier = Modifier.fillMaxWidth(),
        onClick = onClick,
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceVariant),
    ) {
        Row(
            Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 14.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Column(Modifier.weight(1f)) {
                Text(title, style = MaterialTheme.typography.titleMedium)
                if (summary.isNotEmpty()) {
                    Text(
                        summary,
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
            Icon(Icons.AutoMirrored.Filled.KeyboardArrowRight, contentDescription = null)
        }
    }
}

@Composable
private fun SettingsDetail(
    vm: NodeViewModel,
    page: SettingsPage,
    onBack: () -> Unit,
) {
    val title =
        when (page) {
            SettingsPage.ACCOUNT -> stringResource(R.string.settings_account)
            SettingsPage.THEME -> stringResource(R.string.settings_section_theme)
            SettingsPage.LANGUAGE -> stringResource(R.string.settings_language)
            SettingsPage.NOTIFICATIONS -> stringResource(R.string.settings_section_notifications)
            SettingsPage.BEHAVIOR -> stringResource(R.string.settings_behavior)
            SettingsPage.FLCLASH -> stringResource(R.string.settings_flclash)
            SettingsPage.ABOUT -> stringResource(R.string.settings_section_about)
        }
    Column(
        modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(24.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        Column(Modifier.widthIn(max = 760.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                IconButton(onClick = onBack) {
                    Icon(
                        Icons.AutoMirrored.Filled.ArrowBack,
                        contentDescription = stringResource(R.string.settings_back),
                    )
                }
                Text(title, style = MaterialTheme.typography.headlineSmall)
            }
            when (page) {
                SettingsPage.ACCOUNT -> AccountPage(vm)
                SettingsPage.THEME -> ThemePage(vm)
                SettingsPage.LANGUAGE -> LanguagePage()
                SettingsPage.NOTIFICATIONS -> NotificationsPage()
                SettingsPage.BEHAVIOR -> BehaviorPage(vm)
                SettingsPage.FLCLASH -> FlClashPage(vm)
                SettingsPage.ABOUT -> AboutPage()
            }
        }
    }
}

@Composable
private fun AccountPage(vm: NodeViewModel) {
    val sessionInfo by vm.session.collectAsStateWithLifecycle()
    val loginUi by vm.loginUi.collectAsStateWithLifecycle()
    var email by remember { mutableStateOf("") }
    var password by remember { mutableStateOf("") }
    var totp by remember { mutableStateOf("") }
    var cooldown by remember { mutableStateOf(0L) }
    var showSession by remember { mutableStateOf(false) }
    var sessionText by remember { mutableStateOf("") }
    var message by remember { mutableStateOf("") }
    val savedMessage = stringResource(R.string.session_saved)
    val invalidMessage = stringResource(R.string.import_invalid)

    LaunchedEffect(loginUi.kind, loginUi.seconds) {
        var left = loginUi.seconds
        while (left > 0) {
            cooldown = left
            kotlinx.coroutines.delay(1000)
            left -= 1
        }
        cooldown = 0
    }

    SessionStatusLine(sessionInfo, loginUi)
    if (sessionInfo.loggedIn) {
        OutlinedButton(onClick = { vm.logout() }) { Text(stringResource(R.string.settings_logout)) }
    }
    OutlinedTextField(
        value = email,
        onValueChange = { email = it },
        label = { Text(stringResource(R.string.settings_email)) },
        singleLine = true,
        enabled = !loginUi.busy && !sessionInfo.isUsable,
        modifier = Modifier.fillMaxWidth(),
    )
    OutlinedTextField(
        value = password,
        onValueChange = { password = it },
        label = { Text(stringResource(R.string.settings_password)) },
        singleLine = true,
        enabled = !loginUi.busy && !sessionInfo.isUsable,
        visualTransformation =
            androidx.compose.ui.text.input
                .PasswordVisualTransformation(),
        modifier = Modifier.fillMaxWidth(),
    )
    OutlinedTextField(
        value = totp,
        onValueChange = { totp = it },
        label = { Text(stringResource(R.string.settings_totp)) },
        singleLine = true,
        enabled = !loginUi.busy && !sessionInfo.isUsable,
        modifier = Modifier.fillMaxWidth(),
    )
    Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
        Button(
            onClick = { vm.signIn(email, password, totp) },
            enabled = !loginUi.busy && !sessionInfo.isUsable && cooldown == 0L,
        ) {
            Text(
                when {
                    sessionInfo.isUsable -> stringResource(R.string.settings_already_signed_in)
                    loginUi.busy -> stringResource(R.string.settings_signing_in)
                    cooldown > 0 -> stringResource(R.string.settings_wait, cooldown)
                    else -> stringResource(R.string.settings_sign_in)
                },
            )
        }
        if (sessionInfo.loggedIn && sessionInfo.authState == "unknown") {
            OutlinedButton(onClick = { vm.verifySession() }, enabled = !loginUi.busy) {
                Text(stringResource(R.string.settings_verify_session))
            }
        }
    }
    LoginFeedback(loginUi, cooldown)
    if (BuildConfig.DEBUG) {
        OutlinedButton(onClick = { showSession = true }) {
            Text(stringResource(R.string.settings_debug_session))
        }
    }
    if (message.isNotEmpty()) {
        Text(message, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.primary)
    }
    if (showSession) {
        AlertDialog(
            onDismissRequest = { showSession = false },
            title = { Text(stringResource(R.string.settings_debug_session)) },
            text = {
                OutlinedTextField(
                    value = sessionText,
                    onValueChange = { sessionText = it },
                    label = { Text("SessionJSON") },
                    minLines = 4,
                )
            },
            confirmButton = {
                TextButton(onClick = {
                    val result = vm.setSession(sessionText.trim())
                    message =
                        if (result.isSuccess) {
                            savedMessage
                        } else {
                            result.exceptionOrNull()?.message ?: invalidMessage
                        }
                    showSession = false
                }) { Text(stringResource(R.string.dialog_ok)) }
            },
            dismissButton = {
                TextButton(onClick = { showSession = false }) { Text(stringResource(R.string.dialog_cancel)) }
            },
        )
    }
}

@Composable
private fun ThemePage(vm: NodeViewModel) {
    val themeMode by vm.themeMode.collectAsStateWithLifecycle()
    val dynamic by vm.dynamicColor.collectAsStateWithLifecycle()
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        listOf(
            "system" to R.string.settings_theme_system,
            "light" to R.string.settings_theme_light,
            "dark" to R.string.settings_theme_dark,
        ).forEach { (value, label) ->
            FilterChip(
                selected = themeMode == value,
                onClick = { vm.setThemeMode(value) },
                label = { Text(stringResource(label)) },
            )
        }
    }
    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        Text(stringResource(R.string.settings_dynamic), Modifier.weight(1f))
        Switch(checked = dynamic, onCheckedChange = { vm.setDynamicColor(it) })
    }
}

@Composable
private fun LanguagePage() {
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        FilterChip(
            selected = currentLanguage().isEmpty(),
            onClick = { setLanguage("") },
            label = { Text(stringResource(R.string.settings_language_system)) },
        )
        FilterChip(
            selected = currentLanguage() == "en",
            onClick = { setLanguage("en") },
            label = { Text("English") },
        )
        FilterChip(
            selected = currentLanguage() == "zh",
            onClick = { setLanguage("zh") },
            label = { Text("简体中文") },
        )
    }
}

@Composable
private fun NotificationsPage() {
    val context = LocalContext.current
    if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) {
        Text(stringResource(R.string.settings_notifications_on), style = MaterialTheme.typography.bodyMedium)
        return
    }
    val granted =
        ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) ==
            PackageManager.PERMISSION_GRANTED
    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        Text(
            stringResource(
                if (granted) R.string.settings_notifications_on else R.string.settings_notifications_off,
            ),
            Modifier.weight(1f),
        )
        if (!granted) {
            OutlinedButton(onClick = {
                context.startActivity(
                    Intent(Settings.ACTION_APP_NOTIFICATION_SETTINGS)
                        .putExtra(Settings.EXTRA_APP_PACKAGE, context.packageName),
                )
            }) { Text(stringResource(R.string.settings_notifications_enable)) }
        }
    }
}

@Composable
private fun BehaviorPage(vm: NodeViewModel) {
    val mode by vm.mode.collectAsStateWithLifecycle()
    val autostart by vm.autostart.collectAsStateWithLifecycle()
    val gameLock by vm.gameLock.collectAsStateWithLifecycle()
    val apiDns by vm.apiDns.collectAsStateWithLifecycle()
    val preferredCountry by vm.preferredCountry.collectAsStateWithLifecycle()
    val mtu by vm.mtu.collectAsStateWithLifecycle()
    val transport by vm.transport.collectAsStateWithLifecycle()
    val ipv6 by vm.ipv6.collectAsStateWithLifecycle()
    val excluded by vm.excludedCountries.collectAsStateWithLifecycle()
    val context = LocalContext.current

    var countryText by remember(preferredCountry) { mutableStateOf(preferredCountry) }
    var countryError by remember { mutableStateOf(false) }
    var savedCountry by remember { mutableStateOf("") }
    var dnsText by remember(apiDns) { mutableStateOf(apiDns) }
    var showExcluded by remember { mutableStateOf(false) }
    var message by remember { mutableStateOf("") }

    Text(stringResource(R.string.settings_mode), style = MaterialTheme.typography.labelLarge)
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        FilterChip(
            selected = mode == "normal",
            onClick = { vm.setMode("normal") },
            label = { Text(stringResource(R.string.servers_mode_normal)) },
        )
        FilterChip(
            selected = mode == "game",
            onClick = { vm.setMode("game") },
            label = { Text(stringResource(R.string.servers_mode_game)) },
        )
    }
    OutlinedTextField(
        value = countryText,
        onValueChange = { countryText = it },
        label = { Text(stringResource(R.string.settings_default_country)) },
        supportingText = {
            Text(
                if (countryError) {
                    stringResource(R.string.settings_default_country_invalid)
                } else {
                    stringResource(R.string.settings_default_country_hint)
                },
            )
        },
        isError = countryError,
        singleLine = true,
        modifier = Modifier.fillMaxWidth(),
    )
    Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
        Button(onClick = {
            val ok = vm.setCountryCode(countryText)
            countryError = !ok
            savedCountry = if (ok) countryText.trim().uppercase() else ""
        }) { Text(stringResource(R.string.dialog_ok)) }
        OutlinedButton(onClick = {
            countryText = ""
            countryError = false
            vm.setCountryCode("")
        }) { Text(stringResource(R.string.servers_auto)) }
    }
    if (savedCountry.isNotEmpty()) {
        Text(
            stringResource(R.string.settings_default_country_saved, savedCountry),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.primary,
        )
    }
    Text(stringResource(R.string.settings_mtu), style = MaterialTheme.typography.labelLarge)
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        listOf(1200, 1320, 1420).forEach { value ->
            FilterChip(
                selected = mtu == value,
                onClick = { vm.setMtu(value) },
                label = { Text(value.toString()) },
            )
        }
    }
    Text(
        stringResource(R.string.settings_mtu_hint),
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
    Text(stringResource(R.string.settings_transport), style = MaterialTheme.typography.labelLarge)
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        FilterChip(
            selected = transport == "stealth",
            onClick = { vm.setTransport("stealth") },
            label = { Text(stringResource(R.string.settings_transport_stealth)) },
        )
        FilterChip(
            selected = transport == "udp",
            onClick = { vm.setTransport("udp") },
            label = { Text(stringResource(R.string.settings_transport_udp)) },
        )
    }
    Text(
        stringResource(R.string.settings_transport_hint),
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        Text(stringResource(R.string.settings_ipv6), Modifier.weight(1f))
        Switch(checked = ipv6, onCheckedChange = { vm.setIpv6(it) })
    }
    Text(
        stringResource(R.string.settings_ipv6_hint),
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
    OutlinedButton(onClick = { showExcluded = true }, modifier = Modifier.fillMaxWidth()) {
        Text(
            if (excluded.isEmpty()) {
                stringResource(R.string.settings_excluded_none)
            } else {
                stringResource(R.string.settings_excluded_selected, excluded.size)
            },
        )
    }
    if (mode == "game") {
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Text(stringResource(R.string.settings_game_lock), Modifier.weight(1f))
            Switch(checked = gameLock, onCheckedChange = { vm.setGameLock(it) })
        }
        Text(
            stringResource(R.string.settings_game_lock_hint),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        Text(stringResource(R.string.settings_autostart), Modifier.weight(1f))
        Switch(checked = autostart, onCheckedChange = { vm.setAutostart(it) })
    }
    OutlinedTextField(
        value = dnsText,
        onValueChange = { dnsText = it },
        label = { Text(stringResource(R.string.settings_api_dns)) },
        supportingText = { Text(stringResource(R.string.settings_api_dns_hint)) },
        singleLine = true,
        modifier = Modifier.fillMaxWidth(),
    )
    val savedLabel = stringResource(R.string.session_saved)
    Button(onClick = {
        vm.setApiDns(dnsText.trim())
        message = savedLabel
    }) { Text(stringResource(R.string.dialog_ok)) }
    if (message.isNotEmpty()) {
        Text(message, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.primary)
    }
    OutlinedButton(onClick = {
        val intent =
            Intent(
                Settings.ACTION_APPLICATION_DETAILS_SETTINGS,
                Uri.fromParts("package", context.packageName, null),
            )
        context.startActivity(intent)
    }) {
        Text(
            stringResource(R.string.settings_battery) + " · " + stringResource(R.string.settings_battery_hint),
        )
    }
    if (showExcluded) {
        ExcludedCountriesDialog(
            serversJson = vm.servers.collectAsStateWithLifecycle().value,
            selected = excluded,
            onToggle = { vm.toggleExcludedCountry(it) },
            onDismiss = { showExcluded = false },
        )
    }
}

@Composable
private fun FlClashPage(vm: NodeViewModel) {
    val context = LocalContext.current
    val ipv6 by vm.ipv6.collectAsStateWithLifecycle()
    var showImport by remember { mutableStateOf(false) }
    var message by remember { mutableStateOf("") }
    val copiedMessage = stringResource(R.string.export_copied)
    val okMessage = stringResource(R.string.dialog_ok)
    val invalidMessage = stringResource(R.string.import_invalid)
    val flclashCopiedMessage = stringResource(R.string.settings_flclash_copied)
    val flclashOkMessage = stringResource(R.string.settings_flclash_ok)
    val flclashBadMessage = stringResource(R.string.settings_flclash_bad)

    Text(
        stringResource(R.string.settings_flclash_hint),
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
    Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
        OutlinedButton(onClick = {
            val clipboard = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
            clipboard.setPrimaryClip(
                ClipData.newPlainText(
                    "mihomo profile",
                    io.github.leggerezza.protonnode.data.FlClashConfig
                        .generate(context.packageName, ipv6 = ipv6),
                ),
            )
            message = flclashCopiedMessage
        }) { Text(stringResource(R.string.settings_flclash_copy)) }
        OutlinedButton(onClick = {
            val clipboard = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
            val text =
                clipboard.primaryClip
                    ?.getItemAt(0)
                    ?.text
                    ?.toString()
                    .orEmpty()
            val problems =
                io.github.leggerezza.protonnode.data.FlClashConfig
                    .validate(text, context.packageName)
            message =
                if (problems.isEmpty()) {
                    flclashOkMessage
                } else {
                    flclashBadMessage + " " + problems.joinToString("; ")
                }
        }) { Text(stringResource(R.string.settings_flclash_check)) }
    }
    HorizontalDivider()
    Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
        OutlinedButton(onClick = {
            val clipboard = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
            clipboard.setPrimaryClip(ClipData.newPlainText("proton node config", exportConfig(vm)))
            message = copiedMessage
        }) { Text(stringResource(R.string.settings_export)) }
        OutlinedButton(onClick = { showImport = true }) { Text(stringResource(R.string.settings_import)) }
    }
    if (message.isNotEmpty()) {
        Text(message, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.primary)
    }
    if (showImport) {
        AlertDialog(
            onDismissRequest = { showImport = false },
            title = { Text(stringResource(R.string.settings_import)) },
            text = { Text(stringResource(R.string.import_confirm)) },
            confirmButton = {
                TextButton(onClick = {
                    val clipboard = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
                    val text =
                        clipboard.primaryClip
                            ?.getItemAt(0)
                            ?.text
                            ?.toString()
                            .orEmpty()
                    val ok = importConfig(vm, text)
                    message = if (ok) okMessage else invalidMessage
                    showImport = false
                }) { Text(stringResource(R.string.dialog_import)) }
            },
            dismissButton = {
                TextButton(onClick = { showImport = false }) { Text(stringResource(R.string.dialog_cancel)) }
            },
        )
    }
}

@Composable
private fun AboutPage() {
    Text(
        stringResource(R.string.about_version, BuildConfig.VERSION_NAME, BuildConfig.VERSION_CODE),
        style = MaterialTheme.typography.bodyMedium,
    )
    Text(
        stringResource(R.string.about_license),
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
    Text(
        stringResource(R.string.about_repo),
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.primary,
    )
    Text(
        stringResource(R.string.about_note),
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
}

/** Subtitle of the Account row: who is signed in, or why nothing works. */
@Composable
private fun accountSummary(info: SessionInfo): String =
    when {
        info.loggedIn && info.isExpired -> stringResource(R.string.settings_banner_expired)
        info.loggedIn -> info.email
        else -> stringResource(R.string.settings_banner_signed_out)
    }

@Composable
private fun notificationSummary(): String {
    if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) return stringResource(R.string.settings_notifications_on)
    val context = LocalContext.current
    val granted =
        ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) ==
            PackageManager.PERMISSION_GRANTED
    return stringResource(if (granted) R.string.settings_notifications_on else R.string.settings_notifications_off)
}

@Composable
private fun SessionBanner(info: SessionInfo) {
    val expired = info.loggedIn && info.isExpired
    val title =
        when {
            expired -> stringResource(R.string.settings_banner_expired)
            info.loggedIn -> stringResource(R.string.settings_banner_signed_in, info.email)
            else -> stringResource(R.string.settings_banner_signed_out)
        }
    val detail =
        when {
            expired -> stringResource(R.string.settings_banner_expired_hint)
            info.loggedIn -> stringResource(R.string.settings_banner_plan, info.plan, info.lastVerifiedAt)
            else -> stringResource(R.string.settings_banner_signed_out_hint)
        }
    val container =
        if (expired) MaterialTheme.colorScheme.errorContainer else MaterialTheme.colorScheme.surfaceVariant
    val content =
        if (expired) {
            MaterialTheme.colorScheme.onErrorContainer
        } else {
            MaterialTheme.colorScheme.onSurfaceVariant
        }
    Card(
        colors = CardDefaults.cardColors(containerColor = container),
        modifier = Modifier.fillMaxWidth(),
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(title, style = MaterialTheme.typography.titleSmall, color = content)
            Text(detail, style = MaterialTheme.typography.bodySmall, color = content)
        }
    }
}

/** One line that tells the user whether the stored session is actually usable. */
@Composable
private fun SessionStatusLine(
    info: SessionInfo,
    ui: LoginUi,
) {
    val (text, color) =
        when {
            !info.loggedIn -> stringResource(R.string.settings_signed_out) to MaterialTheme.colorScheme.onSurfaceVariant
            info.isExpired -> stringResource(R.string.settings_session_expired) to MaterialTheme.colorScheme.error
            info.isVerified && info.lastVerifiedAt.isNotEmpty() ->
                stringResource(R.string.settings_session_verified, info.email, formatTimestamp(info.lastVerifiedAt)) to
                    MaterialTheme.colorScheme.primary
            else ->
                stringResource(R.string.settings_session_unverified, info.email) to
                    MaterialTheme.colorScheme.onSurfaceVariant
        }
    Text(text, style = MaterialTheme.typography.bodyMedium, color = color)
    if (info.loggedIn && info.isExpired && info.authError.isNotEmpty()) {
        Text(info.authError, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error)
    }
    if (ui.busy) {
        LinearProgressIndicator(Modifier.fillMaxWidth())
    }
}

/** Explicit feedback for every login outcome. */
@Composable
private fun LoginFeedback(
    ui: LoginUi,
    cooldownSeconds: Long,
) {
    when (ui.kind) {
        LoginMessageKind.ALREADY_SIGNED_IN ->
            Text(
                stringResource(R.string.settings_already_signed_in_as, ui.detail),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.primary,
            )
        LoginMessageKind.MISSING_FIELDS ->
            Text(
                stringResource(R.string.settings_login_missing),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.error,
            )
        LoginMessageKind.SUCCESS ->
            Text(
                stringResource(R.string.settings_login_success, ui.detail),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.primary,
            )
        LoginMessageKind.VERIFIED ->
            Text(
                stringResource(R.string.settings_verify_ok, ui.detail),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.primary,
            )
        LoginMessageKind.FAILED ->
            Text(
                stringResource(R.string.settings_login_failed, ui.detail) +
                    if (cooldownSeconds > 0) " · " + stringResource(R.string.settings_wait, cooldownSeconds) else "",
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.error,
            )
        LoginMessageKind.COOLDOWN ->
            Text(
                stringResource(R.string.settings_wait, ui.seconds),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.error,
            )
        LoginMessageKind.NONE -> Unit
    }
}

private fun currentLanguage(): String = AppCompatDelegate.getApplicationLocales().toLanguageTags().substringBefore("-")

private fun setLanguage(tag: String) {
    val locales = if (tag.isEmpty()) LocaleListCompat.getEmptyLocaleList() else LocaleListCompat.forLanguageTags(tag)
    AppCompatDelegate.setApplicationLocales(locales)
}

/** Export per PROMPT.md 6.4: version, profile, preferences, lastScan, no credentials. */
fun exportConfig(vm: NodeViewModel): String =
    ConfigCodec.export(
        NodeProfile(
            mode = vm.mode.value,
            socksPort = 1080,
            theme = vm.themeMode.value,
            dynamicColor = vm.dynamicColor.value,
            autostart = vm.autostart.value,
            apiDns = vm.apiDns.value,
        ),
        vm.lastScanCached(),
    )

/** Import with schema + checksum validation; never applies a partial file. */
fun importConfig(
    vm: NodeViewModel,
    raw: String,
): Boolean {
    val profile = ConfigCodec.parse(raw) ?: return false
    vm.setMode(profile.mode)
    vm.setThemeMode(profile.theme)
    vm.setDynamicColor(profile.dynamicColor)
    vm.setAutostart(profile.autostart)
    vm.setApiDns(profile.apiDns)
    return true
}
