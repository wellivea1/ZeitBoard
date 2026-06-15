package org.non24.planner

import android.content.Intent
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.core.net.toUri
import androidx.health.connect.client.PermissionController
import androidx.lifecycle.viewmodel.compose.viewModel
import org.non24.planner.ui.AppViewModel
import org.non24.planner.ui.Non24App

class MainActivity : ComponentActivity() {
    private val container by lazy { AppContainer(this) }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent {
            val appViewModel: AppViewModel = viewModel(factory = AppViewModel.factory(container))
            val permissionLauncher = rememberLauncherForActivityResult(
                contract = PermissionController.createRequestPermissionResultContract(),
                onResult = appViewModel::onHealthPermissionResult,
            )
            Non24App(
                viewModel = appViewModel,
                requiredHealthPermissions = container.healthConnectRepository.requiredPermissions,
                onRequestHealthPermissions = permissionLauncher::launch,
                onOpenHealthConnectListing = ::openHealthConnectListing,
            )
        }
    }

    private fun openHealthConnectListing() {
        val packageName = "com.google.android.apps.healthdata"
        val marketIntent = Intent(Intent.ACTION_VIEW, "market://details?id=$packageName".toUri())
        val webIntent = Intent(
            Intent.ACTION_VIEW,
            "https://play.google.com/store/apps/details?id=$packageName".toUri(),
        )
        runCatching { startActivity(marketIntent) }
            .onFailure { startActivity(webIntent) }
    }
}
