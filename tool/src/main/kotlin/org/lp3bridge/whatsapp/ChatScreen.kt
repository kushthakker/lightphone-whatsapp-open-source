package org.lp3bridge.whatsapp

import android.media.AudioAttributes
import android.media.MediaDataSource
import android.media.MediaPlayer
import android.graphics.BitmapFactory
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.draw.clipToBounds
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewModelScope
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
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter

private val messageTimeFormatter = DateTimeFormatter.ofPattern("dd MMM · h:mm a")

private fun decodeBridgeImage(bytes: ByteArray): ImageBitmap? =
    BitmapFactory.decodeByteArray(bytes, 0, bytes.size)?.asImageBitmap()

internal fun formatMessageTime(timestamp: Long, zoneId: ZoneId = ZoneId.systemDefault()): String =
    Instant.ofEpochSecond(timestamp).atZone(zoneId).format(messageTimeFormatter)

internal fun reconcileMessages(
    current: List<Message>,
    incoming: List<Message>,
    limit: Int = 150,
): List<Message> {
    val byId = linkedMapOf<String, Message>()
    current.forEach { byId[it.id] = it }
    incoming.forEach { byId[it.id] = it }
    return byId.values
        .sortedWith(compareBy<Message> { it.timestamp }.thenBy { it.id })
        .takeLast(limit)
}

