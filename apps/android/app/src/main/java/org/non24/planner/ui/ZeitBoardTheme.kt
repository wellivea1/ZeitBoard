// Variable-font settings on a resource font are still marked experimental.
@file:OptIn(ExperimentalTextApi::class)

package org.non24.planner.ui

import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Shapes
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.ExperimentalTextApi
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontVariation
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import org.non24.planner.R

// Almanac (ui-refactor-plan.md §14), the desktop's paper and ink: Newsreader
// for reading and Public Sans for controls, both bundled (res/font, under the
// SIL Open Font License in assets/licenses), square corners, and colour kept
// for what the rhythm means. The one bright colour, the accent, marks now.

/** The page's colours in one theme; Paper by day and Ink at night, as on the desktop. */
@Immutable
internal data class AlmanacPalette(
    val ink: Color,
    val muted: Color,
    val subtle: Color,
    val line: Color,
    val divider: Color,
    val paper: Color,
    val canvas: Color,
    val panelAlt: Color,
    val accent: Color,
    val danger: Color,
    val sage: Color,
    val sageSoft: Color,
    val sleepBlue: Color,
    val blueSoft: Color,
    val amber: Color,
    val amberSoft: Color,
    val uncertainFill: Color,
)

internal val PaperPalette = AlmanacPalette(
    ink = Color(0xFF221F1A),
    muted = Color(0xFF5C564C),
    subtle = Color(0xFF645E53),
    line = Color(0xFFC4BBA8),
    divider = Color(0xFFCFC7B6),
    paper = Color(0xFFF7F4ED),
    canvas = Color(0xFFF2EEE5),
    panelAlt = Color(0xFFEBE6DA),
    accent = Color(0xFFB3401F),
    danger = Color(0xFF8F2D1A),
    sage = Color(0xFF2B3D5E),
    sageSoft = Color(0xFFE8E2D4),
    sleepBlue = Color(0xFF2B3D5E),
    blueSoft = Color(0xFFE3E5EA),
    amber = Color(0xFF9A6A1F),
    amberSoft = Color(0xFFEFE3CC),
    uncertainFill = Color(0xFFB17A2E),
)

// Ink: the same page at night, cream on a blue-black. The desktop's Ink theme.
internal val InkPalette = AlmanacPalette(
    ink = Color(0xFFEBE5D6),
    muted = Color(0xFFABA391),
    subtle = Color(0xFF9F9887),
    line = Color(0xFF4A463F),
    divider = Color(0xFF403D37),
    paper = Color(0xFF1C1D21),
    canvas = Color(0xFF151619),
    panelAlt = Color(0xFF212226),
    accent = Color(0xFFE27D5D),
    danger = Color(0xFFEAB0A2),
    sage = Color(0xFF8EA4D8),
    sageSoft = Color(0xFF262831),
    sleepBlue = Color(0xFF8EA4D8),
    blueSoft = Color(0xFF1F2533),
    amber = Color(0xFFD0A04C),
    amberSoft = Color(0xFF2B2518),
    uncertainFill = Color(0xFFD0A04C),
)

internal val LocalAlmanacPalette = staticCompositionLocalOf { PaperPalette }

// The names the screens use. Each reads the palette of the theme in force, so
// a screen follows the system into Ink without knowing it. Inside a drawing
// lambda, which is not composable, read them first and draw with the value.
internal val Ink: Color @Composable @ReadOnlyComposable get() = LocalAlmanacPalette.current.ink
internal val Muted: Color @Composable @ReadOnlyComposable get() = LocalAlmanacPalette.current.muted
internal val Subtle: Color @Composable @ReadOnlyComposable get() = LocalAlmanacPalette.current.subtle
internal val Line: Color @Composable @ReadOnlyComposable get() = LocalAlmanacPalette.current.line
internal val Divider: Color @Composable @ReadOnlyComposable get() = LocalAlmanacPalette.current.divider
internal val Paper: Color @Composable @ReadOnlyComposable get() = LocalAlmanacPalette.current.paper
internal val CanvasColor: Color @Composable @ReadOnlyComposable get() = LocalAlmanacPalette.current.canvas
internal val PanelAlt: Color @Composable @ReadOnlyComposable get() = LocalAlmanacPalette.current.panelAlt
internal val Chrome: Color @Composable @ReadOnlyComposable get() = LocalAlmanacPalette.current.canvas
internal val Accent: Color @Composable @ReadOnlyComposable get() = LocalAlmanacPalette.current.accent
internal val DangerText: Color @Composable @ReadOnlyComposable get() = LocalAlmanacPalette.current.danger

// The "sage" names predate the redesign and now mean the action and focus
// colours, as they do on the desktop.
internal val Sage: Color @Composable @ReadOnlyComposable get() = LocalAlmanacPalette.current.sage
internal val SageDark: Color @Composable @ReadOnlyComposable get() = LocalAlmanacPalette.current.ink
internal val SageSoft: Color @Composable @ReadOnlyComposable get() = LocalAlmanacPalette.current.sageSoft
internal val SleepBlue: Color @Composable @ReadOnlyComposable get() = LocalAlmanacPalette.current.sleepBlue
internal val BlueSoft: Color @Composable @ReadOnlyComposable get() = LocalAlmanacPalette.current.blueSoft
internal val Amber: Color @Composable @ReadOnlyComposable get() = LocalAlmanacPalette.current.amber
internal val AmberSoft: Color @Composable @ReadOnlyComposable get() = LocalAlmanacPalette.current.amberSoft
internal val UncertainFill: Color @Composable @ReadOnlyComposable get() = LocalAlmanacPalette.current.uncertainFill

