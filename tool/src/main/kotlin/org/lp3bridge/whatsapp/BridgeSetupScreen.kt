package org.lp3bridge.whatsapp

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.text.input.rememberTextFieldState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewModelScope
import com.thelightphone.sdk.LightQrCodeScanner
import com.thelightphone.sdk.LightScreen
import com.thelightphone.sdk.LightViewModel
import com.thelightphone.sdk.SealedLightActivity
import com.thelightphone.sdk.SimpleLightScreen
import com.thelightphone.sdk.ui.LightBarButton
import com.thelightphone.sdk.ui.LightBottomBar
import com.thelightphone.sdk.ui.LightIcons
import com.thelightphone.sdk.ui.LightText
import com.thelightphone.sdk.ui.LightTextInputEditor
import com.thelightphone.sdk.ui.LightTextVariant
import com.thelightphone.sdk.ui.LightTheme
import com.thelightphone.sdk.ui.LightThemeController
import com.thelightphone.sdk.ui.LightThemeTokens
import com.thelightphone.sdk.ui.LightTopBar
import com.thelightphone.sdk.ui.LightTopBarCenter
import com.thelightphone.sdk.ui.defaultKeyboardOptions
import com.thelightphone.sdk.ui.lightClickable
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch

enum class BridgeSetupResult { SAVED, CLEARED }

class BridgeSetupViewModel(
    private val configStore: BridgeConfigStore,
    private val verifier: BridgeConfigurationVerifier = BridgeConfigurationVerifier(),
) : LightViewModel<BridgeSetupResult>() {
    data class State(
        val loadingSummary: Boolean = true,
        val summary: BridgeConfigSummary? = null,
        val candidate: BridgeConfig? = null,
        val testing: Boolean = false,
        val error: String? = null,
        val result: BridgeSetupResult? = null,
    )

    private val _state = MutableStateFlow(State())
    val state: StateFlow<State> = _state
    private var summaryLoaded = false

    override fun onScreenShow(screen: SimpleLightScreen<BridgeSetupResult>) {
        super.onScreenShow(screen)
        if (summaryLoaded) return
        summaryLoaded = true
        viewModelScope.launch(Dispatchers.IO) {
            _state.value = _state.value.copy(
                loadingSummary = false,
                summary = configStore.summary(),
            )
        }
    }

    fun acceptQr(payload: String) {
        val config = parseQrBridgeConfig(payload).getOrElse { error ->
            _state.value = _state.value.copy(error = error.message ?: "Invalid setup QR code")
            return
        }
        verifyAndSave(config)
    }

    fun acceptManual(baseUrl: String, apiToken: String) {
        val config = runCatching { bridgeConfig(baseUrl, apiToken) }.getOrElse { error ->
            _state.value = _state.value.copy(error = error.message ?: "Invalid bridge setup")
            return
        }
        verifyAndSave(config)
    }

    fun retry() {
        _state.value.candidate?.let(::verifyAndSave)
    }

    fun clear() {
        viewModelScope.launch(Dispatchers.IO) {
            configStore.clear()
            _state.value = _state.value.copy(result = BridgeSetupResult.CLEARED)
        }
    }

    private fun verifyAndSave(config: BridgeConfig) {
        viewModelScope.launch(Dispatchers.IO) {
            _state.value = _state.value.copy(candidate = config, testing = true, error = null)
            try {
                // The unauthenticated health check always precedes authenticated status validation.
                verifier.verify(config)
                configStore.replace(config)
                _state.value = _state.value.copy(testing = false, result = BridgeSetupResult.SAVED)
            } catch (error: Exception) {
                _state.value = _state.value.copy(
                    testing = false,
                    error = if (isBridgeUnauthorized(error)) {
                        "The API token was rejected. Retry, rescan, or reconfigure."
                    } else {
                        error.message ?: "Bridge validation failed. Retry, rescan, or reconfigure."
                    },
                )
            }
        }
    }
}

