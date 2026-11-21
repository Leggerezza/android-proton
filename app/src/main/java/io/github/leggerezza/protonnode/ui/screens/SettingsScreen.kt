// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.leggerezza.protonnode.ui.screens

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.net.Uri
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
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
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

@Composable
fun SettingsScreen(vm: NodeViewModel) {
    val context = LocalContext.current
    val themeMode by vm.themeMode.collectAsStateWithLifecycle()
    val dynamic by vm.dynamicColor.collectAsStateWithLifecycle()
    val mode by vm.mode.collectAsStateWithLifecycle()
    val autostart by vm.autostart.collectAsStateWithLifecycle()
    val gameLock by vm.gameLock.collectAsStateWithLifecycle()
    val apiDns by vm.apiDns.collectAsStateWithLifecycle()
    val sessionInfo by vm.session.collectAsStateWithLifecycle()
    val loginUi by vm.loginUi.collectAsStateWithLifecycle()
    var loginCooldownSeconds by remember { mutableStateOf(0L) }
    val preferredCountry by vm.preferredCountry.collectAsStateWithLifecycle()
    var countryText by remember(preferredCountry) { mutableStateOf(preferredCountry) }
    var countryError by remember { mutableStateOf(false) }
    var savedCountry by remember { mutableStateOf("") }

    var dnsText by remember(apiDns) { mutableStateOf(apiDns) }
    var loginEmail by remember { mutableStateOf("") }
    var loginPassword by remember { mutableStateOf("") }
    var loginTotp by remember { mutableStateOf("") }
    var showImport by remember { mutableStateOf(false) }
    var showSession by remember { mutableStateOf(false) }
    var sessionText by remember { mutableStateOf("") }
    var message by remember { mutableStateOf("") }
    val copiedMessage = stringResource(R.string.export_copied)
    val invalidMessage = stringResource(R.string.import_invalid)
    val okMessage = stringResource(R.string.dialog_ok)
    val savedMessage = stringResource(R.string.session_saved)
    val flclashCopiedMessage = stringResource(R.string.settings_flclash_copied)
    val flclashOkMessage = stringResource(R.string.settings_flclash_ok)
    val flclashBadMessage = stringResource(R.string.settings_flclash_bad)

    // A countdown keeps the button honest while the gate cools down.
    LaunchedEffect(loginUi.kind, loginUi.seconds) {
        var left = loginUi.seconds
        while (left > 0) {
            loginCooldownSeconds = left
            kotlinx.coroutines.delay(1000)
            left -= 1
        }
        loginCooldownSeconds = 0
    }

    Column(
        modifier =
            Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(24.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        Column(Modifier.widthIn(max = 720.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
            Text(stringResource(R.string.settings_title), style = MaterialTheme.typography.headlineSmall)

            Section(stringResource(R.string.settings_appearance))
            Text(stringResource(R.string.settings_theme), style = MaterialTheme.typography.labelLarge)
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

            Text(stringResource(R.string.settings_language), style = MaterialTheme.typography.labelLarge)
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                FilterChip(selected = currentLanguage().isEmpty(), onClick = {
                    setLanguage("")
                }, label = { Text(stringResource(R.string.settings_language_system)) })
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

            HorizontalDivider()
            Section(stringResource(R.string.settings_behavior))
            Text(stringResource(R.string.settings_mode), style = MaterialTheme.typography.labelLarge)
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                FilterChip(selected = mode == "normal", onClick = {
                    vm.setMode("normal")
                }, label = { Text(stringResource(R.string.servers_mode_normal)) })
                FilterChip(selected = mode == "game", onClick = {
                    vm.setMode("game")
                }, label = { Text(stringResource(R.string.servers_mode_game)) })
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
                // The saved value is shown through a composed string, not getString().
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
            Button(onClick = {
                vm.setApiDns(dnsText.trim())
                message = "saved"
            }) { Text(stringResource(R.string.dialog_ok)) }

            HorizontalDivider()
            Section(stringResource(R.string.settings_account))
            SessionStatusLine(sessionInfo, loginUi)
            if (sessionInfo.loggedIn) {
                OutlinedButton(onClick = { vm.logout() }) { Text(stringResource(R.string.settings_logout)) }
            }
            OutlinedTextField(
                value = loginEmail,
                onValueChange = { loginEmail = it },
                label = { Text(stringResource(R.string.settings_email)) },
                singleLine = true,
                enabled = !loginUi.busy && !sessionInfo.isUsable,
                modifier = Modifier.fillMaxWidth(),
            )
            OutlinedTextField(
                value = loginPassword,
                onValueChange = { loginPassword = it },
                label = { Text(stringResource(R.string.settings_password)) },
                singleLine = true,
                enabled = !loginUi.busy && !sessionInfo.isUsable,
                visualTransformation =
                    androidx.compose.ui.text.input
                        .PasswordVisualTransformation(),
                modifier = Modifier.fillMaxWidth(),
            )
            OutlinedTextField(
                value = loginTotp,
                onValueChange = { loginTotp = it },
                label = { Text(stringResource(R.string.settings_totp)) },
                singleLine = true,
                enabled = !loginUi.busy && !sessionInfo.isUsable,
                modifier = Modifier.fillMaxWidth(),
            )
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                Button(
                    onClick = { vm.signIn(loginEmail, loginPassword, loginTotp) },
                    enabled = !loginUi.busy && !sessionInfo.isUsable && loginCooldownSeconds == 0L,
                ) {
                    Text(
                        when {
                            sessionInfo.isUsable -> stringResource(R.string.settings_already_signed_in)
                            loginUi.busy -> stringResource(R.string.settings_signing_in)
                            loginCooldownSeconds > 0 -> stringResource(R.string.settings_wait, loginCooldownSeconds)
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
            LoginFeedback(loginUi, loginCooldownSeconds)
            if (BuildConfig.DEBUG) {
                OutlinedButton(
                    onClick = { showSession = true },
                ) { Text(stringResource(R.string.settings_debug_session)) }
            }

            HorizontalDivider()
            Section(stringResource(R.string.settings_flclash))
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
                                .generate(context.packageName),
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
                            flclashBadMessage + " " +
                                problems.joinToString("; ")
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

            if (message.isNotEmpty()) {
                Text(message, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.primary)
            }
        }
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
                TextButton(
                    onClick = { showImport = false },
                ) { Text(stringResource(R.string.dialog_cancel)) }
            },
        )
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
                TextButton(
                    onClick = { showSession = false },
                ) { Text(stringResource(R.string.dialog_cancel)) }
            },
        )
    }
}

@Composable
private fun Section(title: String) {
    Text(title, style = MaterialTheme.typography.titleMedium)
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
    if (info.isExpired && info.authError.isNotEmpty()) {
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
