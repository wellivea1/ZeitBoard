package org.non24.planner.ui

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsFocusedAsState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Snackbar
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.drawscope.rotate
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.NavGraph.Companion.findStartDestination
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import java.time.Instant
import java.time.ZoneId
import java.time.ZoneOffset
import java.time.format.DateTimeFormatter
import java.util.Locale
import org.non24.planner.domain.Confidence
import org.non24.planner.data.DurableLocalDataState
import org.non24.planner.domain.DataMode
import org.non24.planner.domain.EffectiveSleepEpisode
import org.non24.planner.domain.HealthConnectAvailability
import org.non24.planner.domain.HealthPermissionState
import org.non24.planner.domain.MedicationEvent
import org.non24.planner.domain.TimeWindow
import org.non24.planner.domain.resolveTemporalZone

private enum class Destination(
    val route: String,
    val label: String,
    val icon: DestinationIcon,
) {
    STATUS("status", "Status", DestinationIcon.STATUS),
    CORRECT("correct", "Correct", DestinationIcon.CORRECT),
    MEDICATION("medication", "Medication", DestinationIcon.MEDICATION),
    SETTINGS("settings", "Settings", DestinationIcon.SETTINGS),
}

private enum class DestinationIcon {
    STATUS,
    CORRECT,
    MEDICATION,
    SETTINGS,
}

@Composable
fun Non24App(
    viewModel: AppViewModel,
    requiredHealthPermissions: Set<String>,
    onRequestHealthPermissions: (Set<String>) -> Unit,
    onOpenHealthConnectListing: () -> Unit,
) {
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()
    val message by viewModel.message.collectAsStateWithLifecycle()
    val medicationSaveState by viewModel.medicationSaveState.collectAsStateWithLifecycle()
    val navController = rememberNavController()
    val backStackEntry by navController.currentBackStackEntryAsState()
    val currentRoute = backStackEntry?.destination?.route ?: Destination.STATUS.route
    val snackbarHostState = remember { SnackbarHostState() }

    LaunchedEffect(message) {
        message?.let {
            snackbarHostState.showSnackbar(it)
            viewModel.clearMessage()
        }
    }

    Non24Theme {
        Scaffold(
            containerColor = MaterialTheme.colorScheme.background,
            topBar = {
                Column {
                    BrandBar()
                    if (uiState.settings.dataMode == DataMode.FIXTURE) {
                        FixtureBanner()
                    }
                }
            },
            snackbarHost = {
                SnackbarHost(snackbarHostState) { data ->
                    Snackbar(
                        snackbarData = data,
                        containerColor = Ink,
                        contentColor = Paper,
                        shape = MaterialTheme.shapes.medium,
                    )
                }
            },
            bottomBar = {
                DestinationBar(
                    currentRoute = currentRoute,
                    onDestinationSelected = { destination ->
                        navController.navigate(destination.route) {
                            popUpTo(navController.graph.findStartDestination().id) {
                                saveState = true
                            }
                            launchSingleTop = true
                            restoreState = true
                        }
                    },
                )
            },
        ) { padding ->
            NavHost(
                navController = navController,
                startDestination = Destination.STATUS.route,
                modifier = Modifier.padding(padding),
            ) {
                composable(Destination.STATUS.route) {
                    StatusScreen(
                        state = uiState,
                        onRetryLocalData = viewModel::retryLocalData,
                        onUseHealthConnect = { viewModel.setDataMode(DataMode.HEALTH_CONNECT) },
                        onRequestPermission = { onRequestHealthPermissions(requiredHealthPermissions) },
                        onRefreshHealth = viewModel::refreshHealthConnect,
                        onOpenHealthConnectListing = onOpenHealthConnectListing,
                    )
                }
                composable(Destination.CORRECT.route) {
                    CorrectionScreen(
                        state = uiState,
                        onRetryLocalData = viewModel::retryLocalData,
                        onSave = viewModel::saveLatestSleepCorrection,
                    )
                }
                composable(Destination.MEDICATION.route) {
                    MedicationScreen(
                        state = uiState,
                        saveState = medicationSaveState,
                        onRetryLocalData = viewModel::retryLocalData,
                        onSave = viewModel::addMedicationEvent,
                        onSaveResultConsumed = viewModel::consumeMedicationSaveResult,
                    )
                }
                composable(Destination.SETTINGS.route) {
                    SettingsScreen(
                        state = uiState,
                        onDataModeChanged = viewModel::setDataMode,
                        onUse24HourChanged = viewModel::setUse24HourTime,
                    )
                }
            }
        }
    }
}