class BridgeSetupScreen(
    sealedActivity: SealedLightActivity,
    private val configStore: BridgeConfigStore,
) : LightScreen<BridgeSetupResult, BridgeSetupViewModel>(sealedActivity) {
    override val viewModelClass = BridgeSetupViewModel::class.java
    override fun createViewModel() = BridgeSetupViewModel(configStore)

    @Composable
    override fun Content() {
        val state by viewModel.state.collectAsState()
        val colors by LightThemeController.colors.collectAsState()
        LaunchedEffect(state.result) {
            state.result?.let(::goBack)
        }
        LightTheme(colors = colors) {
            Column(modifier = Modifier.fillMaxSize().background(LightThemeTokens.colors.background)) {
                LightTopBar(
                    leftButton = LightBarButton.LightIcon(LightIcons.BACK, onClick = { goBack() }),
                    center = LightTopBarCenter.Text("Bridge setup"),
                )
                when {
                    state.loadingSummary -> LightText(
                        "Loading setup…", LightTextVariant.Copy, modifier = Modifier.padding(32.dp),
                    )
                    state.testing -> LightText(
                        "Testing bridge…", LightTextVariant.Copy, modifier = Modifier.padding(32.dp),
                    )
                    else -> SetupDetails(state)
                }
                if (!state.loadingSummary && !state.testing) {
                    LightBottomBar(items = setupActions(state))
                }
            }
        }
    }

    @Composable
    private fun SetupDetails(state: BridgeSetupViewModel.State) {
        Column(modifier = Modifier.padding(32.dp)) {
            if (state.summary == null) {
                LightText("Connect WhatsApp", LightTextVariant.Heading)
                LightText(
                    "Scan a setup QR code from your bridge. Manual setup is available if scanning is not possible.",
                    LightTextVariant.Detail,
                    lighten = true,
                    modifier = Modifier.padding(top = 12.dp),
                )
            } else {
                LightText("Bridge configured", LightTextVariant.Heading)
                LightText(
                    state.summary.baseUrl,
                    LightTextVariant.Detail,
                    lighten = true,
                    modifier = Modifier.padding(top = 12.dp),
                )
                LightText(
                    "API token: ${state.summary.tokenMarker}",
                    LightTextVariant.Detail,
                    lighten = true,
                    modifier = Modifier.padding(top = 4.dp),
                )
            }
            state.error?.let { error ->
                LightText(
                    error,
                    LightTextVariant.Detail,
                    lighten = true,
                    modifier = Modifier.padding(top = 24.dp),
                )
                if (state.candidate != null) {
                    LightText(
                        "RETRY",
                        LightTextVariant.Button,
                        modifier = Modifier.padding(top = 16.dp).lightClickable(onClick = viewModel::retry),
                    )
                }
            }
        }
    }

    private fun setupActions(state: BridgeSetupViewModel.State): List<LightBarButton> = buildList {
        add(LightBarButton.Text("SCAN QR", onClick = ::scanQr))
        add(LightBarButton.Text("MANUAL", onClick = ::startManualSetup))
        if (state.summary != null) add(LightBarButton.Text("CLEAR", onClick = viewModel::clear))
    }

    private fun scanQr() {
        navigateTo(
            screenFactory = { BridgeQrScannerScreen(it) },
            resultCallback = viewModel::acceptQr,
        )
    }

    private fun startManualSetup() {
        navigateTo(
            screenFactory = { BridgeTextEditorScreen(it, "Bridge URL", "NEXT") },
            resultCallback = { baseUrl ->
                if (baseUrl.isNotBlank()) {
                    navigateTo(
                        screenFactory = { BridgeTextEditorScreen(it, "API token", "TEST") },
                        resultCallback = { apiToken -> viewModel.acceptManual(baseUrl, apiToken) },
                    )
                }
            },
        )
    }
}

class BridgeQrScannerScreen(sealedActivity: SealedLightActivity) : SimpleLightScreen<String>(sealedActivity) {
    @Composable
    override fun Content() {
        val colors by LightThemeController.colors.collectAsState()
        LightTheme(colors = colors) {
            LightQrCodeScanner(
                title = "Scan bridge setup",
                onScanned = { payload -> goBack(payload) },
                onBack = { goBack() },
            )
        }
    }
}

class BridgeTextEditorViewModel : LightViewModel<String>() {
    val keyboardOptions = MutableStateFlow(defaultKeyboardOptions())
}

class BridgeTextEditorScreen(
    sealedActivity: SealedLightActivity,
    private val title: String,
    private val submitLabel: String,
) : LightScreen<String, BridgeTextEditorViewModel>(sealedActivity) {
    override val viewModelClass = BridgeTextEditorViewModel::class.java
    override fun createViewModel() = BridgeTextEditorViewModel()

    @Composable
    override fun Content() {
        val colors by LightThemeController.colors.collectAsState()
        val text = rememberTextFieldState()
        LightTheme(colors = colors) {
            LightTextInputEditor(
                title = title,
                state = text,
                keyboardOptionsFlow = viewModel.keyboardOptions,
                submitLabel = submitLabel,
                submitIcon = LightIcons.ACCEPT,
                singleLine = true,
                onSubmit = { goBack(it.toString()) },
                onBack = ::goBack,
            )
        }
    }
}