class ChatViewModel(
    private val conversation: Conversation,
    private val bridge: BridgeApi,
) : LightViewModel<Unit>() {
    data class State(
        val loading: Boolean = true,
        val messages: List<Message> = emptyList(),
        val images: Map<String, ImageBitmap> = emptyMap(),
        val loadedMediaIds: Set<String> = emptySet(),
        val lastSentMessageId: String? = null,
        val playingMessageId: String? = null,
        val error: String? = null,
    )

    private val _state = MutableStateFlow(State())
    val state: StateFlow<State> = _state
    private val refreshMutex = Mutex()
    private var pollingJob: Job? = null
    private var mediaPlayer: MediaPlayer? = null

    override fun onScreenShow(screen: SimpleLightScreen<Unit>) {
        super.onScreenShow(screen)
        refresh()
        startPolling()
    }

    override fun onScreenHide(screen: SimpleLightScreen<Unit>) {
        pollingJob?.cancel()
        pollingJob = null
        super.onScreenHide(screen)
    }

    override fun onAppPause() {
        pollingJob?.cancel()
        pollingJob = null
        super.onAppPause()
    }

    override fun onCleared() {
        releasePlayer()
        super.onCleared()
    }

    fun refresh() {
        viewModelScope.launch(Dispatchers.IO) {
            load(showLoading = true)
        }
    }

    private fun startPolling() {
        pollingJob?.cancel()
        pollingJob = viewModelScope.launch(Dispatchers.IO) {
            while (isActive) {
                delay(15_000)
                load(showLoading = false)
            }
        }
    }

    private suspend fun load(showLoading: Boolean) = refreshMutex.withLock {
        if (showLoading) _state.update { it.copy(loading = true, error = null) }
        try {
            val messages = bridge.messages(conversation.id)
            _state.update { current ->
                current.copy(
                    loading = false,
                    messages = reconcileMessages(current.messages, messages),
                    error = null,
                )
            }
            bridge.markRead(conversation.id)
            loadImages(messages)
        } catch (error: Exception) {
            _state.update { it.copy(loading = false, error = error.message ?: "Sync failed") }
        }
    }

    private suspend fun loadImages(messages: List<Message>) {
        val pending = messages
            .filter { it.mediaType == "image" && it.id !in _state.value.loadedMediaIds }
            .takeLast(20)
        for (message in pending) {
            val image = runCatching { decodeBridgeImage(bridge.media(message.id)) }.getOrNull()
            _state.update { current ->
                current.copy(
                    images = image?.let { current.images + (message.id to it) } ?: current.images,
                    loadedMediaIds = current.loadedMediaIds + message.id,
                )
            }
        }
    }

    fun send(text: String) {
        val trimmed = text.trim()
        if (trimmed.isEmpty()) return
        viewModelScope.launch(Dispatchers.IO) {
            try {
                val message = bridge.send(conversation.id, trimmed)
                _state.update { current ->
                    current.copy(
                        messages = reconcileMessages(current.messages, listOf(message)),
                        lastSentMessageId = message.id,
                        error = null,
                    )
                }
            } catch (error: Exception) {
                _state.update { it.copy(error = error.message ?: "Send failed") }
            }
        }
    }

    fun sendVoice(note: VoiceNote) {
        viewModelScope.launch(Dispatchers.IO) {
            try {
                val message = bridge.sendVoice(conversation.id, note.audio, note.durationSeconds)
                _state.update { current ->
                    current.copy(
                        messages = reconcileMessages(current.messages, listOf(message)),
                        lastSentMessageId = message.id,
                        error = null,
                    )
                }
            } catch (error: Exception) {
                _state.update { it.copy(error = error.message ?: "Voice note send failed") }
            }
        }
    }

    fun toggleVoice(messageId: String) {
        if (_state.value.playingMessageId == messageId) {
            releasePlayer()
            return
        }
        releasePlayer()
        _state.update { it.copy(playingMessageId = messageId, error = null) }
        viewModelScope.launch(Dispatchers.IO) {
            try {
                val audio = bridge.media(messageId)
                if (_state.value.playingMessageId != messageId) return@launch
                withContext(Dispatchers.Main) {
                    if (_state.value.playingMessageId != messageId) return@withContext
                    val player = MediaPlayer().apply {
                        setAudioAttributes(
                            AudioAttributes.Builder()
                                .setContentType(AudioAttributes.CONTENT_TYPE_SPEECH)
                                .setUsage(AudioAttributes.USAGE_MEDIA)
                                .build(),
                        )
                    }
                    mediaPlayer = player
                    player.setDataSource(ByteArrayMediaDataSource(audio))
                    player.setOnPreparedListener { prepared ->
                        if (mediaPlayer === prepared) prepared.start()
                    }
                    player.setOnCompletionListener { completed ->
                        if (mediaPlayer === completed) releasePlayer()
                    }
                    player.setOnErrorListener { failed, _, _ ->
                        if (mediaPlayer === failed) releasePlayer("Voice note could not be played")
                        true
                    }
                    player.prepareAsync()
                }
            } catch (error: Exception) {
                if (_state.value.playingMessageId == messageId) {
                    releasePlayer(error.message ?: "Voice note could not be played")
                }
            }
        }
    }

    private fun releasePlayer(error: String? = null) {
        val player = mediaPlayer
        mediaPlayer = null
        runCatching { player?.stop() }
        player?.release()
        _state.update { current ->
            current.copy(playingMessageId = null, error = error ?: current.error)
        }
    }
}

private class ByteArrayMediaDataSource(private val bytes: ByteArray) : MediaDataSource() {
    override fun getSize(): Long = bytes.size.toLong()

    override fun readAt(position: Long, buffer: ByteArray, offset: Int, size: Int): Int {
        if (position >= bytes.size) return -1
        val start = position.toInt()
        val length = minOf(size, bytes.size - start)
        bytes.copyInto(buffer, offset, start, start + length)
        return length
    }

    override fun close() = Unit
}

