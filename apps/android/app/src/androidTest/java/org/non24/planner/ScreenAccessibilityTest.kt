package org.non24.planner

import android.graphics.Rect
import android.os.Build
import android.os.SystemClock
import android.util.Log
import android.view.accessibility.AccessibilityNodeInfo
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.filters.SdkSuppress
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/**
 * TalkBack reads the accessibility tree. The test emulator has no TalkBack, so
 * this walks that tree itself on each destination, as the desktop's screens
 * were swept: every screen has a heading to move to, and everything that can
 * be tapped or switched says what it is and is large enough to touch.
 *
 * It needs Android 14: without a screen reader on, Compose sends no change
 * events, so the tree must be read afresh each time (UiAutomation.clearCache).
 */
@RunWith(AndroidJUnit4::class)
@SdkSuppress(minSdkVersion = Build.VERSION_CODES.UPSIDE_DOWN_CAKE)
class ScreenAccessibilityTest {
    private val automation = InstrumentationRegistry.getInstrumentation().uiAutomation
    private val density =
        InstrumentationRegistry.getInstrumentation().targetContext.resources.displayMetrics.density

    @Test
    fun everyDestinationHasHeadingsAndNamedControlsLargeEnoughToTouch() {
        ActivityScenario.launch(MainActivity::class.java).use {
            for (destination in listOf("Now", "Plan", "Log", "Doses", "Settings")) {
                val headings = linkedSetOf<String>()
                val nameless = linkedSetOf<String>()
                // The largest each control was seen: one cut by a page's edge
                // is whole on the next.
                val targets = mutableMapOf<String, Pair<Float, Float>>()
                var nodes = open(destination)
                var pages = 1
                // The whole screen, a page at a time, as TalkBack scrolls it:
                // what is scrolled out of view is not in the tree.
                while (true) {
                    headings += headings(nodes)
                    for (control in nodes.filter { it.isClickable || it.isCheckable }) {
                        val bounds = Rect().also(control::getBoundsInScreen)
                        val name = spoken(control)
                        if (name.isBlank()) nameless += "${control.className} at ${bounds.toShortString()}"
                        if (!control.isVisibleToUser) continue
                        val key = "${control.className} $name"
                        val (width, height) = targets[key] ?: (0f to 0f)
                        targets[key] = maxOf(width, bounds.width() / density) to maxOf(height, bounds.height() / density)
                    }
                    val scroller = nodes.firstOrNull { scrollForward in it.actionList } ?: break
                    if (pages == MAX_PAGES || !scroller.performAction(scrollForward.id)) break
                    nodes = settled()
                    pages++
                }
                val smallest = targets.minByOrNull { (_, size) -> minOf(size.first, size.second) }
                Log.i(TAG, "$destination, $pages pages, headings: $headings, smallest target: $smallest")
                assertTrue("$destination has no heading", headings.isNotEmpty())
                assertEquals("$destination has controls with no name", emptyList<String>(), nameless.toList())
                // The design's smallest target (ui-ux-design.md, "Touch targets").
                val small = targets
                    .filter { (_, size) -> size.first < MIN_TARGET_DP || size.second < MIN_TARGET_DP }
                    .map { (control, size) -> "$control ${size.first.toInt()}x${size.second.toInt()}dp" }
                assertEquals("$destination has targets under ${MIN_TARGET_DP}dp", emptyList<String>(), small)
            }
        }
    }

    // Opens a destination from the bar along the bottom, with the action
    // TalkBack's double tap sends, and reads it once it has settled. The one
    // shown is selected, not clickable.
    private fun open(label: String): List<AccessibilityNodeInfo> {
        val shown = settled()
        val tab = shown.lastOrNull {
            (it.isClickable || it.isSelected) && spoken(it).equals(label, ignoreCase = true)
        } ?: error("No destination named $label")
        if (tab.isSelected) return shown
        assertTrue("$label did not open", tab.performAction(AccessibilityNodeInfo.ACTION_CLICK))
        return settled(changedFrom = headings(shown))
    }

    // Compose sends no events to wait on, and a destination fades in over the
    // one it leaves: read until the headings have changed and the tree has held
    // still for a second.
    private fun settled(changedFrom: List<String>? = null): List<AccessibilityNodeInfo> {
        val deadline = SystemClock.uptimeMillis() + 15_000
        var last: List<String>? = null
        var stillSince = 0L
        while (SystemClock.uptimeMillis() < deadline) {
            val nodes = nodes()
            val snapshot = nodes.map { "${it.className}|${it.text}|${it.contentDescription}|${it.isSelected}" }
            val now = SystemClock.uptimeMillis()
            if (snapshot != last) {
                last = snapshot
                stillSince = now
            } else if (now - stillSince >= 1_000 && headings(nodes) != changedFrom) {
                return nodes
            }
            SystemClock.sleep(200)
        }
        error("The screen did not settle")
    }

    private fun nodes(): List<AccessibilityNodeInfo> {
        automation.clearCache()
        val all = mutableListOf<AccessibilityNodeInfo>()
        fun walk(node: AccessibilityNodeInfo) {
            all += node
            for (index in 0 until node.childCount) node.getChild(index)?.let(::walk)
        }
        walk(automation.rootInActiveWindow ?: error("No window to read"))
        return all
    }

    private fun headings(nodes: List<AccessibilityNodeInfo>) = nodes.filter { it.isHeading }.map(::spoken)

    // What a screen reader says for a node: its description, its text or its
    // hint, and otherwise what its children say.
    private fun spoken(node: AccessibilityNodeInfo): String {
        val own = listOf(node.contentDescription, node.text, node.hintText)
            .firstOrNull { !it.isNullOrBlank() }
        if (own != null) return own.toString()
        return (0 until node.childCount)
            .mapNotNull(node::getChild)
            .joinToString(" ", transform = ::spoken)
            .trim()
    }

    private companion object {
        const val TAG = "ScreenAccessibility"
        const val MAX_PAGES = 12
        const val MIN_TARGET_DP = 44f
        val scrollForward: AccessibilityNodeInfo.AccessibilityAction =
            AccessibilityNodeInfo.AccessibilityAction.ACTION_SCROLL_FORWARD
    }
}
