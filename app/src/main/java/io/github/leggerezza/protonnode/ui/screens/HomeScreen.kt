// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.leggerezza.protonnode.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.leggerezza.protonnode.R
import io.github.leggerezza.protonnode.core.StatusSnapshot
import io.github.leggerezza.protonnode.ui.NodeViewModel
import io.github.leggerezza.protonnode.ui.theme.formatBytes
import io.github.leggerezza.protonnode.ui.theme.formatUptime

@Composable
fun HomeScreen(
    vm: NodeViewModel,
    expanded: Boolean,
) {
    val status by vm.status.collectAsStateWithLifecycle()
    val transport by vm.transport.collectAsStateWithLifecycle()
    val ipv6 by vm.ipv6.collectAsStateWithLifecycle()
    val rxRate by vm.rxRate.collectAsStateWithLifecycle()
    val txRate by vm.txRate.collectAsStateWithLifecycle()
    LaunchedEffect(Unit) {
        while (true) {
            vm.tick()
            kotlinx.coroutines.delay(1000)
        }
    }
    val sessionInfo by vm.session.collectAsStateWithLifecycle()
    val pinnedServer by vm.preferredServer.collectAsStateWithLifecycle()

    LaunchedEffect(Unit) {
        while (true) {
            vm.refreshAll()
            kotlinx.coroutines.delay(1500)
        }
    }

    val signedIn = sessionInfo.isUsable

    Column(
        modifier =
            Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(24.dp)
                .widthIn(max = 1100.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        StatusCard(status)
        Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            if (status.isRunning) {
                Button(onClick = { vm.disconnect() }) {
                    Text(stringResource(R.string.action_disconnect))
                }
            } else {
                Button(onClick = { vm.connect() }, enabled = signedIn) {
                    Text(stringResource(R.string.action_connect))
                }
            }
        }
        if (!signedIn) {
            OutlinedCard(Modifier.fillMaxWidth()) {
                Text(
                    if (sessionInfo.isExpired) {
                        stringResource(R.string.home_session_expired)
                    } else {
                        stringResource(R.string.home_no_session)
                    },
                    Modifier.padding(16.dp),
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
        }
        if (expanded) {
            Row(horizontalArrangement = Arrangement.spacedBy(16.dp)) {
                DetailsCard(status, transport, ipv6, Modifier.weight(1f))
                TrafficCard(status, rxRate, txRate, Modifier.weight(1f))
            }
        } else {
            DetailsCard(status, transport, ipv6, Modifier.fillMaxWidth())
            TrafficCard(status, rxRate, txRate, Modifier.fillMaxWidth())
        }
        if (pinnedServer.isNotEmpty()) {
            Text(
                stringResource(R.string.home_pinned, pinnedServer),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.primary,
            )
        }
        if (status.lastError.isNotEmpty()) {
            OutlinedCard(Modifier.fillMaxWidth()) {
                Column(Modifier.padding(16.dp)) {
                    Text(stringResource(R.string.home_last_error), style = MaterialTheme.typography.labelLarge)
                    Spacer(Modifier.height(4.dp))
                    Text(status.lastError, style = MaterialTheme.typography.bodySmall)
                }
            }
        }
    }
}

@Composable
private fun StatusCard(status: StatusSnapshot) {
    Card(Modifier.fillMaxWidth()) {
        Column(Modifier.padding(24.dp)) {
            Text(stateLabel(status.state), style = MaterialTheme.typography.headlineMedium)
            Spacer(Modifier.height(8.dp))
            if (status.server.isNotEmpty()) {
                Text(status.server, style = MaterialTheme.typography.titleMedium)
                Text(
                    status.entryIP + " · " + status.sni,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (status.isRunning && status.socksPort > 0) {
                Spacer(Modifier.height(8.dp))
                Text(
                    stringResource(R.string.home_served_port, status.socksPort),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

@Composable
private fun stateLabel(state: String): String =
    when (state) {
        StatusSnapshot.STATE_CONNECTED -> stringResource(R.string.state_connected)
        StatusSnapshot.STATE_SCANNING -> stringResource(R.string.state_scanning)
        StatusSnapshot.STATE_CONNECTING -> stringResource(R.string.state_connecting)
        StatusSnapshot.STATE_RECONNECTING -> stringResource(R.string.state_reconnecting)
        else -> stringResource(R.string.state_idle)
    }

@Composable
private fun DetailsCard(
    status: StatusSnapshot,
    transport: String,
    ipv6: Boolean,
    modifier: Modifier,
) {
    OutlinedCard(modifier) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(stringResource(R.string.home_server), style = MaterialTheme.typography.labelLarge)
            infoRow(stringResource(R.string.home_entry), status.entryIP.ifEmpty { "—" })
            infoRow(
                stringResource(R.string.home_handshake),
                if (status.handshakeMs > 0) status.handshakeMs.toString() + " ms" else "—",
            )
            infoRow(stringResource(R.string.home_mode), status.mode.ifEmpty { "—" })
            infoRow(
                stringResource(R.string.home_transport),
                if (transport ==
                    "udp"
                ) {
                    stringResource(R.string.settings_transport_udp)
                } else {
                    stringResource(R.string.settings_transport_stealth)
                },
            )
            infoRow(stringResource(R.string.home_ipv6), if (ipv6) "on" else "off")
        }
    }
}

@Composable
private fun TrafficCard(
    status: StatusSnapshot,
    rxRate: Long,
    txRate: Long,
    modifier: Modifier,
) {
    OutlinedCard(modifier) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(stringResource(R.string.home_traffic), style = MaterialTheme.typography.labelLarge)
            // Rate first: it answers "is it slow right now?", the totals do not.
            infoRow("↓ " + formatRate(rxRate), formatBytes(status.rxBytes))
            infoRow("↑ " + formatRate(txRate), formatBytes(status.txBytes))
            infoRow(stringResource(R.string.home_uptime), formatUptime(status.uptimeSec))
        }
    }
}

/** Formats bytes per second, e.g. "12.3 KB/s". */
private fun formatRate(bytesPerSecond: Long): String =
    if (bytesPerSecond <= 0) "0 B/s" else formatBytes(bytesPerSecond) + "/s"

@Composable
private fun infoRow(
    label: String,
    value: String,
) {
    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
        Text(label, style = MaterialTheme.typography.bodyMedium)
        Spacer(Modifier.width(12.dp))
        Text(value, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}
