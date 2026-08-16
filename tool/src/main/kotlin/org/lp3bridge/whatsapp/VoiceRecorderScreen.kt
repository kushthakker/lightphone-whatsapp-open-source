package org.lp3bridge.whatsapp

import android.Manifest
import android.media.AudioAttributes
import android.media.MediaPlayer
import android.media.MediaRecorder
import android.os.SystemClock
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewModelScope
import com.thelightphone.sdk.LightScreen
import com.thelightphone.sdk.LightViewModel
import com.thelightphone.sdk.SealedLightActivity
import com.thelightphone.sdk.SimpleLightScreen
import com.thelightphone.sdk.checkPermission
import com.thelightphone.sdk.rememberPermissionRequestLauncher
import com.thelightphone.sdk.shared.LightServiceMethod
import com.thelightphone.sdk.shared.asKotlinResult
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
import java.io.File
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch

data class VoiceNote(val audio: ByteArray, val durationSeconds: Int)

class VoiceRecorderViewModel(private val filesDir: File) : LightViewModel<VoiceNote>() {
    enum class Stage { CHECKING_PERMISSION, NEEDS_PERMISSION, READY, RECORDING, RECORDED }

    data class State(
        val stage: Stage = Stage.CHECKING_PERMISSION,
        val elapsedSeconds: Int = 0,
        val playingPreview: Boolean = false,
        val error: String? = null,
    )

    private val _state = MutableStateFlow(State())
    val state: StateFlow<State> = _state
    private var recorder: MediaRecorder? = null
    private var player: MediaPlayer? = null
    private var recordingFile: File? = null
    private var recordingStartedAt = 0L
    private var timerJob: Job? = null

    override fun onScreenShow(screen: SimpleLightScreen<VoiceNote>) {
        super.onScreenShow(screen)
        if (_state.value.stage == Stage.CHECKING_PERMISSION || _state.value.stage == Stage.NEEDS_PERMISSION) {
            checkMicrophonePermission()
        }
    }

    override fun onAppPause() {
        if (_state.value.stage == Stage.RECORDING) stopRecording()
        stopPreview()
        super.onAppPause()
    }

    override fun onCleared() {
        if (_state.value.stage == Stage.RECORDING) runCatching { recorder?.stop() }
        releaseRecorder()
        stopPreview()
        recordingFile?.delete()
        super.onCleared()
    }

    fun checkMicrophonePermission() {
        viewModelScope.launch(Dispatchers.IO) {
            val granted = checkPermission(Manifest.permission.RECORD_AUDIO).asKotlinResult
                .getOrNull()?.permissionResult == LightServiceMethod.GetPermission.Result.Granted
            _state.value = _state.value.copy(
                stage = if (granted) Stage.READY else Stage.NEEDS_PERMISSION,
                error = null,
            )
        }
    }

    @Suppress("DEPRECATION")
    fun startRecording() {
        if (_state.value.stage != Stage.READY) return
        stopPreview()
        recordingFile?.delete()
        val file = File.createTempFile("voice-", ".ogg", filesDir)
        try {
            val newRecorder = MediaRecorder().apply {
                setAudioSource(MediaRecorder.AudioSource.MIC)
                setOutputFormat(MediaRecorder.OutputFormat.OGG)
                setAudioEncoder(MediaRecorder.AudioEncoder.OPUS)
                setAudioSamplingRate(48_000)
                setAudioChannels(1)
                setAudioEncodingBitRate(32_000)
                setOutputFile(file.absolutePath)
                prepare()
                start()
            }
            recordingFile = file
            recorder = newRecorder
            recordingStartedAt = SystemClock.elapsedRealtime()
            _state.value = State(stage = Stage.RECORDING)
            timerJob = viewModelScope.launch {
                while (isActive) {
                    val elapsed = ((SystemClock.elapsedRealtime() - recordingStartedAt) / 1_000L).toInt()
                    _state.value = _state.value.copy(elapsedSeconds = elapsed.coerceAtMost(120))
                    if (elapsed >= 120) {
                        stopRecording()
                        break
                    }
                    delay(250)
                }
            }
        } catch (error: Exception) {
            releaseRecorder()
            file.delete()
            _state.value = State(stage = Stage.READY, error = error.message ?: "Microphone could not start")
        }
    }

    fun stopRecording() {
        if (_state.value.stage != Stage.RECORDING) return
        timerJob?.cancel()
        timerJob = null
        val duration = ((SystemClock.elapsedRealtime() - recordingStartedAt + 999L) / 1_000L).toInt().coerceAtMost(120)
        val stopped = runCatching { recorder?.stop() }.isSuccess
        releaseRecorder()
        val file = recordingFile
        if (!stopped || duration < 1 || file == null || !file.isFile || file.length() == 0L) {
            file?.delete()
            recordingFile = null
            _state.value = State(stage = Stage.READY, error = "Record for at least one second")
            return
        }
        _state.value = State(stage = Stage.RECORDED, elapsedSeconds = duration)
    }

