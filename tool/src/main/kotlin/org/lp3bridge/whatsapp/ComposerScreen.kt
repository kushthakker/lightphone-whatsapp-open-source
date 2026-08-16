package org.lp3bridge.whatsapp

import androidx.compose.foundation.text.input.rememberTextFieldState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import com.thelightphone.sdk.LightScreen
import com.thelightphone.sdk.LightViewModel
import com.thelightphone.sdk.SealedLightActivity
import com.thelightphone.sdk.ui.LightIcons
import com.thelightphone.sdk.ui.LightTextInputEditor
import com.thelightphone.sdk.ui.LightTheme
import com.thelightphone.sdk.ui.LightThemeController
import com.thelightphone.sdk.ui.defaultKeyboardOptions
import kotlinx.coroutines.flow.MutableStateFlow

class ComposerViewModel : LightViewModel<String>() {
    val keyboardOptions = MutableStateFlow(defaultKeyboardOptions())
}

class ComposerScreen(sealedActivity: SealedLightActivity, private val recipient: String) : LightScreen<String, ComposerViewModel>(sealedActivity) {
    override val viewModelClass = ComposerViewModel::class.java
    override fun createViewModel() = ComposerViewModel()

    @Composable
    override fun Content() {
        val colors by LightThemeController.colors.collectAsState()
        val text = rememberTextFieldState()
        LightTheme(colors = colors) {
            LightTextInputEditor(
                title = recipient,
                state = text,
                keyboardOptionsFlow = viewModel.keyboardOptions,
                submitLabel = "SEND",
                submitIcon = LightIcons.SEND,
                onSubmit = { value ->
                    val message = value.toString().trim()
                    if (message.isNotEmpty()) goBack(message)
                },
                onBack = { goBack() },
            )
        }
    }
}
