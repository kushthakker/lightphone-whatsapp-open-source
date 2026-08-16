package org.lp3bridge.whatsapp

import io.ktor.client.HttpClient
import io.ktor.client.call.body
import io.ktor.client.engine.okhttp.OkHttp
import io.ktor.client.plugins.contentnegotiation.ContentNegotiation
import io.ktor.client.request.HttpRequestBuilder
import io.ktor.client.request.get
import io.ktor.client.request.header
import io.ktor.client.request.post
import io.ktor.client.request.setBody
import io.ktor.client.statement.HttpResponse
import io.ktor.http.ContentType
import io.ktor.http.HttpHeaders
import io.ktor.http.contentType
import io.ktor.serialization.kotlinx.json.json
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json

@Serializable
data class Conversation(
    val id: String,
    val displayName: String,
    val kind: String = "direct",
    val pinned: Boolean = false,
    val lastMessage: String = "",
    val lastMessageAt: Long = 0,
    val unreadCount: Int = 0,
)

@Serializable
data class Message(
    val id: String,
    val conversationId: String,
    val fromMe: Boolean,
    val timestamp: Long,
    val text: String,
    val status: String,
    val senderName: String = "",
    val mediaType: String = "",
    val mediaMime: String = "",
    val mediaWidth: Int = 0,
    val mediaHeight: Int = 0,
    val mediaDuration: Int = 0,
)

@Serializable
private data class ConversationsResponse(val conversations: List<Conversation>)

@Serializable
private data class MessagesResponse(val conversation: Conversation, val messages: List<Message>)

@Serializable
private data class SendRequest(val text: String)

interface BridgeApi : AutoCloseable {
    suspend fun healthz()
    suspend fun status()
    suspend fun conversations(): List<Conversation>
    suspend fun messages(conversationId: String): List<Message>
    suspend fun send(conversationId: String, text: String): Message
    suspend fun sendVoice(conversationId: String, audio: ByteArray, durationSeconds: Int): Message
    suspend fun markRead(conversationId: String)
    suspend fun media(messageId: String): ByteArray

    override fun close() = Unit
}

fun interface BridgeClientFactory {
    fun create(config: BridgeConfig): BridgeApi
}

object DefaultBridgeClientFactory : BridgeClientFactory {
    override fun create(config: BridgeConfig): BridgeApi = BridgeClient(config)
}

class BridgeUnauthorizedException : Exception("Bridge credentials were rejected")

class BridgeRequestException(statusCode: Int) : Exception("Bridge request failed ($statusCode)")

/** A configured, instance-scoped bridge client. It never reads build-time configuration. */
class BridgeClient(
    private val config: BridgeConfig,
    private val client: HttpClient = HttpClient(OkHttp) {
        expectSuccess = false
        install(ContentNegotiation) { json(Json { ignoreUnknownKeys = true }) }
    },
) : BridgeApi {
    private fun HttpRequestBuilder.authorize() {
        header(HttpHeaders.Authorization, "Bearer ${config.apiToken}")
    }

    private suspend fun HttpResponse.requireSuccess() {
        when (status.value) {
            401, 403 -> throw BridgeUnauthorizedException()
            in 200..299 -> Unit
            else -> throw BridgeRequestException(status.value)
        }
    }

    override suspend fun healthz() {
        client.get("${config.baseUrl}/healthz").requireSuccess()
    }

    override suspend fun status() {
        client.get("${config.baseUrl}/api/v1/status") { authorize() }.requireSuccess()
    }

    override suspend fun conversations(): List<Conversation> {
        val response = client.get("${config.baseUrl}/api/v1/conversations") { authorize() }
        response.requireSuccess()
        return response.body<ConversationsResponse>().conversations
    }

    override suspend fun messages(conversationId: String): List<Message> {
        val response = client.get("${config.baseUrl}/api/v1/conversations/$conversationId/messages?limit=150") {
            authorize()
        }
        response.requireSuccess()
        return response.body<MessagesResponse>().messages
    }

    override suspend fun send(conversationId: String, text: String): Message {
        val response = client.post("${config.baseUrl}/api/v1/conversations/$conversationId/messages") {
            authorize()
            contentType(ContentType.Application.Json)
            setBody(SendRequest(text))
        }
        response.requireSuccess()
        return response.body()
    }

    override suspend fun sendVoice(conversationId: String, audio: ByteArray, durationSeconds: Int): Message {
        val response = client.post("${config.baseUrl}/api/v1/conversations/$conversationId/voice") {
            authorize()
            header("X-Voice-Duration-Seconds", durationSeconds)
            contentType(ContentType.parse("audio/ogg; codecs=opus"))
            setBody(audio)
        }
        response.requireSuccess()
        return response.body()
    }

    override suspend fun markRead(conversationId: String) {
        client.post("${config.baseUrl}/api/v1/conversations/$conversationId/read") { authorize() }
            .requireSuccess()
    }

    override suspend fun media(messageId: String): ByteArray {
        val response = client.get("${config.baseUrl}/api/v1/messages/$messageId/media") { authorize() }
        response.requireSuccess()
        return response.body()
    }

    override fun close() {
        client.close()
    }
}

class BridgeConfigurationVerifier(
    private val clientFactory: BridgeClientFactory = DefaultBridgeClientFactory,
) {
    /** Health is intentionally unauthenticated; status proves the supplied token before storage. */
    suspend fun verify(config: BridgeConfig) {
        val client = clientFactory.create(config)
        try {
            client.healthz()
            client.status()
        } finally {
            client.close()
        }
    }
}

internal fun isBridgeUnauthorized(error: Throwable): Boolean =
    generateSequence(error) { it.cause }.any { it is BridgeUnauthorizedException }