    fun togglePreview() {
        if (_state.value.playingPreview) {
            stopPreview()
            return
        }
        val file = recordingFile ?: return
        try {
            val newPlayer = MediaPlayer().apply {
                setAudioAttributes(
                    AudioAttributes.Builder()
                        .setContentType(AudioAttributes.CONTENT_TYPE_SPEECH)
                        .setUsage(AudioAttributes.USAGE_MEDIA)
                        .build(),
                )
                setDataSource(file.absolutePath)
                setOnCompletionListener { stopPreview() }
                setOnErrorListener { _, _, _ ->
                    stopPreview("Preview could not be played")
                    true
                }
                prepare()
                start()
            }
            player = newPlayer
            _state.value = _state.value.copy(playingPreview = true, error = null)
        } catch (error: Exception) {
            stopPreview(error.message ?: "Preview could not be played")
        }
    }

    fun retake() {
        stopPreview()
        recordingFile?.delete()
        recordingFile = null
        _state.value = State(stage = Stage.READY)
    }

    fun finish(): VoiceNote? {
        stopPreview()
        val file = recordingFile ?: return null
        val audio = runCatching { file.readBytes() }.getOrNull() ?: return null
        if (audio.isEmpty() || audio.size > 5 * 1024 * 1024) {
            _state.value = _state.value.copy(error = "Voice note is too large")
            return null
        }
        file.delete()
        recordingFile = null
        return VoiceNote(audio, _state.value.elapsedSeconds)
    }

    private fun releaseRecorder() {
        recorder?.release()
        recorder = null
    }

    private fun stopPreview(error: String? = null) {
        val current = player
        player = null
        runCatching { current?.stop() }
        current?.release()
        _state.value = _state.value.copy(playingPreview = false, error = error ?: _state.value.error)
    }
}

class VoiceRecorderScreen(
    sealedActivity: SealedLightActivity,
    private val conversationName: String,
) : LightScreen<VoiceNote, VoiceRecorderViewModel>(sealedActivity) {
    override val viewModelClass = VoiceRecorderViewModel::class.java
    override fun createViewModel() = VoiceRecorderViewModel(lightContext.filesDir)

    @Composable
    override fun Content() {
        val state by viewModel.state.collectAsState()
        val colors by LightThemeController.colors.collectAsState()
        val permissionLauncher = rememberPermissionRequestLauncher(Manifest.permission.RECORD_AUDIO)
        LightTheme(colors = colors) {
            Column(modifier = Modifier.fillMaxSize().background(LightThemeTokens.colors.background)) {
                LightTopBar(
                    leftButton = LightBarButton.LightIcon(LightIcons.BACK, onClick = { goBack() }),
                    center = LightTopBarCenter.Text("Voice to $conversationName"),
                )
                Column(
                    modifier = Modifier.weight(1f).fillMaxWidth().padding(32.dp),
                    horizontalAlignment = Alignment.CenterHorizontally,
                    verticalArrangement = Arrangement.Center,
                ) {
                    LightText(
                        text = when (state.stage) {
                            VoiceRecorderViewModel.Stage.CHECKING_PERMISSION -> "Checking microphone…"
                            VoiceRecorderViewModel.Stage.NEEDS_PERMISSION -> "Microphone permission is required."
                            VoiceRecorderViewModel.Stage.READY -> "Tap RECORD to begin. Maximum 2 minutes."
                            VoiceRecorderViewModel.Stage.RECORDING -> "Recording\n${formatVoiceDuration(state.elapsedSeconds)}"
                            VoiceRecorderViewModel.Stage.RECORDED -> "Recorded\n${formatVoiceDuration(state.elapsedSeconds)}"
                        },
                        variant = LightTextVariant.Heading,
                        align = TextAlign.Center,
                        modifier = Modifier.fillMaxWidth(),
                    )
                    state.error?.let {
                        LightText(it, LightTextVariant.Detail, lighten = true, align = TextAlign.Center, modifier = Modifier.fillMaxWidth().padding(top = 18.dp))
                    }
                }
                LightBottomBar(
                    items = when (state.stage) {
                        VoiceRecorderViewModel.Stage.CHECKING_PERMISSION -> emptyList()
                        VoiceRecorderViewModel.Stage.NEEDS_PERMISSION -> listOf(
                            LightBarButton.Text("ENABLE", onClick = { permissionLauncher?.launch() }),
                        )
                        VoiceRecorderViewModel.Stage.READY -> listOf(
                            LightBarButton.Text("RECORD", onClick = viewModel::startRecording),
                        )
                        VoiceRecorderViewModel.Stage.RECORDING -> listOf(
                            LightBarButton.Text("STOP", onClick = viewModel::stopRecording),
                        )
                        VoiceRecorderViewModel.Stage.RECORDED -> listOf(
                            LightBarButton.Text("RETAKE", onClick = viewModel::retake),
                            LightBarButton.Text(if (state.playingPreview) "STOP" else "PLAY", onClick = viewModel::togglePreview),
                            LightBarButton.Text("SEND", onClick = { viewModel.finish()?.let { goBack(it) } }),
                        )
                    },
                )
            }
        }
    }
}
