package org.lp3bridge.whatsapp

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.text.input.rememberTextFieldState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.thelightphone.sdk.LightScreen
import com.thelightphone.sdk.LightViewModel
import com.thelightphone.sdk.SealedLightActivity
import com.thelightphone.sdk.ui.LightBarButton
import com.thelightphone.sdk.ui.LightIcons
import com.thelightphone.sdk.ui.LightText
import com.thelightphone.sdk.ui.LightTextField
import com.thelightphone.sdk.ui.LightTextInputEditor
import com.thelightphone.sdk.ui.LightTextVariant
import com.thelightphone.sdk.ui.LightTheme
import com.thelightphone.sdk.ui.LightThemeController
import com.thelightphone.sdk.ui.LightThemeTokens
import com.thelightphone.sdk.ui.LightTopBar
import com.thelightphone.sdk.ui.LightTopBarCenter
import com.thelightphone.sdk.ui.defaultKeyboardOptions
import com.thelightphone.sdk.ui.lightClickable
import kotlinx.coroutines.flow.MutableStateFlow

internal fun filterConversations(
    conversations: List<Conversation>,
    query: String,
): List<Conversation> {
    val normalizedQuery = query.trim()
    if (normalizedQuery.isEmpty()) return emptyList()

    return conversations
        .filter { it.displayName.contains(normalizedQuery, ignoreCase = true) }
        .sortedWith(
            compareByDescending<Conversation> {
                it.displayName.startsWith(normalizedQuery, ignoreCase = true)
            }.thenByDescending { it.pinned }
                .thenByDescending { it.lastMessageAt }
                .thenBy { it.displayName.lowercase() },
        )
}

class ConversationSearchViewModel : LightViewModel<Unit>() {
    val query = MutableStateFlow("")

    fun setQuery(value: String) {
        query.value = value.trim()
    }
}

class ConversationSearchScreen(
    sealedActivity: SealedLightActivity,
    private val conversations: List<Conversation>,
    private val initialQuery: String,
    private val bridge: BridgeApi,
) : LightScreen<Unit, ConversationSearchViewModel>(sealedActivity) {
    override val viewModelClass = ConversationSearchViewModel::class.java
    override fun createViewModel() = ConversationSearchViewModel()

    @Composable
    override fun Content() {
        val colors by LightThemeController.colors.collectAsState()
        val query by viewModel.query.collectAsState()
        val results = remember(conversations, query) { filterConversations(conversations, query) }
        LaunchedEffect(initialQuery) {
            if (query.isBlank()) viewModel.setQuery(initialQuery)
        }

        LightTheme(colors = colors) {
            Column(modifier = Modifier.fillMaxSize().background(LightThemeTokens.colors.background)) {
                LightTopBar(
                    leftButton = LightBarButton.LightIcon(LightIcons.BACK, onClick = { goBack() }),
                    center = LightTopBarCenter.Text("Find chat"),
                )
                LightTextField(
                    label = "Contact or group",
                    value = query,
                    placeholder = "Search chats",
                    onClick = {
                        navigateTo(
                            screenFactory = { ConversationSearchEditorScreen(it, query) },
                            resultCallback = viewModel::setQuery,
                        )
                    },
                    modifier = Modifier.padding(horizontal = 32.dp, vertical = 16.dp),
                )

                when {
                    query.isBlank() -> LightText(
                        "Enter a contact or group name.",
                        LightTextVariant.Detail,
                        lighten = true,
                        modifier = Modifier.padding(horizontal = 32.dp, vertical = 16.dp),
                    )
                    results.isEmpty() -> LightText(
                        "No chats found.",
                        LightTextVariant.Copy,
                        modifier = Modifier.padding(horizontal = 32.dp, vertical = 16.dp),
                    )
                    else -> LazyColumn(modifier = Modifier.weight(1f).fillMaxWidth()) {
                        items(results, key = { it.id }) { conversation ->
                            Column(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .lightClickable {
                                        navigateTo(screenFactory = { ChatScreen(it, conversation, bridge) })
                                    }
                                    .padding(horizontal = 32.dp, vertical = 18.dp),
                            ) {
                                LightText(
                                    conversation.displayName,
                                    LightTextVariant.Copy,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis,
                                )
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
    }
}

class ConversationSearchEditorViewModel : LightViewModel<String>() {
    val keyboardOptions = MutableStateFlow(defaultKeyboardOptions())
}

class ConversationSearchEditorScreen(
    sealedActivity: SealedLightActivity,
    private val initialQuery: String,
) : LightScreen<String, ConversationSearchEditorViewModel>(sealedActivity) {
    override val viewModelClass = ConversationSearchEditorViewModel::class.java
    override fun createViewModel() = ConversationSearchEditorViewModel()

    @Composable
    override fun Content() {
        val colors by LightThemeController.colors.collectAsState()
        val text = rememberTextFieldState(initialQuery)

        LightTheme(colors = colors) {
            LightTextInputEditor(
                title = "Search chats",
                state = text,
                keyboardOptionsFlow = viewModel.keyboardOptions,
                submitLabel = "SEARCH",
                submitIcon = LightIcons.SEARCH,
                onSubmit = { goBack(it.toString().trim()) },
                onBack = { goBack() },
            )
        }
    }
}
