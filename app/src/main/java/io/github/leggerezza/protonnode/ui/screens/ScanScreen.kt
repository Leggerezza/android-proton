// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.leggerezza.protonnode.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.material3.Button
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.leggerezza.protonnode.R
import io.github.leggerezza.protonnode.ui.NodeViewModel
import org.json.JSONObject

data class ScanRow(
    val server: String,
    val entryIP: String,
    val rttMs: Long,
    val level: Int,
    val country: String,
)

fun parseScanResults(json: String): List<ScanRow> =
    runCatching {
        val o = JSONObject(json)
        val arr = o.optJSONArray("results") ?: return emptyList()
        (0 until arr.length()).map { i ->
            val r = arr.getJSONObject(i)
            ScanRow(
                server = r.optString("server"),
                entryIP = r.optString("entryIP"),
                rttMs = r.optLong("rttMs", 0),
                level = r.optInt("level", 0),
                country = r.optString("country", ""),
            )
        }
    }.getOrDefault(emptyList())

@Composable
fun ScanScreen(
    vm: NodeViewModel,
    onUseServer: (name: String, country: String) -> Unit,
) {
    val progress by vm.scanProgress.collectAsStateWithLifecycle()
    val resultsJson by vm.scanResults.collectAsStateWithLifecycle()
    var scope by remember { mutableStateOf("all") }

    val state = NodeViewModel.jsonString(progress, "state")
    val stage = NodeViewModel.jsonString(progress, "stage")
    val done = NodeViewModel.jsonInt(progress, "done")
    val total = NodeViewModel.jsonInt(progress, "total")
    val passed = NodeViewModel.jsonInt(progress, "passed")
    val running = state == "running"
    val rows = remember(resultsJson) { parseScanResults(resultsJson) }

    Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Column(Modifier.widthIn(max = 900.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text(stringResource(R.string.scan_title), style = MaterialTheme.typography.headlineSmall)
            Text(
                stringResource(R.string.scan_level_hint),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                listOf(
                    "all" to R.string.scan_scope_all,
                    "game" to R.string.scan_scope_game,
                    "secure-core" to R.string.scan_scope_secure,
                ).forEach { (value, label) ->
                    FilterChip(
                        selected = scope == value,
                        enabled = !running,
                        onClick = { scope = value },
                        label = { Text(stringResource(label)) },
                    )
                }
            }
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                Button(onClick = { vm.startScan(scope) }, enabled = !running) {
                    Text(stringResource(R.string.scan_start))
                }
                if (running) {
                    OutlinedButton(onClick = { vm.cancelScan() }) {
                        Text(stringResource(R.string.scan_cancel))
                    }
                }
            }
            if (total > 0) {
                LinearProgressIndicator(
                    progress = { if (total == 0L) 0f else (done.toFloat() / total.toFloat()).coerceIn(0f, 1f) },
                    modifier = Modifier.fillMaxWidth(),
                )
                Text(
                    stringResource(
                        R.string.scan_stage,
                        stage.ifEmpty { "—" },
                        done.toInt(),
                        total.toInt(),
                        passed.toInt(),
                    ),
                    style = MaterialTheme.typography.bodySmall,
                )
            }
            if (NodeViewModel.jsonString(progress, "state") == "cancelled") {
                Text("cancelled", style = MaterialTheme.typography.bodySmall)
            }
            if (rows.isEmpty()) {
                Text(stringResource(R.string.scan_no_results), style = MaterialTheme.typography.bodyMedium)
            } else {
                Text(
                    stringResource(R.string.scan_use_hint),
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                rows.forEach { row ->
                    Row(
                        Modifier.fillMaxWidth().padding(vertical = 8.dp),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = androidx.compose.ui.Alignment.CenterVertically,
                    ) {
                        Column {
                            Text(row.server, style = MaterialTheme.typography.titleSmall)
                            Text(
                                row.entryIP + " · L" + row.level +
                                    (if (row.country.isNotEmpty()) " · " + row.country else ""),
                                style = MaterialTheme.typography.bodySmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                        Row(verticalAlignment = androidx.compose.ui.Alignment.CenterVertically) {
                            Text(row.rttMs.toString() + " ms", style = MaterialTheme.typography.bodySmall)
                            androidx.compose.foundation.layout
                                .Spacer(Modifier.width(12.dp))
                            OutlinedButton(onClick = { onUseServer(row.server, row.country) }) {
                                Text(stringResource(R.string.scan_use))
                            }
                        }
                    }
                    HorizontalDivider()
                }
            }
        }
    }
}