@Composable
private fun StatusScreen(
    state: AppUiState,
    onRetryLocalData: () -> Unit,
    onUseHealthConnect: () -> Unit,
    onRequestPermission: () -> Unit,
    onRefreshHealth: () -> Unit,
    onOpenHealthConnectListing: () -> Unit,
) {
    ScreenColumn {
        ScreenHeader(
            kicker = "Today",
            title = "Status",
            description = "Estimated sleep timing and the latest observation on this device.",
        )

        DurableLocalDataNotice(state.localDataState, onRetryLocalData)

        if (state.localDataState != DurableLocalDataState.Loading) {
            if (state.estimate != null) {
                StatusEstimatePanel(state)
            } else if (state.localDataState == DurableLocalDataState.Ready) {
                EmptyEstimatePanel()
            }

            if (
                state.latestSleepEpisode != null ||
                state.localDataState == DurableLocalDataState.Ready
            ) {
                LatestSleepPanel(
                    episode = state.latestSleepEpisode,
                    use24HourTime = state.settings.use24HourTime,
                )
                if (
                    state.correctionReviews.any {
                        it.currentEpisodeId == state.latestSleepEpisode?.source?.id
                    }
                ) {
                    InfoStrip("A correction for an earlier source revision requires review.")
                }
            }
        }

        SectionHeading("Health Connect")
        HealthConnectPanel(
            state = state,
            onUseHealthConnect = onUseHealthConnect,
            onRequestPermission = onRequestPermission,
            onRefreshHealth = onRefreshHealth,
            onOpenHealthConnectListing = onOpenHealthConnectListing,
        )

        Text(
            "Not a medical device. Activity does not identify exact circadian phase or DLMO.",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
private fun StatusEstimatePanel(state: AppUiState) {
    val estimate = requireNotNull(state.estimate)
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .background(
                Brush.linearGradient(
                    colors = listOf(Color(0xFF31564F), Color(0xFF5A776E)),
                ),
            )
            .padding(horizontal = 16.dp, vertical = 14.dp),
        verticalArrangement = Arrangement.spacedBy(11.dp),
    ) {
        Text(
            estimate.label.uppercase(Locale.ROOT),
            style = MaterialTheme.typography.labelSmall,
            color = Color(0xFFDDE9E4),
        )
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween,
        ) {
            Text(
                "Forecast available",
                style = MaterialTheme.typography.headlineMedium,
                color = Color.White,
            )
            ConfidenceBadge(estimate.confidence)
        }

        HorizontalDivider(color = Color.White.copy(alpha = 0.22f))
        ForecastRow(
            label = "Predicted sleep window",
            window = estimate.predictedSleepWindow,
            use24HourTime = state.settings.use24HourTime,
        )
        ForecastRow(
            label = "Predicted waking window",
            window = estimate.predictedWakingWindow,
            use24HourTime = state.settings.use24HourTime,
        )
        HorizontalDivider(color = Color.White.copy(alpha = 0.22f))

        estimate.confidenceReasons.forEach { reason ->
            Row(
                verticalAlignment = Alignment.Top,
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                Box(
                    modifier = Modifier
                        .padding(top = 5.dp)
                        .size(4.dp)
                        .background(Color(0xFFDDE9E4), CircleShape),
                )
                Text(
                    reason,
                    modifier = Modifier.weight(1f),
                    style = MaterialTheme.typography.bodySmall,
                    color = Color.White.copy(alpha = 0.9f),
                )
            }
        }
        Text(
            "Snapshot / ${estimate.algorithmVersion}",
            style = MaterialTheme.typography.labelSmall,
            color = Color.White.copy(alpha = 0.62f),
        )
    }
}

@Composable
private fun EmptyEstimatePanel() {
    RuledSection {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(9.dp),
        ) {
            StatusDot(color = Amber)
            Text("Estimate unavailable", style = MaterialTheme.typography.titleMedium)
        }
        Text(
            "Health Connect imports sleep sessions only. Predicted windows require an " +
                "estimate supplied through the shared repository contract.",
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
private fun ForecastRow(
    label: String,
    window: TimeWindow,
    use24HourTime: Boolean,
) {
    Column(verticalArrangement = Arrangement.spacedBy(3.dp)) {
        Text(
            label.uppercase(Locale.ROOT),
            style = MaterialTheme.typography.labelSmall,
            color = Color(0xFFDDE9E4),
        )
        Text(
            formatWindow(window, use24HourTime),
            style = MaterialTheme.typography.titleMedium,
            color = Color.White,
        )
    }
}

@Composable
private fun HealthConnectPanel(
    state: AppUiState,
    onUseHealthConnect: () -> Unit,
    onRequestPermission: () -> Unit,
    onRefreshHealth: () -> Unit,
    onOpenHealthConnectListing: () -> Unit,
) {
    RuledSection {
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween,
        ) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(9.dp),
            ) {
                StatusDot(
                    color = if (state.healthAvailability == HealthConnectAvailability.AVAILABLE) {
                        Sage
                    } else {
                        Amber
                    },
                )
                Text(
                    when (state.healthAvailability) {
                        HealthConnectAvailability.AVAILABLE -> "Available on this device"
                        HealthConnectAvailability.UPDATE_REQUIRED -> "Provider update required"
                        HealthConnectAvailability.UNAVAILABLE -> "Unavailable on this device"
                    },
                    style = MaterialTheme.typography.titleMedium,
                )
            }
            Text(
                "READ SLEEP",
                style = MaterialTheme.typography.labelSmall,
                color = SageDark,
            )
        }

        Text(
            "Requested access is limited to sleep sessions.",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )

        state.healthRefreshError?.let { error ->
            Text(
                error,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.error,
            )
        }

        when (state.healthAvailability) {
            HealthConnectAvailability.AVAILABLE -> {
                HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
                DataRow(
                    label = "Permission",
                    value = when (state.healthPermissionState) {
                        HealthPermissionState.GRANTED -> "Granted"
                        HealthPermissionState.REQUIRED -> "Required"
                        HealthPermissionState.UNKNOWN -> "Checking"
                        HealthPermissionState.UNAVAILABLE -> "Unavailable"
                    },
                )
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    if (state.settings.dataMode != DataMode.HEALTH_CONNECT) {
                        SecondaryButton(
                            text = "Use Health Connect",
                            onClick = onUseHealthConnect,
                            modifier = Modifier.weight(1f),
                        )
                    }
                    if (state.healthPermissionState != HealthPermissionState.GRANTED) {
                        PrimaryButton(
                            text = "Grant read access",
                            onClick = onRequestPermission,
                            modifier = Modifier.weight(1f),
                        )
                    } else {
                        PrimaryButton(
                            text = "Refresh sleep",
                            onClick = onRefreshHealth,
                            modifier = Modifier.weight(1f),
                        )
                    }
                }
            }
            HealthConnectAvailability.UPDATE_REQUIRED -> {
                PrimaryButton(
                    text = "Update Health Connect",
                    onClick = onOpenHealthConnectListing,
                    modifier = Modifier.widthIn(min = 176.dp),
                )
            }
            HealthConnectAvailability.UNAVAILABLE -> {
                Text(
                    "Synthetic fixture mode remains available on devices without Health Connect.",
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
        }
    }
}

@Composable
private fun CorrectionScreen(
    state: AppUiState,
    onRetryLocalData: () -> Unit,
    onSave: (String, String) -> Unit,
) {
    val canDisplaySnapshot = state.localDataState != DurableLocalDataState.Loading
    val canSave = state.localDataState == DurableLocalDataState.Ready
    val latest = state.latestSleepEpisode
    val startZone = resolveTemporalZone(latest?.ianaTimeZoneId, latest?.startZoneOffset)
    val endZone = resolveTemporalZone(latest?.ianaTimeZoneId, latest?.endZoneOffset)
    val usesDeviceZoneFallback = latest != null && latest.ianaTimeZoneId == null &&
        (latest.startZoneOffset == null || latest.endZoneOffset == null)
    val endpointZoneLabel = if (startZone == endZone) {
        startZone.id
    } else {
        "${startZone.id} / ${endZone.id}"
    }
    val zoneLabel = endpointZoneLabel + if (usesDeviceZoneFallback) " / device fallback" else ""

    var startText by remember(latest?.source?.id, latest?.start, startZone) {
        mutableStateOf(latest?.start?.let { formatForInput(it, startZone) }.orEmpty())
    }
    var endText by remember(latest?.source?.id, latest?.end, endZone) {
        mutableStateOf(latest?.end?.let { formatForInput(it, endZone) }.orEmpty())
    }

    ScreenColumn {
        ScreenHeader(
            kicker = "Observations",
            title = "Correct sleep",
            description = "Append a correction without changing the source observation.",
        )
        DurableLocalDataNotice(state.localDataState, onRetryLocalData)
        InfoStrip("Imported observations stay unchanged. Corrections remain a separate history.")

        if (canDisplaySnapshot && latest == null && canSave) {
            RuledSection {
                Text("No sleep episode is available.", style = MaterialTheme.typography.bodyMedium)
            }
        } else if (canDisplaySnapshot && latest != null) {
            RuledSection {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween,
                ) {
                    Text("Latest principal sleep", style = MaterialTheme.typography.titleMedium)
                    Text(
                        zoneLabel,
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
                DenseTextField(
                    value = startText,
                    onValueChange = { startText = it },
                    label = "Sleep time",
                    helper = "yyyy-MM-dd HH:mm / add -04:00 or -05:00 when a time repeats",
                    imeAction = ImeAction.Next,
                )
                DenseTextField(
                    value = endText,
                    onValueChange = { endText = it },
                    label = "Wake time",
                    helper = "yyyy-MM-dd HH:mm / add an offset only when needed",
                    imeAction = ImeAction.Done,
                )
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.End,
                ) {
                    PrimaryButton(
                        text = "Save correction",
                        onClick = { onSave(startText, endText) },
                        enabled = canSave,
                        modifier = Modifier.widthIn(min = 150.dp),
                    )
                }
                latest.appliedCorrection?.let {
                    InlineStatus("A manual correction is active for this source episode.")
                }
            }
        }

        if (canDisplaySnapshot && state.correctionReviews.isNotEmpty()) {
            SectionHeading("Corrections requiring review")
            InfoStrip(
                "A source record changed after correction. ZeitBoard did not apply the old " +
                    "correction to the new revision.",
            )
            RuledSection {
                state.correctionReviews.take(5).forEachIndexed { index, review ->
                    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                        Text(
                            "Prior corrected interval",
                            style = MaterialTheme.typography.titleSmall,
                        )
                        Text(
                            formatDisplay(
                                review.correction.correctedStart,
                                review.correction.ianaTimeZoneId,
                                review.correction.startZoneOffset,
                                state.settings.use24HourTime,
                            ) + " to " + formatDisplay(
                                review.correction.correctedEnd,
                                review.correction.ianaTimeZoneId,
                                review.correction.endZoneOffset,
                                state.settings.use24HourTime,
                            ),
                            style = MaterialTheme.typography.bodyMedium,
                        )
                        Text(
                            "Source revision changed. Review the current observation and save " +
                                "a new correction only if it still applies.",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    if (index < minOf(4, state.correctionReviews.lastIndex)) {
                        HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
                    }
                }
                if (state.correctionReviews.size > 5) {
                    Text(
                        "+${state.correctionReviews.size - 5} more corrections require review.",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }
    }
}

@Composable
private fun MedicationScreen(
    state: AppUiState,
    saveState: MedicationSaveState,
    onRetryLocalData: () -> Unit,
    onSave: (String, String) -> Unit,
    onSaveResultConsumed: (Long) -> Unit,
) {
    val zone = ZoneId.systemDefault()
    var name by remember { mutableStateOf("") }
    var occurredAt by remember { mutableStateOf(formatForInput(Instant.now(), zone)) }
    val savePending = saveState is MedicationSaveState.Saving
    val canSave = state.localDataState == DurableLocalDataState.Ready && !savePending

    LaunchedEffect(saveState) {
        if (saveState is MedicationSaveState.Succeeded) {
            name = ""
            occurredAt = formatForInput(Instant.now(), zone)
            onSaveResultConsumed(saveState.requestId)
        }
    }

    ScreenColumn {
        ScreenHeader(
            kicker = "Local record",
            title = "Medication event",
            description = "Record what happened without medication or timing advice.",
        )
        DurableLocalDataNotice(state.localDataState, onRetryLocalData)

        RuledSection {
            DenseTextField(
                value = name,
                onValueChange = { name = it },
                label = "Medication label",
                placeholder = "Medication",
                imeAction = ImeAction.Next,
            )
            DenseTextField(
                value = occurredAt,
                onValueChange = { occurredAt = it },
                label = "Occurred at",
                helper = "yyyy-MM-dd HH:mm [offset] / ${zone.id}",
                imeAction = ImeAction.Done,
            )
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.End,
            ) {
                PrimaryButton(
                    text = if (savePending) "Saving..." else "Save event",
                    onClick = { onSave(name, occurredAt) },
                    enabled = canSave,
                    modifier = Modifier.widthIn(min = 132.dp),
                )
            }
            when (saveState) {
                is MedicationSaveState.Failed -> InfoStrip(saveState.message)
                is MedicationSaveState.Saving -> InlineStatus("Saving to local storage.")
                MedicationSaveState.Idle,
                is MedicationSaveState.Succeeded,
                -> Unit
            }
        }

        if (
            state.localDataState != DurableLocalDataState.Loading &&
            state.medicationEvents.isNotEmpty()
        ) {
            SectionHeading("Recent local events")
            RuledSection(verticalPadding = 0.dp, spacing = 0.dp) {
                state.medicationEvents.take(5).forEachIndexed { index, event ->
                    MedicationEventRow(event, state.settings.use24HourTime)
                    if (index < minOf(4, state.medicationEvents.lastIndex)) {
                        HorizontalDivider(
                            modifier = Modifier.padding(horizontal = 14.dp),
                            color = MaterialTheme.colorScheme.outlineVariant,
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun SettingsScreen(
    state: AppUiState,
    onDataModeChanged: (DataMode) -> Unit,
    onUse24HourChanged: (Boolean) -> Unit,
) {
    ScreenColumn {
        ScreenHeader(
            kicker = "Device",
            title = "Settings",
            description = "Local data source, display, and privacy controls.",
        )

        SectionHeading("Data source")
        RuledSection {
            Text(
                "Fixture mode is always labeled and contains synthetic data only.",
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            SegmentedDataMode(
                selected = state.settings.dataMode,
                healthConnectEnabled = state.healthAvailability == HealthConnectAvailability.AVAILABLE,
                onSelected = onDataModeChanged,
            )
        }

        SectionHeading("Display")
        RuledSection {
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.SpaceBetween,
            ) {
                Column(
                    modifier = Modifier.weight(1f),
                    verticalArrangement = Arrangement.spacedBy(3.dp),
                ) {
                    Text("24-hour time", style = MaterialTheme.typography.titleMedium)
                    Text(
                        "Use 24-hour labels for displayed times.",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                CompactSwitch(
                    checked = state.settings.use24HourTime,
                    onCheckedChange = onUse24HourChanged,
                )
            }
        }

        SectionHeading("Privacy")
        RuledSection {
            Text("Local-first companion", style = MaterialTheme.typography.titleMedium)
            PrivacyLine(
                "Imported sleep snapshots, corrections, and medication events are stored " +
                    "in ZeitBoard's app-private database.",
            )
            PrivacyLine("No analytics, telemetry, tracking SDKs, or health-data upload.")
            PrivacyLine("Medication labels and exact behavioral timestamps are never logged.")
        }
    }
}

@Composable
private fun LatestSleepPanel(
    episode: EffectiveSleepEpisode?,
    use24HourTime: Boolean,
) {
    RuledSection {
        Text("Latest sleep observation", style = MaterialTheme.typography.titleMedium)
        HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
        if (episode == null) {
            Text(
                "No sleep session is available from the selected source.",
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        } else {
            DataRow(
                label = "Sleep",
                value = formatDisplay(
                    episode.start,
                    episode.ianaTimeZoneId,
                    episode.startZoneOffset,
                    use24HourTime,
                ),
            )
            DataRow(
                label = "Wake",
                value = formatDisplay(
                    episode.end,
                    episode.ianaTimeZoneId,
                    episode.endZoneOffset,
                    use24HourTime,
                ),
            )
            if (
                episode.ianaTimeZoneId == null &&
                (episode.startZoneOffset == null || episode.endZoneOffset == null)
            ) {
                Text(
                    "Source offset unavailable; displayed in the device zone.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            InlineStatus(
                if (episode.appliedCorrection == null) {
                    "Source observation"
                } else {
                    "Manual correction applied"
                },
            )
        }
    }
}

@Composable
private fun DataRow(label: String, value: String) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.Top,
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Text(
            label.uppercase(Locale.ROOT),
            modifier = Modifier.width(68.dp),
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Text(
            value,
            modifier = Modifier.weight(1f),
            style = MaterialTheme.typography.bodyMedium,
            fontWeight = FontWeight.Medium,
        )
    }
}

@Composable
private fun ConfidenceBadge(confidence: Confidence) {
    val (text, background, foreground) = when (confidence) {
        Confidence.LOW -> Triple("LOW", Color(0xFFF1E1DF), Color(0xFF714D4A))
        Confidence.MODERATE -> Triple("MODERATE", AmberSoft, Color(0xFF695528))
        Confidence.HIGH -> Triple("HIGH", SageSoft, Color(0xFF315C49))
    }
    Text(
        text = text,
        modifier = Modifier
            .clip(MaterialTheme.shapes.small)
            .background(background)
            .padding(horizontal = 8.dp, vertical = 4.dp),
        style = MaterialTheme.typography.labelSmall,
        color = foreground,
    )
}

@Composable
private fun MedicationEventRow(event: MedicationEvent, use24HourTime: Boolean) {
    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.SpaceBetween,
    ) {
        Text(
            event.displayName,
            modifier = Modifier.weight(1f),
            style = MaterialTheme.typography.bodyMedium,
            fontWeight = FontWeight.SemiBold,
        )
        Text(
            formatDisplay(event.occurredAt, event.timeZoneId, null, use24HourTime),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
private fun SegmentedDataMode(
    selected: DataMode,
    healthConnectEnabled: Boolean,
    onSelected: (DataMode) -> Unit,
) {
    Surface(
        modifier = Modifier.fillMaxWidth(),
        shape = MaterialTheme.shapes.small,
        color = Paper,
        border = BorderStroke(1.dp, Line),
    ) {
        Row(modifier = Modifier.fillMaxWidth().height(44.dp).selectableGroup()) {
            DataModeOption(
                text = "Fixture",
                selected = selected == DataMode.FIXTURE,
                onClick = { onSelected(DataMode.FIXTURE) },
                modifier = Modifier.weight(1f),
            )
            Box(modifier = Modifier.fillMaxHeight().width(1.dp).background(Line))
            DataModeOption(
                text = "Health Connect",
                selected = selected == DataMode.HEALTH_CONNECT,
                enabled = healthConnectEnabled,
                onClick = { onSelected(DataMode.HEALTH_CONNECT) },
                modifier = Modifier.weight(1f),
            )
        }
    }
}

@Composable
private fun DataModeOption(
    text: String,
    selected: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
) {
    Box(
        modifier = modifier
            .fillMaxHeight()
            .alpha(if (enabled) 1f else 0.45f)
            .background(if (selected) SageSoft else Color.Transparent)
            .selectable(
                selected = selected,
                enabled = enabled,
                role = Role.RadioButton,
                onClick = onClick,
            ),
        contentAlignment = Alignment.Center,
    ) {
        Text(
            text,
            style = MaterialTheme.typography.labelLarge,
            color = if (selected) SageDark else Muted,
        )
    }
}

@Composable
private fun DenseTextField(
    value: String,
    onValueChange: (String) -> Unit,
    label: String,
    modifier: Modifier = Modifier,
    helper: String? = null,
    placeholder: String = "",
    imeAction: ImeAction,
) {
    val interactionSource = remember { MutableInteractionSource() }
    val focused by interactionSource.collectIsFocusedAsState()
    val shape = MaterialTheme.shapes.small

    Column(
        modifier = modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(5.dp),
    ) {
        Text(
            label.uppercase(Locale.ROOT),
            style = MaterialTheme.typography.labelSmall,
            color = if (focused) SageDark else MaterialTheme.colorScheme.onSurfaceVariant,
        )
        BasicTextField(
            value = value,
            onValueChange = onValueChange,
            modifier = Modifier
                .fillMaxWidth()
                .semantics { contentDescription = label },
            textStyle = MaterialTheme.typography.bodyMedium.copy(color = Ink),
            singleLine = true,
            interactionSource = interactionSource,
            keyboardOptions = KeyboardOptions(imeAction = imeAction),
            cursorBrush = SolidColor(SageDark),
            decorationBox = { innerTextField ->
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .heightIn(min = 44.dp)
                        .clip(shape)
                        .background(Paper)
                        .border(1.dp, if (focused) Sage else Line, shape)
                        .padding(horizontal = 11.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Box(modifier = Modifier.weight(1f)) {
                        if (value.isEmpty() && placeholder.isNotEmpty()) {
                            Text(
                                placeholder,
                                style = MaterialTheme.typography.bodyMedium,
                                color = Subtle,
                            )
                        }
                        innerTextField()
                    }
                }
            },
        )
        helper?.let {
            Text(
                it,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun PrimaryButton(
    text: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
) {
    Button(
        onClick = onClick,
        enabled = enabled,
        modifier = modifier.heightIn(min = 48.dp),
        shape = MaterialTheme.shapes.small,
        colors = ButtonDefaults.buttonColors(
            containerColor = SageDark,
            contentColor = Color.White,
        ),
        contentPadding = PaddingValues(horizontal = 13.dp, vertical = 0.dp),
    ) {
        Text(text, style = MaterialTheme.typography.labelLarge)
    }
}

@Composable
private fun SecondaryButton(
    text: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    OutlinedButton(
        onClick = onClick,
        modifier = modifier.heightIn(min = 44.dp),
        shape = MaterialTheme.shapes.small,
        border = BorderStroke(1.dp, Color(0xFFCFD5D0)),
        colors = ButtonDefaults.outlinedButtonColors(contentColor = SageDark),
        contentPadding = PaddingValues(horizontal = 12.dp, vertical = 0.dp),
    ) {
        Text(text, style = MaterialTheme.typography.labelLarge)
    }
}

@Composable
private fun CompactSwitch(
    checked: Boolean,
    onCheckedChange: (Boolean) -> Unit,
) {
    Box(
        modifier = Modifier
            .size(48.dp)
            .toggleable(
                value = checked,
                role = Role.Switch,
                onValueChange = onCheckedChange,
            ),
        contentAlignment = Alignment.Center,
    ) {
        Box(
            modifier = Modifier
                .width(36.dp)
                .height(20.dp)
                .clip(CircleShape)
                .background(if (checked) SageDark else Chrome)
                .border(1.dp, if (checked) SageDark else Line, CircleShape),
        )
        Box(
            modifier = Modifier
                .align(if (checked) Alignment.CenterEnd else Alignment.CenterStart)
                .padding(horizontal = 8.dp)
                .size(16.dp)
                .background(if (checked) Color.White else Subtle, CircleShape),
        )
    }
}

@Composable
private fun RuledSection(
    modifier: Modifier = Modifier,
    verticalPadding: Dp = 12.dp,
    spacing: Dp = 10.dp,
    content: @Composable ColumnScope.() -> Unit,
) {
    Column(modifier = modifier.fillMaxWidth()) {
        HorizontalDivider(color = MaterialTheme.colorScheme.outline)
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(vertical = verticalPadding),
            verticalArrangement = Arrangement.spacedBy(spacing),
            content = content,
        )
        HorizontalDivider(color = MaterialTheme.colorScheme.outline)
    }
}

@Composable
private fun ScreenHeader(kicker: String, title: String, description: String) {
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text(
            kicker.uppercase(Locale.ROOT),
            style = MaterialTheme.typography.labelSmall,
            color = Sage,
        )
        Text(title, style = MaterialTheme.typography.headlineLarge)
        Text(
            description,
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
private fun SectionHeading(text: String) {
    Text(
        text.uppercase(Locale.ROOT),
        modifier = Modifier.padding(top = 2.dp),
        style = MaterialTheme.typography.labelMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
}

@Composable
private fun DurableLocalDataNotice(
    state: DurableLocalDataState,
    onRetry: () -> Unit,
) {
    when (state) {
        DurableLocalDataState.Loading ->
            InfoStrip("Loading saved local data.")
        DurableLocalDataState.Ready -> Unit
        is DurableLocalDataState.Failed -> {
            RuledSection {
                Text(
                    state.message,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.error,
                )
                Text(
                    "Any data shown is the last snapshot loaded successfully. New local writes " +
                        "are disabled until storage recovers.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                SecondaryButton(
                    text = "Retry local data",
                    onClick = onRetry,
                    modifier = Modifier.widthIn(min = 140.dp),
                )
            }
        }
    }
}

@Composable
private fun InfoStrip(text: String) {
    Row(
        modifier = Modifier.fillMaxWidth().background(SageSoft),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(modifier = Modifier.width(3.dp).heightIn(min = 42.dp).background(Sage))
        Text(
            text,
            modifier = Modifier.padding(horizontal = 11.dp, vertical = 9.dp),
            style = MaterialTheme.typography.bodySmall,
            color = SageDark,
        )
    }
}

@Composable
private fun InlineStatus(text: String) {
    Row(
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(7.dp),
    ) {
        StatusDot(color = Sage, size = 6.dp)
        Text(
            text,
            style = MaterialTheme.typography.labelMedium,
            color = SageDark,
        )
    }
}

@Composable
private fun PrivacyLine(text: String) {
    Row(
        verticalAlignment = Alignment.Top,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        StatusDot(color = Sage, size = 5.dp, modifier = Modifier.padding(top = 5.dp))
        Text(
            text,
            modifier = Modifier.weight(1f),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
private fun StatusDot(
    color: Color,
    modifier: Modifier = Modifier,
    size: Dp = 7.dp,
) {
    Box(modifier = modifier.size(size).background(color, CircleShape))
}

@Composable
private fun BrandBar() {
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .background(Chrome)
            .statusBarsPadding(),
    ) {
        Row(
            modifier = Modifier.fillMaxWidth().height(56.dp).padding(horizontal = 16.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            OrbitMark()
            Column(verticalArrangement = Arrangement.spacedBy(1.dp)) {
                Text(
                    "ZeitBoard",
                    style = MaterialTheme.typography.titleMedium,
                    fontWeight = FontWeight.Bold,
                )
                Text(
                    "COMPANION",
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
        HorizontalDivider(color = Line)
    }
}

@Composable
private fun OrbitMark() {
    Canvas(
        modifier = Modifier
            .size(28.dp)
            .semantics { contentDescription = "ZeitBoard" },
    ) {
        val center = Offset(size.width / 2f, size.height / 2f)
        drawCircle(color = Sage, radius = size.minDimension * 0.47f, style = Stroke(width = 1.4.dp.toPx()))
        drawArc(
            color = Sage,
            startAngle = -70f,
            sweepAngle = 285f,
            useCenter = false,
            topLeft = Offset(size.width * 0.19f, size.height * 0.19f),
            size = Size(size.width * 0.62f, size.height * 0.62f),
            style = Stroke(width = 1.2.dp.toPx(), cap = StrokeCap.Round),
        )
        drawCircle(color = Sage, radius = size.minDimension * 0.13f, center = center)
        drawCircle(
            color = Chrome,
            radius = size.minDimension * 0.065f,
            center = Offset(size.width * 0.55f, size.height * 0.03f),
        )
    }
}

@Composable
private fun FixtureBanner() {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .background(AmberSoft)
            .padding(horizontal = 16.dp, vertical = 7.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        StatusDot(color = Amber, size = 6.dp)
        Text(
            "FIXTURE MODE / SYNTHETIC DATA",
            style = MaterialTheme.typography.labelSmall,
            color = Color(0xFF695528),
        )
    }
}

@Composable
private fun DestinationBar(
    currentRoute: String,
    onDestinationSelected: (Destination) -> Unit,
) {
    Column(modifier = Modifier.fillMaxWidth().background(Chrome).navigationBarsPadding()) {
        HorizontalDivider(color = Line)
        Row(
            modifier = Modifier.fillMaxWidth().height(58.dp).selectableGroup(),
        ) {
            Destination.entries.forEach { destination ->
                val selected = currentRoute == destination.route
                Column(
                    modifier = Modifier
                        .weight(1f)
                        .fillMaxHeight()
                        .background(if (selected) Paper else Chrome)
                        .selectable(
                            selected = selected,
                            role = Role.Tab,
                            onClick = { onDestinationSelected(destination) },
                        ),
                    horizontalAlignment = Alignment.CenterHorizontally,
                ) {
                    Box(
                        modifier = Modifier
                            .fillMaxWidth()
                            .height(2.dp)
                            .background(if (selected) Sage else Color.Transparent),
                    )
                    Spacer(modifier = Modifier.height(6.dp))
                    DestinationGlyph(
                        destination.icon,
                        color = if (selected) SageDark else Muted,
                    )
                    Spacer(modifier = Modifier.height(3.dp))
                    Text(
                        destination.label,
                        style = MaterialTheme.typography.labelSmall,
                        color = if (selected) SageDark else Muted,
                    )
                }
            }
        }
    }
}

@Composable
private fun DestinationGlyph(icon: DestinationIcon, color: Color) {
    Canvas(modifier = Modifier.size(17.dp)) {
        val stroke = 1.5.dp.toPx()
        when (icon) {
            DestinationIcon.STATUS -> {
                val square = size.width * 0.31f
                val gap = size.width * 0.18f
                listOf(
                    Offset(0f, 0f),
                    Offset(square + gap, 0f),
                    Offset(0f, square + gap),
                    Offset(square + gap, square + gap),
                ).forEach { topLeft ->
                    drawRoundRect(
                        color = color,
                        topLeft = topLeft,
                        size = Size(square, square),
                        cornerRadius = CornerRadius(1.5.dp.toPx()),
                        style = Stroke(width = stroke),
                    )
                }
            }
            DestinationIcon.CORRECT -> {
                drawLine(color, Offset(size.width * 0.12f, size.height * 0.28f), Offset(size.width * 0.88f, size.height * 0.28f), stroke, StrokeCap.Round)
                drawLine(color, Offset(size.width * 0.12f, size.height * 0.72f), Offset(size.width * 0.88f, size.height * 0.72f), stroke, StrokeCap.Round)
                drawCircle(color, size.width * 0.10f, Offset(size.width * 0.35f, size.height * 0.28f))
                drawCircle(color, size.width * 0.10f, Offset(size.width * 0.66f, size.height * 0.72f))
            }
            DestinationIcon.MEDICATION -> {
                rotate(-38f) {
                    drawRoundRect(
                        color = color,
                        topLeft = Offset(size.width * 0.23f, size.height * 0.08f),
                        size = Size(size.width * 0.54f, size.height * 0.84f),
                        cornerRadius = CornerRadius(size.width * 0.27f),
                        style = Stroke(width = stroke),
                    )
                    drawLine(
                        color,
                        Offset(size.width * 0.23f, size.height * 0.50f),
                        Offset(size.width * 0.77f, size.height * 0.50f),
                        stroke,
                    )
                }
            }
            DestinationIcon.SETTINGS -> {
                val center = Offset(size.width / 2f, size.height / 2f)
                drawCircle(color, size.width * 0.31f, center, style = Stroke(width = stroke))
                drawCircle(color, size.width * 0.09f, center)
                repeat(4) { index ->
                    rotate(index * 90f, center) {
                        drawLine(
                            color,
                            Offset(center.x, size.height * 0.02f),
                            Offset(center.x, size.height * 0.18f),
                            stroke,
                            StrokeCap.Round,
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun ScreenColumn(content: @Composable ColumnScope.() -> Unit) {
    Box(
        modifier = Modifier.fillMaxSize(),
        contentAlignment = Alignment.TopCenter,
    ) {
        Column(
            modifier = Modifier
                .widthIn(max = 680.dp)
                .fillMaxWidth()
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 16.dp, vertical = 14.dp),
            verticalArrangement = Arrangement.spacedBy(11.dp),
            content = content,
        )
    }
}

private fun formatForInput(instant: Instant, zoneId: ZoneId): String =
    AppViewModel.INPUT_FORMATTER.format(instant.atZone(zoneId))

private fun formatDisplay(
    instant: Instant,
    ianaTimeZoneId: String?,
    offset: ZoneOffset?,
    use24HourTime: Boolean,
): String {
    val zone = resolveTemporalZone(ianaTimeZoneId, offset)
    val pattern = if (use24HourTime) "EEE, MMM d HH:mm z" else "EEE, MMM d h:mm a z"
    return DateTimeFormatter.ofPattern(pattern, Locale.getDefault()).format(instant.atZone(zone))
}

private fun formatWindow(window: TimeWindow, use24HourTime: Boolean): String {
    val zone = ZoneId.systemDefault()
    val start = window.start.atZone(zone)
    val end = window.end.atZone(zone)
    val dateFormatter = DateTimeFormatter.ofPattern("EEE, MMM d", Locale.getDefault())
    val timeFormatter = DateTimeFormatter.ofPattern(
        if (use24HourTime) "HH:mm" else "h:mm a",
        Locale.getDefault(),
    )
    val zoneFormatter = DateTimeFormatter.ofPattern("z", Locale.getDefault())
    return if (start.toLocalDate() == end.toLocalDate()) {
        "${dateFormatter.format(start)} / ${timeFormatter.format(start)} - " +
            "${timeFormatter.format(end)} ${zoneFormatter.format(end)}"
    } else {
        "${dateFormatter.format(start)} ${timeFormatter.format(start)} - " +
            "${dateFormatter.format(end)} ${timeFormatter.format(end)} ${zoneFormatter.format(end)}"
    }
}
