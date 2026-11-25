// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.leggerezza.protonnode.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
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
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.leggerezza.protonnode.R
import io.github.leggerezza.protonnode.ui.NodeViewModel
import org.json.JSONArray

data class ServerRow(
    val name: String,
    val country: String,
    val load: Int,
    val score: Double,
    val secureCore: Boolean,
    val p2p: Boolean,
    val tor: Boolean,
    val streaming: Boolean,
    val ipv6: Boolean,
    val entryIP: String,
    val rttMs: Long,
)

private val GAME_ORDER = listOf("JP", "HK", "KR", "SG", "US")

fun parseServers(json: String): List<ServerRow> =
    runCatching {
        val array = JSONArray(json)
        (0 until array.length()).map { i ->
            val o = array.getJSONObject(i)
            ServerRow(
                name = o.optString("name"),
                country = o.optString("exitCountry"),
                load = o.optInt("load"),
                score = o.optDouble("score", 0.0),
                secureCore = o.optBoolean("secureCore", false),
                p2p = o.optBoolean("p2p", false),
                tor = o.optBoolean("tor", false),
                streaming = o.optBoolean("streaming", false),
                ipv6 = o.optBoolean("ipv6", false),
                entryIP = o.optString("entryIP", ""),
                rttMs = o.optLong("rttMs", 0L),
            )
        }
    }.getOrDefault(emptyList())

@Composable
fun ServersScreen(vm: NodeViewModel) {
    val serversJson by vm.servers.collectAsStateWithLifecycle()
    val sessionInfo by vm.session.collectAsStateWithLifecycle()
    val mode by vm.mode.collectAsStateWithLifecycle()
    val pinnedServer by vm.preferredServer.collectAsStateWithLifecycle()
    val pinnedCountry by vm.preferredCountry.collectAsStateWithLifecycle()
    var query by remember { mutableStateOf("") }

    // Load (or refresh) when the screen opens: the ViewModel's initial load may
    // have raced the session restore.
    LaunchedEffect(Unit) { vm.loadServers() }
    var gameOnly by remember(mode) { mutableStateOf(false) }
    var featureFilter by remember { mutableStateOf("") }
    var byLatency by remember { mutableStateOf(false) }
    var groupByCountry by remember { mutableStateOf(false) }
    val status by vm.status.collectAsStateWithLifecycle()
    val currentServer = status.server

    val all = remember(serversJson) { parseServers(serversJson) }
    val signedIn = sessionInfo.isUsable

    val filtered =
        all
            .asSequence()
            .filter { row ->
                val q = query.trim()
                q.isEmpty() || row.name.contains(q, ignoreCase = true) || row.country.contains(q, ignoreCase = true)
            }
            // PROMPT.md 7: game mode never offers Secure Core, and Tor exits are
            // far too slow for it (two extra hops).
            .filter { row -> mode != "game" || !row.secureCore }
            .filter { row -> mode != "game" || !row.tor }
            .filter { row ->
                when (featureFilter) {
                    "p2p" -> row.p2p
                    "tor" -> row.tor
                    "secure-core" -> row.secureCore
                    "streaming" -> row.streaming
                    else -> true
                }
            }.filter { row -> !(mode == "game" && gameOnly) || row.country in GAME_ORDER }
            .sortedWith(
                if (byLatency) {
                    compareBy({ if (it.rttMs > 0) 0 else 1 }, { it.rttMs })
                } else if (mode == "game") {
                    compareBy({ GAME_ORDER.indexOf(it.country).let { i -> if (i < 0) 99 else i } }, { it.load })
                } else {
                    compareBy({ it.name })
                },
            ).toList()

    Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Column(Modifier.widthIn(max = 900.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text(stringResource(R.string.servers_title), style = MaterialTheme.typography.headlineSmall)
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                Text(
                    stringResource(R.string.servers_selection) + ": " +
                        when {
                            pinnedServer.isNotEmpty() -> pinnedServer
                            pinnedCountry.isNotEmpty() -> pinnedCountry
                            else -> stringResource(R.string.servers_auto)
                        },
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.primary,
                    modifier = Modifier.weight(1f),
                )
                if (pinnedServer.isNotEmpty() || pinnedCountry.isNotEmpty()) {
                    TextButton(onClick = { vm.clearSelection() }) { Text(stringResource(R.string.servers_auto)) }
                }
            }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                listOf("US", "JP", "HK", "SG", "KR", "DE", "NL", "GB").forEach { cc ->
                    FilterChip(
                        selected = pinnedCountry == cc && pinnedServer.isEmpty(),
                        onClick = { vm.selectCountry(cc) },
                        label = { Text(cc) },
                    )
                }
            }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                FilterChip(
                    selected = mode == "game",
                    onClick = { vm.setMode("game") },
                    label = { Text(stringResource(R.string.servers_mode_game)) },
                )
                FilterChip(
                    selected = mode == "normal",
                    onClick = { vm.setMode("normal") },
                    label = { Text(stringResource(R.string.servers_mode_normal)) },
                )
                if (mode == "game") {
                    FilterChip(
                        selected = gameOnly,
                        onClick = { gameOnly = !gameOnly },
                        label = { Text("JP/HK/KR/SG/US") },
                    )
                }
                FilterChip(
                    selected = byLatency,
                    onClick = { byLatency = !byLatency },
                    label = { Text("By latency") },
                )
            }
            Row(
                modifier = Modifier.horizontalScroll(rememberScrollState()),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                listOf("p2p" to "P2P", "tor" to "Tor", "secure-core" to "Secure Core", "streaming" to "Streaming")
                    .forEach { (value, label) ->
                        FilterChip(
                            selected = featureFilter == value,
                            onClick = { featureFilter = if (featureFilter == value) "" else value },
                            label = { Text(label) },
                        )
                    }
            }
            OutlinedTextField(
                value = query,
                onValueChange = { query = it },
                label = { Text(stringResource(R.string.servers_search)) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )
            if (!signedIn) {
                Text(stringResource(R.string.servers_not_signed_in), style = MaterialTheme.typography.bodyMedium)
            } else if (filtered.isEmpty()) {
                Text(stringResource(R.string.servers_empty), style = MaterialTheme.typography.bodyMedium)
            } else {
                Text(filtered.size.toString() + " / " + all.size, style = MaterialTheme.typography.labelMedium)
                if (groupByCountry) {
                    CountryList(filtered, onPick = { vm.selectCountry(it) })
                } else {
                    LazyColumn(Modifier.fillMaxWidth()) {
                        items(filtered, key = { it.name }) { row ->
                            val selected = pinnedServer == row.name
                            Row(
                                Modifier
                                    .fillMaxWidth()
                                    .background(
                                        if (selected) {
                                            MaterialTheme.colorScheme.secondaryContainer
                                        } else {
                                            MaterialTheme.colorScheme.surface
                                        },
                                    ).clickable { vm.selectServer(row.name, row.country) }
                                    .padding(horizontal = 8.dp, vertical = 12.dp),
                                verticalAlignment = Alignment.CenterVertically,
                            ) {
                                Column(Modifier.weight(1f)) {
                                    Text(row.name, style = MaterialTheme.typography.titleSmall)
                                    val badges =
                                        buildList {
                                            if (row.secureCore) add("Secure Core")
                                            if (row.tor) add("Tor")
                                            if (row.p2p) add("P2P")
                                            if (row.streaming) add("Streaming")
                                            if (row.ipv6) add("IPv6")
                                        }
                                    Text(
                                        row.country +
                                            (if (row.name == currentServer) " · Current" else "") +
                                            (if (badges.isNotEmpty()) " · " + badges.joinToString(" · ") else "") +
                                            (if (row.entryIP.isNotEmpty()) " · " + row.entryIP else "") +
                                            " · " + (if (row.rttMs > 0) row.rttMs.toString() + " ms" else "—"),
                                        style = MaterialTheme.typography.bodySmall,
                                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                                    )
                                }
                                if (selected) {
                                    Icon(
                                        Icons.Filled.Check,
                                        contentDescription = null,
                                        modifier = Modifier.padding(end = 8.dp),
                                    )
                                }
                                Text(row.load.toString() + "%", style = MaterialTheme.typography.bodySmall)
                            }
                            HorizontalDivider()
                        }
                    }
                }
            }
        }
    }
}

