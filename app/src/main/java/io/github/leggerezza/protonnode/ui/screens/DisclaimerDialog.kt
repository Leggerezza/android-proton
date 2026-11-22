// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.leggerezza.protonnode.ui.screens

import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.res.stringResource
import io.github.leggerezza.protonnode.R
import kotlinx.coroutines.delay

/** Seconds the notice stays up before Continue becomes available. */
private const val DISCLAIMER_DELAY_SECONDS = 3

/**
 * First-run notice: this is an unofficial, third-party client and ships no VPN
 * servers of its own. It cannot be dismissed by touching outside or by Back, and
 * Continue stays disabled for a few seconds so the text is actually read.
 */
@Composable
fun DisclaimerDialog(onAccept: () -> Unit) {
    var remaining by remember { mutableIntStateOf(DISCLAIMER_DELAY_SECONDS) }
    LaunchedEffect(Unit) {
        while (remaining > 0) {
            delay(1000)
            remaining--
        }
    }
    AlertDialog(
        onDismissRequest = { /* deliberate: the notice is acknowledged, not dismissed */ },
        title = { Text(stringResource(R.string.disclaimer_title)) },
        text = { Text(stringResource(R.string.disclaimer_body)) },
        confirmButton = {
            TextButton(enabled = remaining <= 0, onClick = onAccept) {
                Text(
                    if (remaining > 0) {
                        stringResource(R.string.disclaimer_continue_in, remaining)
                    } else {
                        stringResource(R.string.disclaimer_continue)
                    },
                )
            }
        },
    )
}
