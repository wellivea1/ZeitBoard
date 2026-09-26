package org.non24.planner

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.core.content.ContextCompat
import androidx.core.net.toUri
import androidx.health.connect.client.PermissionController
import androidx.lifecycle.viewmodel.compose.viewModel
import org.non24.planner.ui.AppViewModel
import org.non24.planner.ui.Non24App

class MainActivity : ComponentActivity() {
    private val container by lazy { (application as ZeitBoardApplication).container }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent {
            val appViewModel: AppViewModel = viewModel(factory = AppViewModel.factory(container))
            val permissionLauncher = rememberLauncherForActivityResult(
                contract = PermissionController.createRequestPermissionResultContract(),
                onResult = appViewModel::onHealthPermissionResult,
            )
            val backgroundPermissionLauncher = rememberLauncherForActivityResult(
                contract = PermissionController.createRequestPermissionResultContract(),
                onResult = appViewModel::onBackgroundPermissionResult,
            )
            val noticePermissionLauncher = rememberLauncherForActivityResult(
                contract = ActivityResultContracts.RequestPermission(),
            ) { granted ->
                if (granted) appViewModel.setTimeRequestNotices(true) else appViewModel.onNoticePermissionDenied()
            }
            Non24App(
                viewModel = appViewModel,
                requiredHealthPermissions = container.healthConnectRepository.requiredPermissions,
                onRequestHealthPermissions = permissionLauncher::launch,
                onOpenHealthConnectListing = ::openHealthConnectListing,
                onRequestBackgroundPermission = {
                    backgroundPermissionLauncher.launch(container.healthConnectRepository.requiredPermissions +
                        org.non24.planner.data.HealthConnectPermissions.READ_BACKGROUND)
                },
                onEnableNotices = {
                    if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU ||
                        ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED
                    ) {
                        appViewModel.setTimeRequestNotices(true)
                    } else {
                        noticePermissionLauncher.launch(Manifest.permission.POST_NOTIFICATIONS)
                    }
                },
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
