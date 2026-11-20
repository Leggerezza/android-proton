package io.github.dsh.protonnode.ui.screens

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
import io.github.dsh.protonnode.R
import io.github.dsh.protonnode.core.StatusSnapshot
import io.github.dsh.protonnode.ui.NodeViewModel
import io.github.dsh.protonnode.ui.theme.formatBytes
import io.github.dsh.protonnode.ui.theme.formatUptime

@Composable
fun HomeScreen(
    vm: NodeViewModel,
    expanded: Boolean,
) {
    val status by vm.status.collectAsStateWithLifecycle()
    val sessionInfo by vm.session.collectAsStateWithLifecycle()

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
                DetailsCard(status, Modifier.weight(1f))
                TrafficCard(status, Modifier.weight(1f))
            }
        } else {
            DetailsCard(status, Modifier.fillMaxWidth())
            TrafficCard(status, Modifier.fillMaxWidth())
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
        }
    }
}

@Composable
private fun TrafficCard(
    status: StatusSnapshot,
    modifier: Modifier,
) {
    OutlinedCard(modifier) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(stringResource(R.string.home_traffic), style = MaterialTheme.typography.labelLarge)
            infoRow("↓", formatBytes(status.rxBytes))
            infoRow("↑", formatBytes(status.txBytes))
            infoRow(stringResource(R.string.home_uptime), formatUptime(status.uptimeSec))
        }
    }
}

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
