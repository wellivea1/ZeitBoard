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
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

// Almanac (ui-refactor-plan.md §14), the desktop's paper and ink: a serif for
// reading (Noto Serif, the system's), the system sans for controls, square
// corners, and colour kept for what the rhythm means. The one bright colour,
// the accent, marks now.

internal val Ink = Color(0xFF221F1A)
internal val Muted = Color(0xFF5C564C)
internal val Subtle = Color(0xFF645E53)
internal val Line = Color(0xFFC4BBA8)
internal val Divider = Color(0xFFCFC7B6)
internal val Paper = Color(0xFFF7F4ED)
internal val CanvasColor = Color(0xFFF2EEE5)
internal val PanelAlt = Color(0xFFEBE6DA)
internal val Chrome = CanvasColor
internal val Accent = Color(0xFFB3401F)
internal val DangerText = Color(0xFF8F2D1A)

// The "sage" names predate the redesign and now mean the action and focus
// colours, as they do on the desktop.
internal val Sage = Color(0xFF2B3D5E)
internal val SageDark = Ink
internal val SageSoft = Color(0xFFE8E2D4)
internal val SleepBlue = Color(0xFF2B3D5E)
internal val BlueSoft = Color(0xFFE3E5EA)
internal val Amber = Color(0xFF9A6A1F)
internal val AmberSoft = Color(0xFFEFE3CC)
internal val UncertainFill = Color(0xFFB17A2E)

private val LightColors = lightColorScheme(
    primary = Ink,
    onPrimary = Paper,
    primaryContainer = SageSoft,
    onPrimaryContainer = Ink,
    secondary = SleepBlue,
    onSecondary = Paper,
    secondaryContainer = BlueSoft,
    onSecondaryContainer = SleepBlue,
    tertiary = Amber,
    tertiaryContainer = AmberSoft,
    onTertiaryContainer = Color(0xFF5C3F12),
    background = CanvasColor,
    onBackground = Ink,
    surface = Paper,
    onSurface = Ink,
    surfaceVariant = PanelAlt,
    onSurfaceVariant = Muted,
    outline = Line,
    outlineVariant = Divider,
    error = DangerText,
)

// Ink: the same page at night, cream on a blue-black.
private val DarkColors = darkColorScheme(
    primary = Color(0xFFEBE5D6),
    onPrimary = Color(0xFF151619),
    primaryContainer = Color(0xFF262831),
    onPrimaryContainer = Color(0xFFEBE5D6),
    secondary = Color(0xFF8EA4D8),
    onSecondary = Color(0xFF10131A),
    secondaryContainer = Color(0xFF1F2533),
    onSecondaryContainer = Color(0xFFCFD8EA),
    tertiary = Color(0xFFD0A04C),
    tertiaryContainer = Color(0xFF2B2518),
    onTertiaryContainer = Color(0xFFE6C887),
    background = Color(0xFF151619),
    onBackground = Color(0xFFEBE5D6),
    surface = Color(0xFF1C1D21),
    onSurface = Color(0xFFEBE5D6),
    surfaceVariant = Color(0xFF212226),
    onSurfaceVariant = Color(0xFFABA391),
    outline = Color(0xFF4A463F),
    outlineVariant = Color(0xFF403D37),
    error = Color(0xFFEAB0A2),
)

// Square, like the desktop. Paper does not have rounded corners.
private val ZeitBoardShapes = Shapes(
    extraSmall = RoundedCornerShape(0.dp),
    small = RoundedCornerShape(0.dp),
    medium = RoundedCornerShape(0.dp),
    large = RoundedCornerShape(0.dp),
    extraLarge = RoundedCornerShape(0.dp),
)

private val Serif = FontFamily.Serif
private val Sans = FontFamily.Default

private val ZeitBoardTypography = Typography(
    displaySmall = TextStyle(
        fontFamily = Serif,
        fontSize = 30.sp,
        lineHeight = 36.sp,
        fontWeight = FontWeight.Normal,
        letterSpacing = (-0.3).sp,
    ),
    headlineLarge = TextStyle(
        fontFamily = Serif,
        fontSize = 28.sp,
        lineHeight = 34.sp,
        fontWeight = FontWeight.Normal,
        letterSpacing = (-0.3).sp,
    ),
    headlineMedium = TextStyle(
        fontFamily = Serif,
        fontSize = 22.sp,
        lineHeight = 28.sp,
        fontWeight = FontWeight.Normal,
    ),
    headlineSmall = TextStyle(
        fontFamily = Serif,
        fontSize = 20.sp,
        lineHeight = 27.sp,
        fontWeight = FontWeight.Normal,
    ),
    titleLarge = TextStyle(
        fontFamily = Serif,
        fontSize = 20.sp,
        lineHeight = 25.sp,
        fontWeight = FontWeight.Medium,
    ),
    titleMedium = TextStyle(
        fontFamily = Serif,
        fontSize = 17.sp,
        lineHeight = 22.sp,
        fontWeight = FontWeight.Medium,
    ),
    titleSmall = TextStyle(
        fontFamily = Serif,
        fontSize = 15.sp,
        lineHeight = 20.sp,
        fontWeight = FontWeight.Medium,
    ),
    bodyLarge = TextStyle(
        fontFamily = Sans,
        fontSize = 15.sp,
        lineHeight = 21.sp,
        fontWeight = FontWeight.Normal,
    ),
    bodyMedium = TextStyle(
        fontFamily = Sans,
        fontSize = 14.sp,
        lineHeight = 20.sp,
        fontWeight = FontWeight.Normal,
    ),
    bodySmall = TextStyle(
        fontFamily = Sans,
        fontSize = 12.sp,
        lineHeight = 17.sp,
        fontWeight = FontWeight.Normal,
    ),
    // Buttons.
    labelLarge = TextStyle(
        fontFamily = Sans,
        fontSize = 13.sp,
        lineHeight = 16.sp,
        fontWeight = FontWeight.SemiBold,
    ),
    // Small capitals: section titles, field labels, the tabs.
    labelMedium = TextStyle(
        fontFamily = Sans,
        fontSize = 11.sp,
        lineHeight = 14.sp,
        fontWeight = FontWeight.Bold,
        letterSpacing = 1.1.sp,
    ),
    labelSmall = TextStyle(
        fontFamily = Sans,
        fontSize = 10.sp,
        lineHeight = 13.sp,
        fontWeight = FontWeight.Bold,
        letterSpacing = 1.sp,
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
