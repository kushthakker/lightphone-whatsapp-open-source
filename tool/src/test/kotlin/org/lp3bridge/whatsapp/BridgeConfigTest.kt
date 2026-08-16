package org.lp3bridge.whatsapp

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlinx.coroutines.runBlocking

class BridgeConfigTest {
    private val validToken = "test-token-0123456789abcdefghijklmnop"

    @Test
    fun `parses only the documented v1 QR payload`() {
        val result = parseQrBridgeConfig(
            """{"type":"org.lp3bridge.whatsapp.config","version":1,"baseUrl":"https://bridge.example/","apiToken":"$validToken"}""",
        )

        assertEquals("https://bridge.example", result.getOrThrow().baseUrl)
        assertEquals(validToken, result.getOrThrow().apiToken)
        assertTrue(parseQrBridgeConfig("""{"version":1}""").isFailure)
        assertTrue(
            parseQrBridgeConfig(
                """{"type":"org.lp3bridge.whatsapp.config","version":2,"baseUrl":"https://bridge.example","apiToken":"$validToken"}""",
            ).isFailure,
        )
    }

    @Test
    fun `validates bridge URLs and token length`() {
        assertEquals("https://bridge.example", normalizeBridgeBaseUrl("https://bridge.example/"))
        listOf(
            "http://bridge.example",
            "https://person@bridge.example",
            "https://bridge.example/path",
            "https://bridge.example?debug=true",
            "https://bridge.example#fragment",
        ).forEach { assertTrue(runCatching { normalizeBridgeBaseUrl(it) }.isFailure, it) }
        assertTrue(runCatching { validateApiToken("short") }.isFailure)
    }

    @Test
    fun `masks tokens without returning their full value`() {
        assertEquals("••••mnop", maskApiToken(validToken))
        assertEquals("Saved", savedTokenMarker())
    }

    @Test
    fun `config store atomically writes URL and ciphertext with a deterministic cipher fake`() = runBlocking {
        val storage = FakeStorage()
        val cipher = FakeCipher()
        val store = BridgeConfigStore(storage, cipher)
        val config = bridgeConfig("https://bridge.example/", validToken)

        store.save(config)

        assertEquals("https://bridge.example", storage.value?.baseUrl)
        assertEquals("enc:${validToken.reversed()}", storage.value?.encryptedToken)
        assertFalse(storage.value?.encryptedToken?.contains(validToken) == true)
        assertEquals(0, cipher.decryptCalls)
        assertEquals(BridgeConfigSummary("https://bridge.example", "Saved"), store.summary())
        assertEquals(0, cipher.decryptCalls)
        assertEquals(config, store.load())
        assertEquals(1, cipher.decryptCalls)
        store.clear()
        assertNull(storage.value)
    }

    @Test
    fun `missing config does not create a client or request the bridge`() = runBlocking {
        var factoryCalls = 0
        val loader = ConfiguredBridgeLoader(
            BridgeConfigStore(FakeStorage(), FakeCipher()),
            BridgeClientFactory { factoryCalls++ ; RecordingBridgeApi() },
        )

        assertNull(loader.load())
        assertEquals(0, factoryCalls)
    }

    @Test
    fun `verification calls health before authenticated status and surfaces 401 recovery`() = runBlocking {
        val calls = mutableListOf<String>()
        val client = RecordingBridgeApi(calls = calls, failStatus = true)
        val verifier = BridgeConfigurationVerifier(BridgeClientFactory { client })

        val failure = runCatching { verifier.verify(bridgeConfig("https://bridge.example", validToken)) }.exceptionOrNull()

        assertEquals(listOf("healthz", "status", "close"), calls)
        assertTrue(failure != null && isBridgeUnauthorized(failure))
    }

    private class FakeStorage : BridgeConfigStorage {
        var value: StoredBridgeConfig? = null

        override suspend fun read(): StoredBridgeConfig? = value

        override suspend fun replace(config: StoredBridgeConfig) {
            value = config
        }

        override suspend fun clear() {
            value = null
        }
    }

    private class FakeCipher : TokenCipher {
        var decryptCalls = 0

        override fun encrypt(plaintext: String): String = "enc:${plaintext.reversed()}"

        override fun decrypt(ciphertext: String): String {
            decryptCalls++
            return ciphertext.removePrefix("enc:").reversed()
        }
    }

    private class RecordingBridgeApi(
        private val calls: MutableList<String> = mutableListOf(),
        private val failStatus: Boolean = false,
    ) : BridgeApi {
        override suspend fun healthz() {
            calls += "healthz"
        }

        override suspend fun status() {
            calls += "status"
            if (failStatus) throw BridgeUnauthorizedException()
        }

        override suspend fun conversations(): List<Conversation> {
            calls += "conversations"
            return emptyList()
        }

        override suspend fun messages(conversationId: String): List<Message> = emptyList()
        override suspend fun send(conversationId: String, text: String): Message = error("not used")
        override suspend fun sendVoice(conversationId: String, audio: ByteArray, durationSeconds: Int): Message = error("not used")
        override suspend fun markRead(conversationId: String) = Unit
        override suspend fun media(messageId: String): ByteArray = ByteArray(0)
        override fun close() {
            calls += "close"
        }
    }
}
