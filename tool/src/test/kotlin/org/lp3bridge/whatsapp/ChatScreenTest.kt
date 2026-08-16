package org.lp3bridge.whatsapp

import java.time.ZoneId
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse

class ChatScreenTest {
    private fun message(
        id: String,
        timestamp: Long,
        status: String = "sent",
        mediaType: String = "",
    ) = Message(
        id = id,
        conversationId = "conversation",
        fromMe = true,
        timestamp = timestamp,
        text = id,
        status = status,
        mediaType = mediaType,
    )

    @Test
    fun `formats message timestamp in the requested timezone`() {
        assertEquals(
            "16 Jul · 10:15 AM",
            formatMessageTime(1784196910, ZoneId.of("UTC")),
        )
    }

    @Test
    fun `formats voice note duration`() {
        assertEquals("0:07", formatVoiceDuration(7))
        assertEquals("1:05", formatVoiceDuration(65))
    }

    @Test
    fun `send response replaces message already returned by refresh`() {
        val refreshed = reconcileMessages(
            current = emptyList(),
            incoming = listOf(message("sent-id", 2, status = "sending")),
        )
        val completed = reconcileMessages(
            current = refreshed,
            incoming = listOf(message("sent-id", 2, status = "sent")),
        )

        assertEquals(listOf("sent-id"), completed.map { it.id })
        assertEquals("sent", completed.single().status)
    }

    @Test
    fun `stale refresh does not remove a completed send`() {
        val existing = message("existing", 1)
        val completed = reconcileMessages(
            current = listOf(existing),
            incoming = listOf(message("sent-id", 2)),
        )
        val afterStaleRefresh = reconcileMessages(
            current = completed,
            incoming = listOf(existing),
        )

        assertEquals(listOf("existing", "sent-id"), afterStaleRefresh.map { it.id })
        assertFalse(afterStaleRefresh.groupingBy { it.id }.eachCount().values.any { it > 1 })
    }

    @Test
    fun `voice send response replaces voice note already returned by refresh`() {
        val refreshed = reconcileMessages(
            current = emptyList(),
            incoming = listOf(message("voice-id", 2, status = "sending", mediaType = "voice")),
        )
        val completed = reconcileMessages(
            current = refreshed,
            incoming = listOf(message("voice-id", 2, status = "sent", mediaType = "voice")),
        )

        assertEquals(1, completed.size)
        assertEquals("voice", completed.single().mediaType)
        assertEquals("sent", completed.single().status)
    }

    @Test
    fun `refresh normalization removes duplicate ids and keeps latest messages`() {
        val messages = (1L..151L).map { message("message-$it", it) }
        val reconciled = reconcileMessages(
            current = listOf(message("message-151", 151, status = "sending")),
            incoming = messages + message("message-151", 151, status = "sent"),
        )

        assertEquals(150, reconciled.size)
        assertEquals("message-2", reconciled.first().id)
        assertEquals("message-151", reconciled.last().id)
        assertEquals("sent", reconciled.last().status)
    }
}
