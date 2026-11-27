// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.leggerezza.protonnode.ui.screens

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material3.Button
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.leggerezza.protonnode.R
import io.github.leggerezza.protonnode.core.LogExport
import io.github.leggerezza.protonnode.core.LogLevel
import io.github.leggerezza.protonnode.core.classifyLogLine
import io.github.leggerezza.protonnode.ui.NodeViewModel

@Composable
fun LogsScreen(vm: NodeViewModel) {
    val logs by vm.logs.collectAsStateWithLifecycle()
    val status by vm.status.collectAsStateWithLifecycle()
    val session by vm.session.collectAsStateWithLifecycle()
    val mode by vm.mode.collectAsStateWithLifecycle()
    val transport by vm.transport.collectAsStateWithLifecycle()
    val mtu by vm.mtu.collectAsStateWithLifecycle()
    val ipv6 by vm.ipv6.collectAsStateWithLifecycle()
    val excluded by vm.excludedCountries.collectAsStateWithLifecycle()
    val context = LocalContext.current
    var savedAt by remember { mutableStateOf("") }
    val settings =
        remember(mode, transport, mtu, ipv6, excluded) {
            listOf(
                "mode" to mode,
                "transport" to transport,
                "mtu" to mtu.toString(),
                "ipv6" to ipv6.toString(),
                "excludedCountries" to excluded.joinToString(","),
            )
        }

    LaunchedEffect(Unit) { vm.loadLogs() }

    val allLines = remember(logs) { logs.split("\n").filter { it.isNotBlank() } }
    var filter by remember { mutableStateOf<LogLevel?>(null) }
    val lines =
        remember(allLines, filter) {
            if (filter == null) allLines else allLines.filter { classifyLogLine(it) == filter }
        }
    val counts =
        remember(allLines) {
            LogLevel.entries.associateWith { level -> allLines.count { classifyLogLine(it) == level } }
        }

    // The newest line is the interesting one: open at the bottom and follow it.
    val listState = rememberLazyListState()
    LaunchedEffect(lines.size) {
        if (lines.isNotEmpty()) listState.scrollToItem(lines.lastIndex)
    }

    Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            Button(onClick = { vm.loadLogs() }) { Text(stringResource(R.string.logs_refresh)) }
            OutlinedButton(onClick = {
                val clipboard = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
                clipboard.setPrimaryClip(ClipData.newPlainText("proton logs", logs))
            }) { Text(stringResource(R.string.logs_copy)) }
            OutlinedButton(onClick = {
                // Share packages the report into a real .log file first, the same
                // shape Proton's own client sends.
                val report = LogExport.build(context, status, session, settings, logs)
                val file = LogExport.writeCacheFile(context, report)
                LogExport.share(context, file)
            }) { Text(stringResource(R.string.logs_share)) }
            OutlinedButton(onClick = {
                val report = LogExport.build(context, status, session, settings, logs)
                savedAt = LogExport.saveToDownloads(context, report) ?: ""
            }) { Text(stringResource(R.string.logs_save)) }
        }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            FilterChip(
                selected = filter == null,
                onClick = { filter = null },
                label = { Text(stringResource(R.string.logs_filter_all) + " " + allLines.size) },
            )
            LogLevel.entries.forEach { level ->
                FilterChip(
                    selected = filter == level,
                    onClick = { filter = if (filter == level) null else level },
                    label = { Text(level.label + " " + (counts[level] ?: 0)) },
                )
            }
        }
        if (savedAt.isNotEmpty()) {
            Text(stringResource(R.string.logs_saved, savedAt), style = MaterialTheme.typography.labelSmall)
        }
        Text(
            stringResource(R.string.logs_secure_note),
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        if (lines.isEmpty()) {
            Text(stringResource(R.string.logs_empty), style = MaterialTheme.typography.bodyMedium)
        } else {
            LazyColumn(Modifier.fillMaxWidth(), state = listState) {
                items(lines) { line ->
                    val level = classifyLogLine(line)
                    Text(
                        withLevel(line, level),
                        fontFamily = FontFamily.Monospace,
                        fontSize = 12.sp,
                        style = MaterialTheme.typography.bodySmall,
                        color =
                            when (level) {
                                LogLevel.ERROR -> MaterialTheme.colorScheme.error
                                LogLevel.WARN -> MaterialTheme.colorScheme.tertiary
                                LogLevel.INFO -> MaterialTheme.colorScheme.onSurface
                            },
                        modifier = Modifier.padding(vertical = 1.dp),
                    )
                }
            }
        }
    }
}

@Composable
private fun levelColor(level: LogLevel): Color =
    when (level) {
        LogLevel.ERROR -> MaterialTheme.colorScheme.error
        LogLevel.WARN -> MaterialTheme.colorScheme.tertiary
        LogLevel.INFO -> MaterialTheme.colorScheme.onSurfaceVariant
    }

/** "09:19:30.429 INFO core: …" - the level lives in the line, after the clock. */
private fun withLevel(
    line: String,
    level: LogLevel,
): String {
    val match = Regex("^(\\d{2}:\\d{2}:\\d{2}\\.\\d{3})\\s+(.*)$").find(line)
    return if (match != null) {
        match.groupValues[1] + " " + level.label + " " + match.groupValues[2]
    } else {
        level.label + " " + line
    }
}
