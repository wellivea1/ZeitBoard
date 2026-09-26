package org.non24.planner

import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.luminance
import org.junit.Assert.assertTrue
import org.junit.Test
import org.non24.planner.ui.AlmanacPalette
import org.non24.planner.ui.InkPalette
import org.non24.planner.ui.PaperPalette

class AlmanacPaletteTest {
    private fun contrast(a: Color, b: Color): Double {
        val lighter = maxOf(a.luminance(), b.luminance()) + 0.05
        val darker = minOf(a.luminance(), b.luminance()) + 0.05
        return lighter / darker.toDouble()
    }

    private fun assertContrast(name: String, palette: AlmanacPalette, fg: Color, bg: Color, minimum: Double) {
        val ratio = contrast(fg, bg)
        assertTrue("$name ${"%.2f".format(ratio)}:1 is under $minimum:1", ratio >= minimum)
    }

    @Test
    fun textIsLegibleOnThePageInBothThemes() {
        for ((theme, palette) in listOf("Paper" to PaperPalette, "Ink" to InkPalette)) {
            for ((surface, background) in listOf("paper" to palette.paper, "canvas" to palette.canvas)) {
                // WCAG AA for body text; the muted and subtle greys carry dates and hints.
                assertContrast("$theme ink on $surface", palette, palette.ink, background, 4.5)
                assertContrast("$theme muted on $surface", palette, palette.muted, background, 4.5)
                assertContrast("$theme subtle on $surface", palette, palette.subtle, background, 4.5)
                assertContrast("$theme accent on $surface", palette, palette.accent, background, 4.5)
                assertContrast("$theme danger on $surface", palette, palette.danger, background, 4.5)
            }
        }
    }

    @Test
    fun theDialMarksStandOutInBothThemes() {
        for ((theme, palette) in listOf("Paper" to PaperPalette, "Ink" to InkPalette)) {
            // WCAG 1.4.11: graphics that carry meaning need 3:1 against their ground.
            assertContrast("$theme asleep", palette, palette.sleepBlue, palette.paper, 3.0)
            assertContrast("$theme uncertain", palette, palette.uncertainFill, palette.paper, 3.0)
            assertContrast("$theme now hand", palette, palette.accent, palette.paper, 3.0)
        }
    }
}
