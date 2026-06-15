package org.non24.planner

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import org.non24.planner.ui.Non24Theme

class PermissionsRationaleActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent {
            Non24Theme {
                PermissionsRationale()
            }
        }
    }
}

@Composable
private fun PermissionsRationale() {
    Surface(modifier = Modifier.fillMaxSize()) {
        Column(
            modifier = Modifier.padding(24.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Text("Health Connect privacy", style = MaterialTheme.typography.headlineSmall)
            Text(
                "Non-24 Planner requests read-only access to sleep sessions so you can review " +
                    "imported sleep and add local corrections.",
            )
            Text(
                "Health data remains on this device in phase one. The app has no analytics, " +
                    "tracking SDKs, or health-data upload.",
            )
            Text(
                "Android does not calculate an estimated sleep-wake phase. Any displayed " +
                    "forecast is an explicitly labeled synthetic fixture or a future imported result.",
            )
        }
    }
}
