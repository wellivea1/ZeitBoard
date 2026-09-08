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
                "ZeitBoard requests read-only access to sleep sessions so you can review " +
                    "imported sleep and add local corrections.",
            )
            Text(
                "Health data stays in the app-private database unless you connect to your own server. " +
                    "Connecting uploads Health Connect sleep and provider revisions over TLS. " +
                    "Medication events and manual corrections remain local. There are no analytics or tracking SDKs.",
            )
            Text(
                "Background sleep access is optional and requested separately. With automatic refresh enabled, " +
                    "it lets ZeitBoard import sleep while closed. You can revoke access in Health Connect " +
                    "or turn automatic refresh off in ZeitBoard Settings.",
            )
            Text(
                "Android does not calculate an estimated sleep-wake phase. Any displayed " +
                    "forecast is an explicitly labeled synthetic fixture or a future imported result.",
            )
        }
    }
}