class ChatScreen(
    sealedActivity: SealedLightActivity,
    private val conversation: Conversation,
    private val bridge: BridgeApi,
) : LightScreen<Unit, ChatViewModel>(sealedActivity) {
    override val viewModelClass = ChatViewModel::class.java
    override fun createViewModel() = ChatViewModel(conversation, bridge)

    @Composable
    override fun Content() {
        val state by viewModel.state.collectAsState()
        val colors by LightThemeController.colors.collectAsState()
        val messageListState = rememberLazyListState()
        LaunchedEffect(state.lastSentMessageId) {
            if (state.lastSentMessageId != null) messageListState.animateScrollToItem(0)
        }
        LightTheme(colors = colors) {
            Column(modifier = Modifier.fillMaxSize().background(LightThemeTokens.colors.background)) {
                LightTopBar(
                    leftButton = LightBarButton.LightIcon(LightIcons.BACK, onClick = { goBack() }),
                    center = LightTopBarCenter.Text(conversation.displayName),
                    rightButton = LightBarButton.LightIcon(LightIcons.REFRESH, onClick = viewModel::refresh),
                )
                when {
                    state.loading && state.messages.isEmpty() -> LightText("Syncing…", LightTextVariant.Copy, modifier = Modifier.padding(32.dp).weight(1f))
                    else -> LazyColumn(
                        state = messageListState,
                        modifier = Modifier.weight(1f).fillMaxWidth().padding(horizontal = 24.dp),
                        verticalArrangement = Arrangement.spacedBy(18.dp),
						reverseLayout = true,
                    ) {
                        items(state.messages.asReversed(), key = { it.id }) { message ->
                            Column(modifier = Modifier.fillMaxWidth()) {
                                LightText(
                                    when {
                                        message.fromMe -> "YOU"
                                        conversation.kind == "group" -> message.senderName.ifBlank { "UNKNOWN SENDER" }.uppercase()
                                        else -> conversation.displayName.uppercase()
                                    },
                                    LightTextVariant.Superfine,
                                    lighten = true,
                                    align = if (message.fromMe) TextAlign.End else TextAlign.Start,
                                    modifier = Modifier.fillMaxWidth(),
                                )
                                state.images[message.id]?.let { image ->
                                    Image(
                                        bitmap = image,
                                        contentDescription = "WhatsApp image",
                                        contentScale = ContentScale.Fit,
                                        modifier = Modifier
                                            .fillMaxWidth()
                                            .height(260.dp)
                                            .clipToBounds(),
                                    )
                                }
                                if (message.mediaType != "image" || message.text != "[Image]") {
                                    if (message.mediaType == "voice" || message.mediaType == "audio") {
                                        val playing = state.playingMessageId == message.id
                                        LightText(
                                            text = "${if (playing) "STOP" else "PLAY"} · ${formatVoiceDuration(message.mediaDuration)}",
                                            variant = LightTextVariant.Paragraph,
                                            align = if (message.fromMe) TextAlign.End else TextAlign.Start,
                                            modifier = Modifier.fillMaxWidth().padding(top = 3.dp).lightClickable {
                                                viewModel.toggleVoice(message.id)
                                            },
                                        )
                                    } else {
                                        LightText(
                                            message.text,
                                            LightTextVariant.Paragraph,
                                            align = if (message.fromMe) TextAlign.End else TextAlign.Start,
                                            modifier = Modifier.fillMaxWidth().padding(top = 3.dp),
                                        )
                                    }
                                }
                                LightText(
                                    formatMessageTime(message.timestamp),
                                    LightTextVariant.Superfine,
                                    lighten = true,
                                    align = if (message.fromMe) TextAlign.End else TextAlign.Start,
                                    modifier = Modifier.fillMaxWidth().padding(top = 3.dp),
                                )
                            }
                        }
						item { state.error?.let { LightText(it, LightTextVariant.Detail, lighten = true, modifier = Modifier.padding(vertical = 12.dp)) } }
                    }
                }
                LightBottomBar(
                    items = listOf(
                        LightBarButton.Text("MESSAGE", onClick = {
                            navigateTo({ ComposerScreen(it, conversation.displayName) }, viewModel::send)
                        }),
                        LightBarButton.Text("VOICE", onClick = {
                            navigateTo({ VoiceRecorderScreen(it, conversation.displayName) }, viewModel::sendVoice)
                        }),
                    ),
                )
            }
        }
    }
}

internal fun formatVoiceDuration(seconds: Int): String {
    val bounded = seconds.coerceAtLeast(0)
    return "%d:%02d".format(bounded / 60, bounded % 60)
}