/** ISO-3166 alpha-2 to a flag emoji (regional indicator pair). */
fun flagEmoji(code: String): String {
    val upper = code.trim().uppercase()
    if (upper.length != 2 || upper.any { it !in 'A'..'Z' }) return ""
    return upper.map { Char(0x1F1E6 + (it - 'A')) }.joinToString("")
}

/**
 * Country view: one row per exit country, ranked by the fastest measured entry.
 *
 * A country whose servers were never verified shows "not measured" instead of a
 * number - a blank latency said nothing about whether the entry was skipped,
 * probed and timed out, or simply never reached.
 */
@Composable
private fun CountryList(
    rows: List<ServerRow>,
    onPick: (String) -> Unit,
) {
    val ordered =
        remember(rows) {
            rows
                .groupBy { it.country }
                .mapValues { (_, list) ->
                    val measured = list.mapNotNull { it.rttMs.takeIf { ms -> ms > 0 } }
                    CountryAgg(list.size, measured.minOrNull() ?: 0L)
                }.toList()
                .sortedWith(
                    compareBy({ if (it.second.minRttMs > 0) 0 else 1 }, { it.second.minRttMs }, { -it.second.count }),
                )
        }
    LazyColumn(Modifier.fillMaxWidth()) {
        items(ordered, key = { it.first }) { (code, agg) ->
            Row(
                Modifier
                    .fillMaxWidth()
                    .clickable { onPick(code) }
                    .padding(horizontal = 8.dp, vertical = 12.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Column(Modifier.weight(1f)) {
                    Text(flagEmoji(code) + "  " + countryName(code), style = MaterialTheme.typography.titleSmall)
                    Text(
                        code + " · " + agg.count + " servers",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                Text(
                    if (agg.minRttMs >
                        0
                    ) {
                        agg.minRttMs.toString() + " ms"
                    } else {
                        stringResource(R.string.servers_not_measured)
                    },
                    style = MaterialTheme.typography.bodySmall,
                    color =
                        if (agg.minRttMs > 0) {
                            MaterialTheme.colorScheme.onSurfaceVariant
                        } else {
                            MaterialTheme.colorScheme.outline
                        },
                )
            }
            HorizontalDivider()
        }
    }
}

private data class CountryAgg(
    val count: Int,
    val minRttMs: Long,
)
