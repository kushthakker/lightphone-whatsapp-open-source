package org.lp3bridge.whatsapp

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue
import kotlinx.serialization.json.Json

class BridgeModelsTest {
    private val json = Json { ignoreUnknownKeys = true }

    @Test
    fun decodesPinnedGroupConversation() {
        val conversation = json.decodeFromString<Conversation>(
            """{
                "id":"synthetic-group-fixture",
                "displayName":"Project Group ",
                "kind":"group",
                "pinned":true,
                "lastMessage":"anchor",
                "lastMessageAt":1784195037,
                "unreadCount":27
            }""",
        )

        assertEquals("Project Group ", conversation.displayName)
        assertEquals("group", conversation.kind)
        assertTrue(conversation.pinned)
    }

    @Test
    fun decodesNamedGroupSender() {
        val message = json.decodeFromString<Message>(
            """{
                "id":"message-1",
                "conversationId":"synthetic-group-fixture",
                "fromMe":false,
                "timestamp":1784195037,
                "text":"hello",
                "status":"received",
                "senderName":"Teammate",
                "mediaType":"image",
                "mediaMime":"image/jpeg",
                "mediaWidth":1200,
                "mediaHeight":800,
                "mediaDuration":9
            }""",
        )

        assertEquals("Teammate", message.senderName)
        assertEquals("image", message.mediaType)
        assertEquals(1200, message.mediaWidth)
        assertEquals(9, message.mediaDuration)
    }

    @Test
    fun keepsBackwardCompatibleDefaults() {
        val conversation = json.decodeFromString<Conversation>(
            """{"id":"direct-1","displayName":"Someone"}""",
        )
        val message = json.decodeFromString<Message>(
            """{
                "id":"message-2",
                "conversationId":"direct-1",
                "fromMe":false,
                "timestamp":1,
                "text":"hello",
                "status":"received"
            }""",
        )

        assertEquals("direct", conversation.kind)
        assertEquals("", message.senderName)
    }
}
