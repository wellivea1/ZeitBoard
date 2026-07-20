package org.non24.planner.ui

import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Shapes
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

internal val Ink = Color(0xFF24302D)
internal val Muted = Color(0xFF5B655F)
internal val Subtle = Color(0xFF666F6A)
internal val Line = Color(0xFFDEDFD8)
internal val Paper = Color(0xFFFFFEFA)
internal val CanvasColor = Color(0xFFF5F3EE)
internal val PanelAlt = Color(0xFFFAF9F5)
internal val Chrome = Color(0xFFEBEAE4)
internal val Sage = Color(0xFF55766B)
internal val SageDark = Color(0xFF3E5F55)
internal val SageSoft = Color(0xFFE4ECE7)
internal val SleepBlue = Color(0xFF4A5E7A)
internal val BlueSoft = Color(0xFFE9EFF2)
internal val Amber = Color(0xFF9A6B16)
internal val AmberSoft = Color(0xFFF5ECDA)

private val LightColors = lightColorScheme(
    primary = SageDark,
    onPrimary = Color.White,
    primaryContainer = SageSoft,
    onPrimaryContainer = SageDark,
    secondary = SleepBlue,
    onSecondary = Color.White,
    secondaryContainer = BlueSoft,
    onSecondaryContainer = SleepBlue,
    tertiary = Amber,
    tertiaryContainer = AmberSoft,
    onTertiaryContainer = Color(0xFF695528),
    background = CanvasColor,
    onBackground = Ink,
    surface = Paper,
    onSurface = Ink,
    surfaceVariant = PanelAlt,
    onSurfaceVariant = Muted,
    outline = Line,
    outlineVariant = Color(0xFFECECE7),
)

private val DarkColors = darkColorScheme(
    primary = Color(0xFF8AB4A6),
    onPrimary = Color(0xFF0F1412),
    primaryContainer = Color(0xFF253430),
    onPrimaryContainer = Color(0xFF8FCFB5),
    secondary = Color(0xFF7A9ABF),
    onSecondary = Color(0xFF0D1117),
    secondaryContainer = Color(0xFF25333A),
    onSecondaryContainer = Color(0xFFCDD7E4),
    tertiary = Color(0xFFC99A3E),
    tertiaryContainer = Color(0xFF3B3323),
    onTertiaryContainer = Color(0xFFE6C87C),
    background = Color(0xFF161B19),
    onBackground = Color(0xFFE8EAE5),
    surface = Color(0xFF1E2522),
    onSurface = Color(0xFFE8EAE5),
    surfaceVariant = Color(0xFF222A26),
    onSurfaceVariant = Color(0xFFA5ADA7),
    outline = Color(0xFF3B4541),
    outlineVariant = Color(0xFF252D2A),
)

private val ZeitBoardShapes = Shapes(
    extraSmall = RoundedCornerShape(3.dp),
    small = RoundedCornerShape(4.dp),
    medium = RoundedCornerShape(7.dp),
    large = RoundedCornerShape(9.dp),
    extraLarge = RoundedCornerShape(12.dp),
)

private val ZeitBoardTypography = Typography(
    displaySmall = TextStyle(
        fontSize = 30.sp,
        lineHeight = 33.sp,
        fontWeight = FontWeight.SemiBold,
        letterSpacing = (-0.8).sp,
    ),
    headlineLarge = TextStyle(
        fontSize = 27.sp,
        lineHeight = 30.sp,
        fontWeight = FontWeight.SemiBold,
        letterSpacing = (-0.6).sp,
    ),
    headlineMedium = TextStyle(
        fontSize = 22.sp,
        lineHeight = 26.sp,
        fontWeight = FontWeight.SemiBold,
        letterSpacing = (-0.35).sp,
    ),
    titleLarge = TextStyle(
        fontSize = 19.sp,
        lineHeight = 23.sp,
        fontWeight = FontWeight.SemiBold,
        letterSpacing = (-0.2).sp,
    ),
    titleMedium = TextStyle(
        fontSize = 15.sp,
        lineHeight = 19.sp,
        fontWeight = FontWeight.SemiBold,
    ),
    titleSmall = TextStyle(
        fontSize = 13.sp,
        lineHeight = 17.sp,
        fontWeight = FontWeight.SemiBold,
    ),
    bodyLarge = TextStyle(
        fontSize = 14.sp,
        lineHeight = 20.sp,
        fontWeight = FontWeight.Normal,
    ),
    bodyMedium = TextStyle(
        fontSize = 13.sp,
        lineHeight = 18.sp,
        fontWeight = FontWeight.Normal,
    ),
    bodySmall = TextStyle(
        fontSize = 11.sp,
        lineHeight = 16.sp,
        fontWeight = FontWeight.Normal,
    ),
    labelLarge = TextStyle(
        fontSize = 12.sp,
        lineHeight = 15.sp,
        fontWeight = FontWeight.Bold,
    ),
    labelMedium = TextStyle(
        fontSize = 10.sp,
        lineHeight = 13.sp,
        fontWeight = FontWeight.Bold,
        letterSpacing = 0.45.sp,
    ),
    labelSmall = TextStyle(
        fontSize = 9.sp,
        lineHeight = 12.sp,
        fontWeight = FontWeight.Bold,
        letterSpacing = 0.55.sp,
    ),
)

@Composable
fun Non24Theme(
    darkTheme: Boolean = false,
    content: @Composable () -> Unit,
) {
    MaterialTheme(
        colorScheme = if (darkTheme) DarkColors else LightColors,
        typography = ZeitBoardTypography,
        shapes = ZeitBoardShapes,
        content = content,
    )
}
