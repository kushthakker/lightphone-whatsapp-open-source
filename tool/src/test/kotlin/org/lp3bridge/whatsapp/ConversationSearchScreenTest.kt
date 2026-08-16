package org.lp3bridge.whatsapp

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class ConversationSearchScreenTest {
    private val conversations = listOf(
        Conversation(id = "1", displayName = "Town hall", lastMessageAt = 300),
        Conversation(id = "2", displayName = "Team", kind = "group", pinned = true, lastMessageAt = 100),
        Conversation(id = "3", displayName = "Team updates", lastMessageAt = 200),
        Conversation(id = "4", displayName = "Project group", kind = "group", pinned = true, lastMessageAt = 400),
    )

    @Test
    fun `search is case insensitive and ranks prefix matches first`() {
        assertEquals(
            listOf("Team", "Team updates"),
            filterConversations(conversations, "  TEAM ").map { it.displayName },
        )
    }

    @Test
    fun `blank search shows no results`() {
        assertTrue(filterConversations(conversations, "   ").isEmpty())
    }

    @Test
    fun `search finds a group by partial name`() {
        assertEquals(
            listOf("Project group"),
            filterConversations(conversations, "project").map { it.displayName },
        )
    }
}
