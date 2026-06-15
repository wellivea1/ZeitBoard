package org.non24.planner.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.NavGraph.Companion.findStartDestination
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.Locale
import org.non24.planner.domain.Confidence
import org.non24.planner.domain.DataMode
import org.non24.planner.domain.EffectiveSleepEpisode
import org.non24.planner.domain.HealthConnectAvailability
import org.non24.planner.domain.HealthPermissionState
import org.non24.planner.domain.MedicationEvent
import org.non24.planner.domain.TimeWindow

private enum class Destination(
    val route: String,
    val label: String,
    val shortLabel: String,
) {
    STATUS("status", "Status", "S"),
    CORRECT("correct", "Correct", "C"),
    MEDICATION("medication", "Medication", "M"),
    SETTINGS("settings", "Settings", "G"),
}

private val lightColors = lightColorScheme(
    primary = Color(0xFF3559A8),
    onPrimary = Color.White,
    secondary = Color(0xFF57617A),
    surface = Color(0xFFF7F8FC),
    surfaceVariant = Color(0xFFE1E5F0),
)

private val darkColors = darkColorScheme(
    primary = Color(0xFFAEC6FF),
    secondary = Color(0xFFC0C6DC),
)

@Composable
fun Non24Theme(
    darkTheme: Boolean = false,
    content: @Composable () -> Unit,
) {
    MaterialTheme(
        colorScheme = if (darkTheme) darkColors else lightColors,
        content = content,
    )
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
            topBar = {
                Column {
                    Surface(shadowElevation = 2.dp) {
                        Text(
                            text = "ZeitBoard",
                            modifier = Modifier.fillMaxWidth().padding(horizontal = 20.dp, vertical = 16.dp),
                            style = MaterialTheme.typography.titleLarge,
                            fontWeight = FontWeight.SemiBold,
                        )
                    }
                    if (uiState.settings.dataMode == DataMode.FIXTURE) {
                        FixtureBanner()
                    }
                }
            },
            snackbarHost = { SnackbarHost(snackbarHostState) },
            bottomBar = {
                NavigationBar {
                    Destination.entries.forEach { destination ->
                        NavigationBarItem(
                            selected = currentRoute == destination.route,
                            onClick = {
                                navController.navigate(destination.route) {
                                    popUpTo(navController.graph.findStartDestination().id) {
                                        saveState = true
                                    }
                                    launchSingleTop = true
                                    restoreState = true
                                }
                            },
                            icon = { Text(destination.shortLabel, fontWeight = FontWeight.Bold) },
                            label = { Text(destination.label) },
                        )
                    }
                }
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
                        onUseHealthConnect = { viewModel.setDataMode(DataMode.HEALTH_CONNECT) },
                        onRequestPermission = { onRequestHealthPermissions(requiredHealthPermissions) },
                        onRefreshHealth = viewModel::refreshHealthConnect,
                        onOpenHealthConnectListing = onOpenHealthConnectListing,
                    )
                }
                composable(Destination.CORRECT.route) {
                    CorrectionScreen(
                        state = uiState,
                        onSave = viewModel::saveLatestSleepCorrection,
                    )
                }
                composable(Destination.MEDICATION.route) {
                    MedicationScreen(
                        state = uiState,
                        onSave = viewModel::addMedicationEvent,
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
    onUseHealthConnect: () -> Unit,
    onRequestPermission: () -> Unit,
    onRefreshHealth: () -> Unit,
    onOpenHealthConnectListing: () -> Unit,
) {
    ScreenColumn {
        SectionTitle("Current status")
        if (state.estimate != null) {
            StatusEstimateCard(state)
        } else {
            Card(modifier = Modifier.fillMaxWidth()) {
                Column(
                    modifier = Modifier.padding(18.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    Text("No Android estimate", style = MaterialTheme.typography.titleMedium)
                    Text(
                        "Health Connect mode imports sleep sessions only. Estimated sleep-wake " +
                            "phase and predicted windows must come from the shared core through a repository contract.",
                    )
                }
            }
        }

        LatestSleepCard(
            episode = state.latestSleepEpisode,
            use24HourTime = state.settings.use24HourTime,
        )

        SectionTitle("Health Connect")
        HealthConnectCard(
            state = state,
            onUseHealthConnect = onUseHealthConnect,
            onRequestPermission = onRequestPermission,
            onRefreshHealth = onRefreshHealth,
            onOpenHealthConnectListing = onOpenHealthConnectListing,
        )

        Text(
            "This scaffold is not a medical device and does not identify exact circadian phase or DLMO.",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
private fun StatusEstimateCard(state: AppUiState) {
    val estimate = requireNotNull(state.estimate)
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier.padding(18.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Text(estimate.label, style = MaterialTheme.typography.titleMedium)
            ConfidenceLabel(estimate.confidence)
            WindowRow(
                label = "Predicted sleep window",
                window = estimate.predictedSleepWindow,
                use24HourTime = state.settings.use24HourTime,
            )
            WindowRow(
                label = "Predicted waking window",
                window = estimate.predictedWakingWindow,
                use24HourTime = state.settings.use24HourTime,
            )
            HorizontalDivider()
            estimate.confidenceReasons.forEach { reason ->
                Text("- $reason", style = MaterialTheme.typography.bodySmall)
            }
            Text(
                "Imported snapshot: ${estimate.algorithmVersion}",
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun HealthConnectCard(
    state: AppUiState,
    onUseHealthConnect: () -> Unit,
    onRequestPermission: () -> Unit,
    onRefreshHealth: () -> Unit,
    onOpenHealthConnectListing: () -> Unit,
) {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier.padding(18.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            Text(
                when (state.healthAvailability) {
                    HealthConnectAvailability.AVAILABLE -> "Available on this device"
                    HealthConnectAvailability.UPDATE_REQUIRED -> "Provider update required"
                    HealthConnectAvailability.UNAVAILABLE -> "Unavailable on this device"
                },
                style = MaterialTheme.typography.titleMedium,
            )
            Text("Requested access: read sleep sessions only.")
            when (state.healthAvailability) {
                HealthConnectAvailability.AVAILABLE -> {
                    Text(
                        when (state.healthPermissionState) {
                            HealthPermissionState.GRANTED -> "Sleep permission granted"
                            HealthPermissionState.REQUIRED -> "Sleep permission required"
                            HealthPermissionState.UNKNOWN -> "Checking sleep permission"
                            HealthPermissionState.UNAVAILABLE -> "Sleep permission unavailable"
                        },
                    )
                    Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                        if (state.settings.dataMode != DataMode.HEALTH_CONNECT) {
                            OutlinedButton(onClick = onUseHealthConnect) { Text("Use Health Connect") }
                        }
                        if (state.healthPermissionState != HealthPermissionState.GRANTED) {
                            Button(onClick = onRequestPermission) { Text("Grant read access") }
                        } else {
                            Button(onClick = onRefreshHealth) { Text("Refresh sleep") }
                        }
                    }
                }
                HealthConnectAvailability.UPDATE_REQUIRED -> {
                    Button(onClick = onOpenHealthConnectListing) { Text("Update Health Connect") }
                }
                HealthConnectAvailability.UNAVAILABLE -> {
                    Text("Fixture mode remains available on Android 8 and devices without Health Connect.")
                }
            }
        }
    }
}

@Composable
private fun CorrectionScreen(
    state: AppUiState,
    onSave: (String, String) -> Unit,
) {
    val latest = state.latestSleepEpisode
    val zone = zoneFor(latest?.timeZoneId)
    var startText by remember(latest?.source?.id, latest?.start) {
        mutableStateOf(latest?.start?.let { formatForInput(it, zone) }.orEmpty())
    }
    var endText by remember(latest?.source?.id, latest?.end) {
        mutableStateOf(latest?.end?.let { formatForInput(it, zone) }.orEmpty())
    }

    ScreenColumn {
        SectionTitle("Manual sleep/wake correction")
        Text(
            "Corrections are appended separately. Imported source observations are not rewritten.",
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        if (latest == null) {
            Card(modifier = Modifier.fillMaxWidth()) {
                Text("No sleep episode is available.", modifier = Modifier.padding(18.dp))
            }
        } else {
            Card(modifier = Modifier.fillMaxWidth()) {
                Column(
                    modifier = Modifier.padding(18.dp),
                    verticalArrangement = Arrangement.spacedBy(12.dp),
                ) {
                    Text("Latest principal sleep", style = MaterialTheme.typography.titleMedium)
                    Text("Time zone: ${zone.id}")
                    OutlinedTextField(
                        value = startText,
                        onValueChange = { startText = it },
                        label = { Text("Sleep time") },
                        supportingText = { Text("yyyy-MM-dd HH:mm") },
                        modifier = Modifier.fillMaxWidth(),
                        singleLine = true,
                    )
                    OutlinedTextField(
                        value = endText,
                        onValueChange = { endText = it },
                        label = { Text("Wake time") },
                        supportingText = { Text("yyyy-MM-dd HH:mm") },
                        modifier = Modifier.fillMaxWidth(),
                        singleLine = true,
                    )
                    Button(
                        onClick = { onSave(startText, endText) },
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        Text("Save correction")
                    }
                    latest.appliedCorrection?.let {
                        Text(
                            "A manual correction is active for this source episode.",
                            style = MaterialTheme.typography.bodySmall,
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun MedicationScreen(
    state: AppUiState,
    onSave: (String, String) -> Unit,
) {
    val zone = ZoneId.systemDefault()
    var name by remember { mutableStateOf("") }
    var occurredAt by remember {
        mutableStateOf(formatForInput(Instant.now(), zone))
    }

    ScreenColumn {
        SectionTitle("Quick medication event")
        Text(
            "Record what happened. This screen does not recommend medication or timing.",
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Card(modifier = Modifier.fillMaxWidth()) {
            Column(
                modifier = Modifier.padding(18.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                OutlinedTextField(
                    value = name,
                    onValueChange = { name = it },
                    label = { Text("Medication label") },
                    placeholder = { Text("Medication") },
                    modifier = Modifier.fillMaxWidth(),
                    singleLine = true,
                )
                OutlinedTextField(
                    value = occurredAt,
                    onValueChange = { occurredAt = it },
                    label = { Text("Occurred at") },
                    supportingText = { Text("yyyy-MM-dd HH:mm (${zone.id})") },
                    modifier = Modifier.fillMaxWidth(),
                    singleLine = true,
                )
                Button(
                    onClick = {
                        onSave(name, occurredAt)
                        name = ""
                    },
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    Text("Save event")
                }
            }
        }
        if (state.medicationEvents.isNotEmpty()) {
            SectionTitle("Recent local events")
            state.medicationEvents.take(5).forEach { event ->
                MedicationEventCard(event, state.settings.use24HourTime)
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
        SectionTitle("Data source")
        Card(modifier = Modifier.fillMaxWidth()) {
            Column(
                modifier = Modifier.padding(18.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                Text("Fixture mode is intentionally visible and contains synthetic data only.")
                Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                    FilterChip(
                        selected = state.settings.dataMode == DataMode.FIXTURE,
                        onClick = { onDataModeChanged(DataMode.FIXTURE) },
                        label = { Text("Fixture") },
                    )
                    FilterChip(
                        selected = state.settings.dataMode == DataMode.HEALTH_CONNECT,
                        onClick = { onDataModeChanged(DataMode.HEALTH_CONNECT) },
                        enabled = state.healthAvailability == HealthConnectAvailability.AVAILABLE,
                        label = { Text("Health Connect") },
                    )
                }
            }
        }

        SectionTitle("Display")
        Card(modifier = Modifier.fillMaxWidth()) {
            Row(
                modifier = Modifier.fillMaxWidth().padding(18.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.SpaceBetween,
            ) {
                Column(modifier = Modifier.weight(1f)) {
                    Text("24-hour time", style = MaterialTheme.typography.titleMedium)
                    Text("Use 24-hour labels for displayed times.")
                }
                Switch(
                    checked = state.settings.use24HourTime,
                    onCheckedChange = onUse24HourChanged,
                )
            }
        }

        SectionTitle("Privacy")
        Card(modifier = Modifier.fillMaxWidth()) {
            Column(
                modifier = Modifier.padding(18.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                Text("Local-first phase-one scaffold", style = MaterialTheme.typography.titleMedium)
                Text("No analytics, telemetry, tracking SDKs, or health-data upload.")
                Text("Medication labels and exact behavioral timestamps are never written to logs.")
            }
        }
    }
}

@Composable
private fun LatestSleepCard(
    episode: EffectiveSleepEpisode?,
    use24HourTime: Boolean,
) {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier.padding(18.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Text("Latest sleep observation", style = MaterialTheme.typography.titleMedium)
            if (episode == null) {
                Text("No sleep session is available from the selected source.")
            } else {
                Text("Sleep: ${formatDisplay(episode.start, episode.timeZoneId, use24HourTime)}")
                Text("Wake: ${formatDisplay(episode.end, episode.timeZoneId, use24HourTime)}")
                Text(
                    if (episode.appliedCorrection == null) "Source observation" else "Manual correction applied",
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.primary,
                )
            }
        }
    }
}

@Composable
private fun WindowRow(
    label: String,
    window: TimeWindow,
    use24HourTime: Boolean,
) {
    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Text(label, style = MaterialTheme.typography.labelLarge)
        Text(
            "${formatDisplay(window.start, ZoneId.systemDefault().id, use24HourTime)} to " +
                formatDisplay(window.end, ZoneId.systemDefault().id, use24HourTime),
        )
    }
}

@Composable
private fun ConfidenceLabel(confidence: Confidence) {
    val text = when (confidence) {
        Confidence.LOW -> "Low confidence"
        Confidence.MODERATE -> "Moderate confidence"
        Confidence.HIGH -> "High confidence"
    }
    Text(
        text = text,
        modifier = Modifier
            .background(MaterialTheme.colorScheme.surfaceVariant, MaterialTheme.shapes.small)
            .padding(horizontal = 10.dp, vertical = 6.dp),
        style = MaterialTheme.typography.labelLarge,
    )
}

@Composable
private fun MedicationEventCard(event: MedicationEvent, use24HourTime: Boolean) {
    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceVariant),
    ) {
        Column(modifier = Modifier.padding(14.dp)) {
            Text(event.displayName, fontWeight = FontWeight.SemiBold)
            Text(formatDisplay(event.occurredAt, event.timeZoneId, use24HourTime))
        }
    }
}

@Composable
private fun FixtureBanner() {
    Box(
        modifier = Modifier
            .fillMaxWidth()
            .background(Color(0xFFFFE3A3), MaterialTheme.shapes.medium)
            .padding(14.dp),
    ) {
        Text(
            "FIXTURE MODE - SYNTHETIC DATA",
            color = Color(0xFF5F4500),
            fontWeight = FontWeight.Bold,
        )
    }
}

@Composable
private fun SectionTitle(text: String) {
    Text(text, style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.SemiBold)
}

@Composable
private fun ScreenColumn(content: @Composable ColumnScope.() -> Unit) {
    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(PaddingValues(horizontal = 18.dp, vertical = 20.dp)),
        verticalArrangement = Arrangement.spacedBy(14.dp),
        content = content,
    )
}

private fun zoneFor(zoneId: String?): ZoneId =
    runCatching { ZoneId.of(zoneId ?: ZoneId.systemDefault().id) }
        .getOrDefault(ZoneId.systemDefault())

private fun formatForInput(instant: Instant, zoneId: ZoneId): String =
    AppViewModel.INPUT_FORMATTER.format(instant.atZone(zoneId))

private fun formatDisplay(instant: Instant, zoneId: String, use24HourTime: Boolean): String {
    val zone = zoneFor(zoneId)
    val pattern = if (use24HourTime) "EEE, MMM d HH:mm z" else "EEE, MMM d h:mm a z"
    return DateTimeFormatter.ofPattern(pattern, Locale.getDefault()).format(instant.atZone(zone))
}
