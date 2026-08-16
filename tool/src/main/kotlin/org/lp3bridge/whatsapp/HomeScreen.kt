package org.lp3bridge.whatsapp

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewModelScope
import com.thelightphone.sdk.InitialScreen
import com.thelightphone.sdk.LightScreen
import com.thelightphone.sdk.LightViewModel
import com.thelightphone.sdk.SealedLightActivity
import com.thelightphone.sdk.SimpleLightScreen
import com.thelightphone.sdk.ui.LightBarButton
import com.thelightphone.sdk.ui.LightBottomBar
import com.thelightphone.sdk.ui.LightIcons
import com.thelightphone.sdk.ui.LightText
import com.thelightphone.sdk.ui.LightTextVariant
import com.thelightphone.sdk.ui.LightTheme
import com.thelightphone.sdk.ui.LightThemeController
import com.thelightphone.sdk.ui.LightThemeTokens
import com.thelightphone.sdk.ui.LightTopBar
import com.thelightphone.sdk.ui.LightTopBarCenter
import com.thelightphone.sdk.ui.lightClickable
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/** Testable config boundary: a missing setup cannot construct or call a bridge client. */
internal class ConfiguredBridgeLoader(
    private val configStore: BridgeConfigStore,
    private val clientFactory: BridgeClientFactory,
) {
    suspend fun load(): BridgeApi? = configStore.load()?.let(clientFactory::create)
}

class HomeScreenViewModel(
    private val configStore: BridgeConfigStore,
    private val clientFactory: BridgeClientFactory = DefaultBridgeClientFactory,
) : LightViewModel<Unit>() {
    sealed interface State {
        data object LoadingConfiguration : State
        data object NeedsConfiguration : State
        data object Loading : State
        data class Ready(val conversations: List<Conversation>, val bridge: BridgeApi) : State
        data class Error(val message: String, val needsReconfigure: Boolean) : State
    }

    private val _state = MutableStateFlow<State>(State.LoadingConfiguration)
    val state: StateFlow<State> = _state
    private val configLoader = ConfiguredBridgeLoader(configStore, clientFactory)
    private val refreshMutex = Mutex()
    private var pollingJob: Job? = null
    private var active = false
    private var bridge: BridgeApi? = null

    override fun onScreenShow(screen: SimpleLightScreen<Unit>) {
        super.onScreenShow(screen)
        active = true
        if (bridge == null) loadConfiguration() else {
            refresh()
            startPolling()
        }
    }

    override fun onScreenHide(screen: SimpleLightScreen<Unit>) {
        active = false
        stopPolling()
        super.onScreenHide(screen)
    }

    override fun onAppPause() {
        active = false
        stopPolling()
        super.onAppPause()
    }

    override fun onCleared() {
        stopPolling()
        bridge?.close()
        bridge = null
        super.onCleared()
    }

    fun refresh() {
        val configuredBridge = bridge ?: run {
            loadConfiguration()
            return
        }
        viewModelScope.launch(Dispatchers.IO) {
            refreshMutex.withLock {
                if (bridge === configuredBridge) fetch(configuredBridge, showLoading = true)
            }
        }
    }

    fun configurationChanged() {
        loadConfiguration(replaceCurrent = true)
    }

    private fun loadConfiguration(replaceCurrent: Boolean = false) {
        stopPolling()
        viewModelScope.launch(Dispatchers.IO) {
            refreshMutex.withLock {
                if (replaceCurrent) {
                    bridge?.close()
                    bridge = null
                }
                if (bridge != null) return@withLock

                _state.value = State.LoadingConfiguration
                val configuredBridge = configLoader.load()
                if (configuredBridge == null) {
                    _state.value = State.NeedsConfiguration
                    return@withLock
                }
                bridge = configuredBridge
                fetch(configuredBridge, showLoading = true)
                if (active) startPolling()
            }
        }
    }

    private fun startPolling() {
        pollingJob?.cancel()
        pollingJob = viewModelScope.launch(Dispatchers.IO) {
            while (isActive) {
                delay(30_000)
                val configuredBridge = bridge ?: break
                refreshMutex.withLock {
                    if (bridge === configuredBridge) fetch(configuredBridge, showLoading = false)
                }
            }
        }
    }

    private fun stopPolling() {
        pollingJob?.cancel()
        pollingJob = null
    }

    private suspend fun fetch(configuredBridge: BridgeApi, showLoading: Boolean) {
        if (showLoading) _state.value = State.Loading
        try {
            _state.value = State.Ready(configuredBridge.conversations(), configuredBridge)
        } catch (error: Exception) {
            if (showLoading || _state.value !is State.Ready) {
                _state.value = State.Error(
                    message = if (isBridgeUnauthorized(error)) {
                        "The saved API token was rejected. Reconfigure the bridge."
                    } else {
                        error.message ?: "Bridge unavailable"
                    },
                    needsReconfigure = isBridgeUnauthorized(error),
                )
            }
        }
    }

}