private fun colorsFor(palette: AlmanacPalette, dark: Boolean) =
    if (dark) {
        darkColorScheme(
            primary = palette.ink,
            onPrimary = palette.canvas,
            primaryContainer = palette.sageSoft,
            onPrimaryContainer = palette.ink,
            secondary = palette.sleepBlue,
            onSecondary = Color(0xFF10131A),
            secondaryContainer = palette.blueSoft,
            onSecondaryContainer = Color(0xFFCFD8EA),
            tertiary = palette.amber,
            tertiaryContainer = palette.amberSoft,
            onTertiaryContainer = Color(0xFFE6C887),
            background = palette.canvas,
            onBackground = palette.ink,
            surface = palette.paper,
            onSurface = palette.ink,
            surfaceVariant = palette.panelAlt,
            onSurfaceVariant = palette.muted,
            outline = palette.line,
            outlineVariant = palette.divider,
            error = palette.danger,
        )
    } else {
        lightColorScheme(
            primary = palette.ink,
            onPrimary = palette.paper,
            primaryContainer = palette.sageSoft,
            onPrimaryContainer = palette.ink,
            secondary = palette.sleepBlue,
            onSecondary = palette.paper,
            secondaryContainer = palette.blueSoft,
            onSecondaryContainer = palette.sleepBlue,
            tertiary = palette.amber,
            tertiaryContainer = palette.amberSoft,
            onTertiaryContainer = Color(0xFF5C3F12),
            background = palette.canvas,
            onBackground = palette.ink,
            surface = palette.paper,
            onSurface = palette.ink,
            surfaceVariant = palette.panelAlt,
            onSurfaceVariant = palette.muted,
            outline = palette.line,
            outlineVariant = palette.divider,
            error = palette.danger,
        )
    }

private val LightColors = colorsFor(PaperPalette, dark = false)
private val DarkColors = colorsFor(InkPalette, dark = true)

// Square, like the desktop. Paper does not have rounded corners.
private val ZeitBoardShapes = Shapes(
    extraSmall = RoundedCornerShape(0.dp),
    small = RoundedCornerShape(0.dp),
    medium = RoundedCornerShape(0.dp),
    large = RoundedCornerShape(0.dp),
    extraLarge = RoundedCornerShape(0.dp),
)

// Both faces are variable. Android does not pick an optical size from the text
// size as a browser does, so the display styles ask for Newsreader's display
// cut and the rest for its text cut.
private fun newsreader(opticalSize: Float) = FontFamily(
    listOf(400, 500, 600, 700).flatMap { weight ->
        listOf(
            Font(
                R.font.newsreader,
                FontWeight(weight),
                FontStyle.Normal,
                variationSettings = FontVariation.Settings(
                    FontVariation.weight(weight),
                    FontVariation.Setting("opsz", opticalSize),
                ),
            ),
            Font(
                R.font.newsreader_italic,
                FontWeight(weight),
                FontStyle.Italic,
                variationSettings = FontVariation.Settings(
                    FontVariation.weight(weight),
                    FontVariation.Setting("opsz", opticalSize),
                ),
            ),
        )
    },
)

private val Serif = newsreader(opticalSize = 16f)
private val SerifDisplay = newsreader(opticalSize = 36f)

internal val Sans = FontFamily(
    listOf(400, 500, 600, 700).flatMap { weight ->
        listOf(
            Font(
                R.font.public_sans,
                FontWeight(weight),
                FontStyle.Normal,
                variationSettings = FontVariation.Settings(FontVariation.weight(weight)),
            ),
            Font(
                R.font.public_sans_italic,
                FontWeight(weight),
                FontStyle.Italic,
                variationSettings = FontVariation.Settings(FontVariation.weight(weight)),
            ),
        )
    },
)

/** The reading face, for the few places that set it outside the type scale. */
internal val ReadingSerif = Serif

private val ZeitBoardTypography = Typography(
    displaySmall = TextStyle(
        fontFamily = SerifDisplay,
        fontSize = 30.sp,
        lineHeight = 36.sp,
        fontWeight = FontWeight.Normal,
        letterSpacing = (-0.3).sp,
    ),
    headlineLarge = TextStyle(
        fontFamily = SerifDisplay,
        fontSize = 28.sp,
        lineHeight = 34.sp,
        fontWeight = FontWeight.Normal,
        letterSpacing = (-0.3).sp,
    ),
    headlineMedium = TextStyle(
        fontFamily = SerifDisplay,
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

/** Paper by day; Ink when the system is dark, as the desktop's Auto theme does. */
@Composable
fun Non24Theme(
    darkTheme: Boolean = isSystemInDarkTheme(),
    content: @Composable () -> Unit,
) {
    CompositionLocalProvider(LocalAlmanacPalette provides if (darkTheme) InkPalette else PaperPalette) {
        MaterialTheme(
            colorScheme = if (darkTheme) DarkColors else LightColors,
            typography = ZeitBoardTypography,
            shapes = ZeitBoardShapes,
            content = content,
        )
    }
}
