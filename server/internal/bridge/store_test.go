package bridge

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestStoreRoundTripAndIdempotency(t *testing.T) {
	store, err := OpenStore("file:"+filepath.Join(t.TempDir(), "test.db")+"?_foreign_keys=on", DefaultGroupPolicy())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	jid := "person@example.test"
	msg := Message{ID: "message-1", FromMe: false, Timestamp: 100, Text: "hello", Status: "sent"}
	if err := store.UpsertMessage(ctx, jid, "A Person", "direct", false, msg, 1); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertMessage(ctx, jid, "A Person", "direct", false, msg, 1); err != nil {
		t.Fatal(err)
	}
	conversations, err := store.Conversations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(conversations) != 1 || conversations[0].UnreadCount != 1 {
		t.Fatalf("unexpected conversations: %#v", conversations)
	}
	messages, err := store.Messages(ctx, conversations[0].ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Text != "hello" {
		t.Fatalf("unexpected messages: %#v", messages)
	}
}

func TestPinnedGroupAndSenderName(t *testing.T) {
	store, err := OpenStore("file:"+filepath.Join(t.TempDir(), "group.db")+"?_foreign_keys=on", DefaultGroupPolicy())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	groupJID := "12345@g.us"
	if err := store.UpsertMessage(ctx, groupJID, "Project Updates", "group", true, Message{
		ID: "group-1", Timestamp: 100, Text: "hello team", Status: "sent", SenderName: "Alex",
	}, 1); err != nil {
		t.Fatal(err)
	}
	conversations, err := store.Conversations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(conversations) != 1 || conversations[0].Kind != "group" || !conversations[0].Pinned {
		t.Fatalf("unexpected group conversation: %#v", conversations)
	}
	messages, err := store.Messages(ctx, conversations[0].ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].SenderName != "Alex" {
		t.Fatalf("sender name was not preserved: %#v", messages)
	}
}

func TestAllowlistedGroupIsIncludedWithoutBeingPinned(t *testing.T) {
	policy, err := NewGroupPolicy(GroupModePinned, []string{"Project   Updates"})
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore("file:"+filepath.Join(t.TempDir(), "visible-group.db")+"?_foreign_keys=on", policy)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.UpsertMessage(ctx, "visible@g.us", "project updates", "group", false, Message{
		ID: "visible-group-1", Timestamp: 100, Text: "hello", Status: "sent", SenderName: "Person",
	}, 1); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertMessage(ctx, "hidden@g.us", "Unrelated group", "group", false, Message{
		ID: "hidden-group-1", Timestamp: 200, Text: "hidden", Status: "sent", SenderName: "Person",
	}, 1); err != nil {
		t.Fatal(err)
	}
	conversations, err := store.Conversations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(conversations) != 1 || conversations[0].DisplayName != "project updates" || conversations[0].Pinned {
		t.Fatalf("unexpected visible groups: %#v", conversations)
	}
}

func TestBackfillMentionTextUpdatesMessageAndConversationPreview(t *testing.T) {
	store, err := OpenStore("file:"+filepath.Join(t.TempDir(), "mentions.db")+"?_foreign_keys=on", DefaultGroupPolicy())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	oldText := "@123456 send photo"
	if err := store.UpsertMessage(ctx, "12345@g.us", "Project Updates", "group", true, Message{
		ID: "mention-1", Timestamp: 100, Text: oldText, Status: "sent", SenderName: "You",
	}, 0); err != nil {
		t.Fatal(err)
	}
	messages, err := store.GroupMessagesContainingMentions(ctx)
	if err != nil || len(messages) != 1 {
		t.Fatalf("unexpected mention messages: %#v, err=%v", messages, err)
	}
	if err := store.UpdateMessageText(ctx, messages[0].ID, oldText, "@Alex send photo"); err != nil {
		t.Fatal(err)
	}
	updated, err := store.Messages(ctx, ConversationID("12345@g.us"), 0, 10)
	if err != nil || len(updated) != 1 || updated[0].Text != "@Alex send photo" {
		t.Fatalf("message text was not updated: %#v, err=%v", updated, err)
	}
	conversations, err := store.Conversations(ctx)
	if err != nil || len(conversations) != 1 || conversations[0].LastMessage != "@Alex send photo" {
		t.Fatalf("conversation preview was not updated: %#v, err=%v", conversations, err)
	}
}

func TestOpenStoreMigratesExistingArchive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite3", "file:"+path+"?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE app_conversations (id TEXT PRIMARY KEY, wa_jid TEXT NOT NULL UNIQUE, display_name TEXT NOT NULL, last_message TEXT NOT NULL DEFAULT '', last_message_at INTEGER NOT NULL DEFAULT 0, unread_count INTEGER NOT NULL DEFAULT 0);
CREATE TABLE app_messages (id TEXT PRIMARY KEY, conversation_id TEXT NOT NULL REFERENCES app_conversations(id), from_me INTEGER NOT NULL, timestamp INTEGER NOT NULL, text TEXT NOT NULL, status TEXT NOT NULL);`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore("file:"+path+"?_foreign_keys=on", DefaultGroupPolicy())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.UpsertMessage(context.Background(), "1@g.us", "Project", "group", true, Message{ID: "m1", Timestamp: 1, Text: "migrated", Status: "sent", SenderName: "Me"}, 0); err != nil {
		t.Fatal(err)
	}
}

func TestRenamedGroupStopsUsingStaleAllowlistedName(t *testing.T) {
	policy, err := NewGroupPolicy(GroupModePinned, []string{"Project Updates"})
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore("file:"+filepath.Join(t.TempDir(), "test.db")+"?_foreign_keys=on", policy)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	jid := "renamed@g.us"
	message := Message{ID: "before-rename", Timestamp: 10, Text: "archived", Status: "received"}
	if err := store.UpsertMessage(ctx, jid, "Project Updates", "group", false, message, 0); err != nil {
		t.Fatal(err)
	}
	conversationID := ConversationID(jid)
	if _, err := store.ConversationByID(ctx, conversationID); err != nil {
		t.Fatalf("allowlisted group was not visible before rename: %v", err)
	}
	if err := store.UpsertConversation(ctx, jid, "Renamed Private Group", "group", false); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ConversationByID(ctx, conversationID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("renamed excluded group remained visible: %v", err)
	}
	if _, err := store.Messages(ctx, conversationID, 0, 10); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("archived messages remained accessible after exclusion: %v", err)
	}
}

func TestMessageOrdering(t *testing.T) {
	store, err := OpenStore("file:"+filepath.Join(t.TempDir(), "test.db")+"?_foreign_keys=on", DefaultGroupPolicy())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	jid := "person@example.test"
	for _, msg := range []Message{{ID: "later", Timestamp: 200, Text: "later", Status: "sent"}, {ID: "earlier", Timestamp: 100, Text: "earlier", Status: "sent"}} {
		if err := store.UpsertMessage(ctx, jid, "Person", "direct", false, msg, 0); err != nil {
			t.Fatal(err)
		}
	}
	messages, err := store.Messages(ctx, ConversationID(jid), 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Text != "earlier" || messages[1].Text != "later" {
		t.Fatalf("wrong order: %#v", messages)
	}
	_, newest, err := store.NewestMessage(ctx, ConversationID(jid))
	if err != nil || newest.Text != "later" {
		t.Fatalf("wrong newest message: %#v, err=%v", newest, err)
	}
}
