package org.lp3bridge.whatsapp

import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import com.thelightphone.sdk.SealedLightContext
import java.net.URI
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import kotlinx.coroutines.flow.first
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json

private const val QR_PAYLOAD_TYPE = "org.lp3bridge.whatsapp.config"
private const val QR_PAYLOAD_VERSION = 1
private const val KEYSTORE_PROVIDER = "AndroidKeyStore"
private const val KEY_ALIAS = "org.lp3bridge.whatsapp.bridge-token.v1"
private const val CIPHER_TRANSFORMATION = "AES/GCM/NoPadding"
private const val CIPHERTEXT_VERSION = "v1"

/**
 * A QR payload is one JSON object with this exact shape:
 * {"type":"org.lp3bridge.whatsapp.config","version":1,"baseUrl":"https://bridge.example","apiToken":"at-least-32-characters"}
 */
const val QR_PAYLOAD_FORMAT =
    "JSON: type=org.lp3bridge.whatsapp.config, version=1, baseUrl=https URL, apiToken=32+ chars"

data class BridgeConfig(
    val baseUrl: String,
    val apiToken: String,
)

fun bridgeConfig(baseUrl: String, apiToken: String): BridgeConfig = BridgeConfig(
    baseUrl = normalizeBridgeBaseUrl(baseUrl),
    apiToken = validateApiToken(apiToken),
)

fun normalizeBridgeBaseUrl(value: String): String {
    val input = value.trim()
    require(input.isNotEmpty()) { "Bridge URL is required" }
    val uri = try {
        URI(input)
    } catch (_: Exception) {
        throw IllegalArgumentException("Bridge URL is invalid")
    }
    require(uri.scheme.equals("https", ignoreCase = true)) { "Bridge URL must use HTTPS" }
    require(!uri.host.isNullOrBlank()) { "Bridge URL must include a host" }
    require(uri.userInfo == null) { "Bridge URL must not include credentials" }
    require(uri.path.isNullOrEmpty() || uri.path == "/") { "Bridge URL must not include a path" }
    require(uri.query == null && uri.fragment == null) { "Bridge URL must not include a query or fragment" }

    return input.trimEnd('/').also {
        require(it.length > "https://".length) { "Bridge URL must include a host" }
    }
}

fun validateApiToken(value: String): String {
    val token = value.trim()
    require(token.length >= 32) { "API token must be at least 32 characters" }
    return token
}

/** Pure masking helper for logs/tests. The UI uses [savedTokenMarker] instead, so it never decrypts to render. */
fun maskApiToken(value: String): String = "••••${value.takeLast(4)}"

fun savedTokenMarker(): String = "Saved"

@Serializable
private data class QrBridgeConfigPayload(
    val type: String,
    val version: Int,
    val baseUrl: String,
    val apiToken: String,
)

fun parseQrBridgeConfig(payload: String): Result<BridgeConfig> = runCatching {
    require(payload.length <= 8_192) { "QR setup payload is too large" }
    val parsed = Json { ignoreUnknownKeys = false }
        .decodeFromString<QrBridgeConfigPayload>(payload)
    require(parsed.type == QR_PAYLOAD_TYPE && parsed.version == QR_PAYLOAD_VERSION) {
        "This is not an LP3 Bridge v1 setup QR code"
    }
    bridgeConfig(parsed.baseUrl, parsed.apiToken)
}

interface TokenCipher {
    fun encrypt(plaintext: String): String
    fun decrypt(ciphertext: String): String
}

/** Android Keystore-backed AES/GCM cipher. Only nonce+ciphertext are persisted. */
class AndroidKeystoreTokenCipher : TokenCipher {
    override fun encrypt(plaintext: String): String {
        val cipher = Cipher.getInstance(CIPHER_TRANSFORMATION).apply {
            init(Cipher.ENCRYPT_MODE, key())
        }
        val ciphertext = cipher.doFinal(plaintext.encodeToByteArray())
        return listOf(CIPHERTEXT_VERSION, encode(cipher.iv), encode(ciphertext)).joinToString(":")
    }

