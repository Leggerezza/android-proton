package io.github.dsh.protonnode.ui

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import io.github.dsh.protonnode.core.CoreController
import io.github.dsh.protonnode.core.StatusSnapshot
import io.github.dsh.protonnode.data.Prefs
import io.github.dsh.protonnode.data.SessionStore
import io.github.dsh.protonnode.service.ProtonService
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/**
 * Single source of UI state. Every blocking call goes through CoreController
 * (PROMPT.md 5.2); this class only keeps Compose state and user intent.
 */
class NodeViewModel(
    private val app: Application,
) : AndroidViewModel(app) {
    private val prefs = Prefs(app)
    private val sessionStore = SessionStore(app)

    private val _status = MutableStateFlow(CoreController.lastStatus)
    val status: StateFlow<StatusSnapshot> = _status.asStateFlow()

    private val _session = MutableStateFlow("{}")
    val session: StateFlow<String> = _session.asStateFlow()

    private val _logs = MutableStateFlow("")
    val logs: StateFlow<String> = _logs.asStateFlow()

    private val _scanProgress = MutableStateFlow("")
    val scanProgress: StateFlow<String> = _scanProgress.asStateFlow()

    private val _scanResults = MutableStateFlow("")
    val scanResults: StateFlow<String> = _scanResults.asStateFlow()

    private val _servers = MutableStateFlow("[]")
    val servers: StateFlow<String> = _servers.asStateFlow()

    private val _themeMode = MutableStateFlow(prefs.themeMode)
    val themeMode: StateFlow<String> = _themeMode.asStateFlow()

    private val _dynamicColor = MutableStateFlow(prefs.dynamicColor)
    val dynamicColor: StateFlow<Boolean> = _dynamicColor.asStateFlow()

    private val _mode = MutableStateFlow(prefs.mode)
    val mode: StateFlow<String> = _mode.asStateFlow()

    private val _autostart = MutableStateFlow(prefs.autoStart)
    val autostart: StateFlow<Boolean> = _autostart.asStateFlow()

    private val _apiDns = MutableStateFlow(prefs.apiDns)
    val apiDns: StateFlow<String> = _apiDns.asStateFlow()

    init {
        viewModelScope.launch {
            CoreController.ensureInit(app)
            refreshStatus()
            refreshSession()
            loadServers()
        }
    }

    fun refreshAll() {
        viewModelScope.launch {
            refreshStatus()
            refreshSession()
        }
    }

    private suspend fun refreshStatus() {
        _status.value = CoreController.status()
    }

    private suspend fun refreshSession() {
        _session.value = CoreController.sessionState()
    }

    fun loadServers() {
        viewModelScope.launch {
            _servers.value = CoreController.listServers("", false)
        }
    }

    fun loadLogs(lines: Int = 300) {
        viewModelScope.launch { _logs.value = CoreController.recentLogs(lines) }
    }

    fun connect() {
        // The service owns the node; the UI only expresses intent (P4).
        ProtonService.start(app, configJson())
        prefs.wantRunning = true
        viewModelScope.launch {
            kotlinx.coroutines.delay(300)
            refreshStatus()
        }
    }

    fun disconnect() {
        ProtonService.stop(app)
        prefs.wantRunning = false
        viewModelScope.launch {
            kotlinx.coroutines.delay(300)
            refreshStatus()
        }
    }

    private fun configJson(): String = "{\"mode\":\"" + prefs.mode + "\",\"socksPort\":1080}"

    fun setThemeMode(value: String) {
        prefs.themeMode = value
        _themeMode.value = value
    }

    fun setDynamicColor(value: Boolean) {
        prefs.dynamicColor = value
        _dynamicColor.value = value
    }

    fun setMode(value: String) {
        prefs.mode = value
        _mode.value = value
    }

    fun setAutostart(value: Boolean) {
        prefs.autoStart = value
        _autostart.value = value
    }

    fun setApiDns(value: String) {
        prefs.apiDns = value
        _apiDns.value = value
        viewModelScope.launch { CoreController.setApiDns(value) }
    }

    fun setSession(json: String): Result<Unit> {
        sessionStore.save(json)
        val result =
            kotlinx.coroutines.runBlocking {
                runCatching { CoreController.setSession(json) }.getOrElse { Result.failure(it) }
            }
        viewModelScope.launch { refreshSession() }
        return result
    }

    private val _loginMessage = MutableStateFlow("")
    val loginMessage: StateFlow<String> = _loginMessage.asStateFlow()

    fun login(
        email: String,
        password: String,
        totp: String,
    ) {
        viewModelScope.launch {
            _loginMessage.value = "…"
            val result = CoreController.login(email.trim(), password, totp.trim())
            if (result.isSuccess) {
                // Persist immediately: the login just rotated the tokens.
                CoreController.persistSession(sessionStore)
                _loginMessage.value = "ok"
                refreshSession()
            } else {
                _loginMessage.value = result.exceptionOrNull()?.message ?: "login failed"
            }
        }
    }

    fun logout() {
        sessionStore.clear()
        viewModelScope.launch {
            runCatching { CoreController.logout() }
            refreshSession()
        }
    }

    fun startScan(scope: String) {
        viewModelScope.launch {
            val id = runCatching { CoreController.startScan(scope, 32) }.getOrNull()
            if (id.isNullOrEmpty()) {
                _scanProgress.value = "{\"state\":\"failed\",\"error\":\"could not start the scan\"}"
                return@launch
            }
            while (true) {
                val progress = CoreController.scanProgress(id)
                _scanProgress.value = progress
                val results = CoreController.scanResults(id)
                _scanResults.value = results
                val state = jsonString(progress, "state")
                if (state != "running") break
                kotlinx.coroutines.delay(500)
            }
        }
    }

    fun cancelScan() {
        viewModelScope.launch {
            val id = jsonString(_scanProgress.value, "scanId")
            if (id.isNotEmpty()) CoreController.cancelScan(id)
        }
    }

    fun lastScanJson(): String {
        var value = "{}"
        kotlinx.coroutines.runBlocking { value = CoreController.lastScanSummary() }
        return value
    }

    fun lastScanCached(): String = _scanResults.value

    companion object {
        /** Minimal JSON string reader so the UI does not depend on a parser library. */
        fun jsonString(
            json: String,
            key: String,
        ): String {
            val needle = "\"" + key + "\":"
            var i = json.indexOf(needle)
            if (i < 0) return ""
            i += needle.length
            while (i < json.length && json[i] == ' ') i++
            if (i >= json.length || json[i] != '"') return ""
            i++
            val sb = StringBuilder()
            while (i < json.length && json[i] != '"') {
                if (json[i] == '\\' && i + 1 < json.length) i++
                sb.append(json[i])
                i++
            }
            return sb.toString()
        }

        fun jsonInt(
            json: String,
            key: String,
        ): Long {
            val needle = "\"" + key + "\":"
            val i = json.indexOf(needle)
            if (i < 0) return 0
            var j = i + needle.length
            val start = j
            while (j < json.length && (json[j].isDigit() || json[j] == '-')) j++
            return json.substring(start, j).toLongOrNull() ?: 0
        }
    }
}