@InitialScreen
class HomeScreen(sealedActivity: SealedLightActivity) : LightScreen<Unit, HomeScreenViewModel>(sealedActivity) {
    override val viewModelClass = HomeScreenViewModel::class.java
    override fun createViewModel() = HomeScreenViewModel(BridgeConfigStore.from(lightContext))

    @Composable
    override fun Content() {
        val state by viewModel.state.collectAsState()
        val colors by LightThemeController.colors.collectAsState()
        LightTheme(colors = colors) {
            Column(modifier = Modifier.fillMaxSize().background(LightThemeTokens.colors.background)) {
                LightTopBar(
                    center = LightTopBarCenter.Text("WhatsApp"),
                    rightButton = LightBarButton.LightIcon(LightIcons.REFRESH, onClick = viewModel::refresh),
                )
                when (val current = state) {
                    HomeScreenViewModel.State.LoadingConfiguration -> LightText(
                        "Loading setup…", LightTextVariant.Copy, modifier = Modifier.padding(32.dp),
                    )
                    HomeScreenViewModel.State.NeedsConfiguration -> SetupPrompt()
                    HomeScreenViewModel.State.Loading -> LightText(
                        "Syncing…", LightTextVariant.Copy, modifier = Modifier.padding(32.dp),
                    )
                    is HomeScreenViewModel.State.Error -> ErrorPrompt(current, ::openSetup, viewModel::refresh)
                    is HomeScreenViewModel.State.Ready -> Conversations(current)
                }
                when (val current = state) {
                    is HomeScreenViewModel.State.Ready -> {
                        val conversations = current.conversations
                        LightBottomBar(
                            items = listOf(
                                LightBarButton.LightIcon(
                                    icon = LightIcons.SEARCH,
                                    contentDescription = "Search chats",
                                    onClick = {
                                        navigateTo(
                                            screenFactory = { ConversationSearchEditorScreen(it, "") },
                                            resultCallback = { query ->
                                                if (query.isNotBlank()) {
                                                    navigateTo(
                                                        screenFactory = {
                                                            ConversationSearchScreen(it, conversations, query, current.bridge)
                                                        },
                                                    )
                                                }
                                            },
                                        )
                                    },
                                ),
                                LightBarButton.Text("SETUP", onClick = ::openSetup),
                            ),
                        )
                    }
                    HomeScreenViewModel.State.NeedsConfiguration -> LightBottomBar(
                        items = listOf(LightBarButton.Text("SET UP", onClick = ::openSetup)),
                    )
                    else -> Unit
                }
            }
        }
    }

    private fun openSetup() {
        navigateTo(
            screenFactory = { BridgeSetupScreen(it, BridgeConfigStore.from(lightContext)) },
            resultCallback = { viewModel.configurationChanged() },
        )
    }

    @Composable
    private fun SetupPrompt() {
        Column(modifier = Modifier.padding(32.dp)) {
            LightText("Connect WhatsApp", LightTextVariant.Heading)
            LightText(
                "Scan the setup QR code from your bridge, or use manual setup.",
                LightTextVariant.Detail,
                lighten = true,
                modifier = Modifier.padding(top = 12.dp),
            )
        }
    }

    @Composable
    private fun ErrorPrompt(
        state: HomeScreenViewModel.State.Error,
        onSetup: () -> Unit,
        onRetry: () -> Unit,
    ) {
        Column(modifier = Modifier.padding(32.dp)) {
            LightText("Can't reach WhatsApp", LightTextVariant.Heading)
            LightText(state.message, LightTextVariant.Detail, lighten = true, modifier = Modifier.padding(top = 12.dp))
            LightText(
                if (state.needsReconfigure) "RECONFIGURE" else "RETRY",
                LightTextVariant.Button,
                modifier = Modifier.padding(top = 28.dp).lightClickable(onClick = if (state.needsReconfigure) onSetup else onRetry),
            )
        }
    }

    @Composable
    private fun ColumnScope.Conversations(state: HomeScreenViewModel.State.Ready) {
        if (state.conversations.isEmpty()) {
            LightText(
                "No direct-message history yet.",
                LightTextVariant.Copy,
                lighten = true,
                modifier = Modifier.padding(32.dp),
            )
        } else {
            LazyColumn(modifier = Modifier.weight(1f).fillMaxWidth()) {
                items(state.conversations, key = { it.id }) { conversation ->
                    Column(
                        modifier = Modifier.fillMaxWidth().lightClickable {
                            navigateTo(screenFactory = { ChatScreen(it, conversation, state.bridge) })
                        }.padding(horizontal = 32.dp, vertical = 18.dp),
                    ) {
                        LightText(
                            conversation.displayName,
                            LightTextVariant.Copy,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                        )
                        if (conversation.kind == "group") {
                            LightText(
                                if (conversation.pinned) "PINNED GROUP" else "GROUP",
                                LightTextVariant.Superfine,
                                lighten = true,
                                modifier = Modifier.padding(top = 2.dp),
                            )
                        }
                        LightText(
                            conversation.lastMessage.ifBlank { "No text messages" },
                            LightTextVariant.Detail,
                            lighten = true,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                            modifier = Modifier.padding(top = 4.dp),
                        )
                    }
                }
            }
        }
    }
}
