// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.leggerezza.protonnode.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Checkbox
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import io.github.leggerezza.protonnode.R
import java.util.Locale

/**
 * Picks exit countries that must never be connected.
 *
 * The list is derived from the loaded server list, so it only contains countries
 * Proton actually offers; names come from the platform locale so they read the
 * same as the rest of the system UI.
 */
@Composable
fun ExcludedCountriesDialog(
    serversJson: String,
    selected: Set<String>,
    onToggle: (String) -> Unit,
    onDismiss: () -> Unit,
) {
    var query by remember { mutableStateOf("") }
    val countries =
        remember(serversJson) {
            parseServers(serversJson)
                .groupingBy { it.country }
                .eachCount()
                .toList()
                .sortedBy { it.first }
        }
    val filtered =
        countries.filter { (code, _) ->
            query.isBlank() ||
                code.contains(query.trim(), ignoreCase = true) ||
                countryName(code).contains(query.trim(), ignoreCase = true)
        }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.settings_excluded_title)) },
        text = {
            Column {
                Text(
                    stringResource(R.string.settings_excluded_hint),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                OutlinedTextField(
                    value = query,
                    onValueChange = { query = it },
                    label = { Text(stringResource(R.string.servers_search)) },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                LazyColumn(Modifier.height(360.dp)) {
                    items(filtered, key = { it.first }) { (code, count) ->
                        Row(
                            Modifier.fillMaxWidth(),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.SpaceBetween,
                        ) {
                            Checkbox(
                                checked = code in selected,
                                onCheckedChange = { onToggle(code) },
                            )
                            Text(countryName(code), modifier = Modifier.weight(1f))
                            Text(
                                "$code · $count",
                                style = MaterialTheme.typography.bodySmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                        HorizontalDivider()
                    }
                }
            }
        },
        confirmButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.dialog_ok)) }
        },
    )
}

/** Localised country name for an ISO-3166 alpha-2 code, falling back to the code. */
fun countryName(code: String): String =
    runCatching {
        Locale
            .Builder()
            .setRegion(code)
            .build()
            .getDisplayCountry(Locale.getDefault())
    }.getOrNull()?.takeIf { it.isNotBlank() && it != code } ?: code
