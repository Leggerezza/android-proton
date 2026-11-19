package io.github.dsh.protonnode.net

import android.content.Context
import android.net.ConnectivityManager
import android.net.Network
import android.net.NetworkCapabilities
import android.util.Log

/**
 * Watches the default network and reports a stable fingerprint.
 *
 * The fingerprint feeds the scan cache (PROMPT.md 5.5: invalidated on a network
 * switch) and triggers an immediate reconnect (6.2). It deliberately contains no
 * SSID/user data: only the transport type and the network handle.
 */
object NetworkMonitor {
    private const val TAG = "NetworkMonitor"

    private var manager: ConnectivityManager? = null
    private var callback: ConnectivityManager.NetworkCallback? = null

    fun start(
        context: Context,
        onChanged: (String) -> Unit,
    ) {
        if (callback != null) return
        val cm = context.getSystemService(ConnectivityManager::class.java) ?: return
        manager = cm
        val cb =
            object : ConnectivityManager.NetworkCallback() {
                override fun onAvailable(network: Network) {
                    report(cm, network, onChanged)
                }

                override fun onCapabilitiesChanged(
                    network: Network,
                    capabilities: NetworkCapabilities,
                ) {
                    report(cm, network, onChanged)
                }

                override fun onLost(network: Network) {
                    Log.i(TAG, "network lost")
                }
            }
        runCatching { cm.registerDefaultNetworkCallback(cb) }
            .onSuccess { callback = cb }
            .onFailure { Log.w(TAG, "cannot register the network callback: " + it.message) }
    }

    fun stop() {
        val cb = callback ?: return
        runCatching { manager?.unregisterNetworkCallback(cb) }
        callback = null
        manager = null
    }

    private fun report(
        cm: ConnectivityManager,
        network: Network,
        onChanged: (String) -> Unit,
    ) {
        val caps = cm.getNetworkCapabilities(network) ?: return
        val fingerprint = fingerprintOf(transportOf(caps), network.networkHandle)
        Log.i(TAG, "network " + fingerprint)
        onChanged(fingerprint)
    }

    /** Pure helper, unit-tested: "wifi:123" / "cellular:456" / "other:789". */
    fun fingerprintOf(
        transport: String,
        handle: Long,
    ): String = transport + ":" + handle

    /** Maps a [NetworkCapabilities] to the transport name used in the fingerprint. */
    fun transportOf(capabilities: NetworkCapabilities): String =
        when {
            capabilities.hasTransport(NetworkCapabilities.TRANSPORT_WIFI) -> "wifi"
            capabilities.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR) -> "cellular"
            capabilities.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET) -> "ethernet"
            capabilities.hasTransport(NetworkCapabilities.TRANSPORT_VPN) -> "vpn"
            else -> "other"
        }
}
