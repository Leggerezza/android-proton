package io.github.dsh.protonnode.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
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
import io.github.dsh.protonnode.R
import io.github.dsh.protonnode.ui.NodeViewModel
import org.json.JSONArray

data class ServerRow(
    val name: String,
    val country: String,
    val load: Int,
    val score: Double,
    val secureCore: Boolean,
    val entryIP: String,
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
                entryIP = o.optString("entryIP", ""),
            )
        }
    }.getOrDefault(emptyList())

@Composable
fun ServersScreen(vm: NodeViewModel) {
    val serversJson by vm.servers.collectAsStateWithLifecycle()
    val session by vm.session.collectAsStateWithLifecycle()
    val mode by vm.mode.collectAsStateWithLifecycle()
    var query by remember { mutableStateOf("") }

    // Load (or refresh) when the screen opens: the ViewModel's initial load may
    // have raced the session restore.
    LaunchedEffect(Unit) { vm.loadServers() }
    var gameOnly by remember(mode) { mutableStateOf(false) }

    val all = remember(serversJson) { parseServers(serversJson) }
    val signedIn = session.contains("\"loggedIn\":true")

    val filtered =
        all
            .asSequence()
            .filter { row ->
                val q = query.trim()
                q.isEmpty() || row.name.contains(q, ignoreCase = true) || row.country.contains(q, ignoreCase = true)
            }
            // PROMPT.md 7: game mode never offers Secure Core.
            .filter { row -> mode != "game" || !row.secureCore }
            .filter { row -> !(mode == "game" && gameOnly) || row.country in GAME_ORDER }
            .sortedWith(
                if (mode == "game") {
                    compareBy({ GAME_ORDER.indexOf(it.country).let { i -> if (i < 0) 99 else i } }, { it.load })
                } else {
                    compareBy({ it.name })
                },
            ).toList()

    Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Column(Modifier.widthIn(max = 900.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text(stringResource(R.string.servers_title), style = MaterialTheme.typography.headlineSmall)
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
                LazyColumn(Modifier.fillMaxWidth()) {
                    items(filtered, key = { it.name }) { row ->
                        Row(
                            Modifier.fillMaxWidth().padding(vertical = 12.dp),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            Column(Modifier.weight(1f)) {
                                Text(row.name, style = MaterialTheme.typography.titleSmall)
                                Text(
                                    row.country + (if (row.secureCore) " · Secure Core" else "") +
                                        (if (row.entryIP.isNotEmpty()) " · " + row.entryIP else ""),
                                    style = MaterialTheme.typography.bodySmall,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
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
