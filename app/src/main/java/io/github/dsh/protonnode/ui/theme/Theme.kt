package io.github.dsh.protonnode.ui.theme

import android.os.Build
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.dynamicDarkColorScheme
import androidx.compose.material3.dynamicLightColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext

/** Theme preference stored in [io.github.dsh.protonnode.data.Prefs]. */
enum class ThemeMode { SYSTEM, LIGHT, DARK }

private val FallbackLight =
    lightColorScheme(
        primary = Color(0xFF3F5F90),
        secondary = Color(0xFF565E71),
        tertiary = Color(0xFF6F5575),
    )

private val FallbackDark =
    darkColorScheme(
        primary = Color(0xFFA8C7FA),
        secondary = Color(0xFFBEC6DC),
        tertiary = Color(0xFFDDBCE1),
    )

@Composable
fun ProtonTheme(
    mode: ThemeMode = ThemeMode.SYSTEM,
    dynamicColor: Boolean = true,
    content: @Composable () -> Unit,
) {
    val dark =
        when (mode) {
            ThemeMode.SYSTEM -> isSystemInDarkTheme()
            ThemeMode.LIGHT -> false
            ThemeMode.DARK -> true
        }
    val context = LocalContext.current
    val scheme =
        when {
            dynamicColor && Build.VERSION.SDK_INT >= Build.VERSION_CODES.S ->
                if (dark) dynamicDarkColorScheme(context) else dynamicLightColorScheme(context)
            dark -> FallbackDark
            else -> FallbackLight
        }
    MaterialTheme(colorScheme = scheme, content = content)
}