    override fun decrypt(ciphertext: String): String {
        val parts = ciphertext.split(":")
        require(parts.size == 3 && parts[0] == CIPHERTEXT_VERSION) { "Stored API token is invalid" }
        val cipher = Cipher.getInstance(CIPHER_TRANSFORMATION).apply {
            init(Cipher.DECRYPT_MODE, key(), javax.crypto.spec.GCMParameterSpec(128, decode(parts[1])))
        }
        return cipher.doFinal(decode(parts[2])).decodeToString()
    }

    private fun key(): SecretKey {
        val store = KeyStore.getInstance(KEYSTORE_PROVIDER).apply { load(null) }
        (store.getKey(KEY_ALIAS, null) as? SecretKey)?.let { return it }
        return KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, KEYSTORE_PROVIDER).apply {
            init(
                KeyGenParameterSpec.Builder(
                    KEY_ALIAS,
                    KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT,
                )
                    .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                    .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                    .build(),
            )
        }.generateKey()
    }

    private fun encode(bytes: ByteArray): String = Base64.encodeToString(bytes, Base64.NO_WRAP)

    private fun decode(value: String): ByteArray = Base64.decode(value, Base64.NO_WRAP)
}

data class StoredBridgeConfig(val baseUrl: String, val encryptedToken: String)

interface BridgeConfigStorage {
    suspend fun read(): StoredBridgeConfig?
    suspend fun replace(config: StoredBridgeConfig)
    suspend fun clear()
}

private val bridgeBaseUrlKey = stringPreferencesKey("lp3_bridge_base_url")
private val bridgeTokenCiphertextKey = stringPreferencesKey("lp3_bridge_api_token_ciphertext")

/** Preferences DataStore adapter; URL and ciphertext are replaced in one edit transaction. */
class PreferencesBridgeConfigStorage(
    private val dataStore: DataStore<Preferences>,
) : BridgeConfigStorage {
    override suspend fun read(): StoredBridgeConfig? {
        val preferences = dataStore.data.first()
        val baseUrl = preferences[bridgeBaseUrlKey] ?: return null
        val encryptedToken = preferences[bridgeTokenCiphertextKey] ?: return null
        return StoredBridgeConfig(baseUrl, encryptedToken)
    }

    override suspend fun replace(config: StoredBridgeConfig) {
        dataStore.edit { preferences ->
            preferences[bridgeBaseUrlKey] = config.baseUrl
            preferences[bridgeTokenCiphertextKey] = config.encryptedToken
        }
    }

    override suspend fun clear() {
        dataStore.edit { preferences ->
            preferences.remove(bridgeBaseUrlKey)
            preferences.remove(bridgeTokenCiphertextKey)
        }
    }
}

data class BridgeConfigSummary(val baseUrl: String, val tokenMarker: String)

/**
 * Runtime configuration repository. Its production factory is deliberately restricted to the
 * SDK-provided [SealedLightContext.dataStore]; no Android Context or alternate persistence is used.
 */
class BridgeConfigStore(
    private val storage: BridgeConfigStorage,
    private val tokenCipher: TokenCipher,
) {
    companion object {
        fun from(lightContext: SealedLightContext): BridgeConfigStore = BridgeConfigStore(
            storage = PreferencesBridgeConfigStorage(lightContext.dataStore),
            tokenCipher = AndroidKeystoreTokenCipher(),
        )
    }

    suspend fun load(): BridgeConfig? {
        val stored = storage.read() ?: return null
        return runCatching {
            bridgeConfig(stored.baseUrl, tokenCipher.decrypt(stored.encryptedToken))
        }.getOrNull()
    }

    /** Lets settings show the URL and a marker without decrypting the API token. */
    suspend fun summary(): BridgeConfigSummary? {
        val stored = storage.read() ?: return null
        val baseUrl = runCatching { normalizeBridgeBaseUrl(stored.baseUrl) }.getOrNull() ?: return null
        return BridgeConfigSummary(baseUrl, savedTokenMarker())
    }

    suspend fun save(config: BridgeConfig) {
        val validated = bridgeConfig(config.baseUrl, config.apiToken)
        storage.replace(
            StoredBridgeConfig(
                baseUrl = validated.baseUrl,
                encryptedToken = tokenCipher.encrypt(validated.apiToken),
            ),
        )
    }

    suspend fun replace(config: BridgeConfig) = save(config)

    suspend fun clear() = storage.clear()
}
