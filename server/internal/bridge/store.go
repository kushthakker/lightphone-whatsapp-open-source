package bridge

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

type Conversation struct {
	ID            string `json:"id"`
	DisplayName   string `json:"displayName"`
	Kind          string `json:"kind"`
	Pinned        bool   `json:"pinned"`
	LastMessage   string `json:"lastMessage"`
	LastMessageAt int64  `json:"lastMessageAt"`
	UnreadCount   int    `json:"unreadCount"`
}

type Message struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversationId"`
	FromMe         bool   `json:"fromMe"`
	Timestamp      int64  `json:"timestamp"`
	Text           string `json:"text"`
	Status         string `json:"status"`
	SenderName     string `json:"senderName,omitempty"`
	MediaType      string `json:"mediaType,omitempty"`
	MediaMime      string `json:"mediaMime,omitempty"`
	MediaWidth     int    `json:"mediaWidth,omitempty"`
	MediaHeight    int    `json:"mediaHeight,omitempty"`
	MediaDuration  int    `json:"mediaDuration,omitempty"`
	MediaPath      string `json:"-"`
}

type StoredConversation struct {
	Conversation
	JID string
}

type Store struct {
	db     *sql.DB
	policy GroupPolicy
}

func OpenStore(dsn string, policy GroupPolicy) (*Store, error) {
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open app store: %w", err)
	}
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db, policy: policy}, nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS app_conversations (
  id TEXT PRIMARY KEY,
  wa_jid TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  kind TEXT NOT NULL DEFAULT 'direct',
  pinned INTEGER NOT NULL DEFAULT 0,
  last_message TEXT NOT NULL DEFAULT '',
  last_message_at INTEGER NOT NULL DEFAULT 0,
  unread_count INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS app_messages (
  id TEXT PRIMARY KEY,
  conversation_id TEXT NOT NULL REFERENCES app_conversations(id) ON DELETE CASCADE,
  from_me INTEGER NOT NULL,
  timestamp INTEGER NOT NULL,
  text TEXT NOT NULL,
  status TEXT NOT NULL,
  sender_name TEXT NOT NULL DEFAULT '',
  media_type TEXT NOT NULL DEFAULT '',
  media_mime TEXT NOT NULL DEFAULT '',
  media_width INTEGER NOT NULL DEFAULT 0,
  media_height INTEGER NOT NULL DEFAULT 0,
  media_duration INTEGER NOT NULL DEFAULT 0,
  media_path TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS app_messages_conversation_time
  ON app_messages(conversation_id, timestamp DESC, id DESC);
`
	if _, err := db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("migrate app store: %w", err)
	}
	for _, column := range []struct {
		table      string
		name       string
		definition string
	}{
		{"app_conversations", "kind", "TEXT NOT NULL DEFAULT 'direct'"},
		{"app_conversations", "pinned", "INTEGER NOT NULL DEFAULT 0"},
		{"app_messages", "sender_name", "TEXT NOT NULL DEFAULT ''"},
		{"app_messages", "media_type", "TEXT NOT NULL DEFAULT ''"},
		{"app_messages", "media_mime", "TEXT NOT NULL DEFAULT ''"},
		{"app_messages", "media_width", "INTEGER NOT NULL DEFAULT 0"},
		{"app_messages", "media_height", "INTEGER NOT NULL DEFAULT 0"},
		{"app_messages", "media_duration", "INTEGER NOT NULL DEFAULT 0"},
		{"app_messages", "media_path", "TEXT NOT NULL DEFAULT ''"},
	} {
		if err := ensureColumn(ctx, db, column.table, column.name, column.definition); err != nil {
			return err
		}
	}
	return nil
}

func ensureColumn(ctx context.Context, db *sql.DB, table, name, definition string) error {
	rows, err := db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return fmt.Errorf("inspect %s schema: %w", table, err)
	}
	found := false
	for rows.Next() {
		var cid int
		var columnName, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &columnName, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			_ = rows.Close()
			return err
		}
		if columnName == name {
			found = true
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if found {
		return nil
	}
	if _, err := db.ExecContext(ctx, "ALTER TABLE "+table+" ADD COLUMN "+name+" "+definition); err != nil {
		return fmt.Errorf("add %s.%s: %w", table, name, err)
	}
	return nil
}

func ConversationID(jid string) string {
	sum := sha256.Sum256([]byte(jid))
	return hex.EncodeToString(sum[:12])
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) UpsertConversation(ctx context.Context, jid, displayName, kind string, pinned bool) error {
	if jid == "" {
		return errors.New("jid is required")
	}
	if displayName == "" {
		displayName = jid
	}
	if kind == "" {
		kind = "direct"
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO app_conversations (id, wa_jid, display_name, kind, pinned, last_message, last_message_at, unread_count)
VALUES (?, ?, ?, ?, ?, '', 0, 0)
ON CONFLICT(wa_jid) DO UPDATE SET
  display_name = CASE WHEN excluded.display_name = excluded.wa_jid THEN app_conversations.display_name ELSE excluded.display_name END,
  kind = excluded.kind,
  pinned = excluded.pinned`, ConversationID(jid), jid, displayName, kind, pinned)
	if err != nil {
		return fmt.Errorf("upsert conversation: %w", err)
	}
	return nil
}

func (s *Store) ResetGroupPins(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE app_conversations SET pinned=0 WHERE kind='group'`)
	return err
}

func (s *Store) UpsertMessage(ctx context.Context, jid, displayName, kind string, pinned bool, msg Message, unreadDelta int) error {
	if jid == "" || msg.ID == "" {
		return errors.New("jid and message id are required")
	}
	conversationID := ConversationID(jid)
	if displayName == "" {
		displayName = jid
	}
	if kind == "" {
		kind = "direct"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `
INSERT INTO app_conversations (id, wa_jid, display_name, kind, pinned, last_message, last_message_at, unread_count)
VALUES (?, ?, ?, ?, ?, '', 0, 0)
ON CONFLICT(wa_jid) DO UPDATE SET
  display_name = CASE WHEN excluded.display_name = excluded.wa_jid THEN app_conversations.display_name ELSE excluded.display_name END,
  kind = excluded.kind,
  pinned = excluded.pinned`,
		conversationID, jid, displayName, kind, pinned)
	if err != nil {
		return fmt.Errorf("upsert conversation: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
INSERT INTO app_messages (id, conversation_id, from_me, timestamp, text, status, sender_name, media_type, media_mime, media_width, media_height, media_duration, media_path)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO NOTHING`,
		msg.ID, conversationID, msg.FromMe, msg.Timestamp, msg.Text, msg.Status, msg.SenderName,
		msg.MediaType, msg.MediaMime, msg.MediaWidth, msg.MediaHeight, msg.MediaDuration, msg.MediaPath)
	if err != nil {
		return fmt.Errorf("upsert message: %w", err)
	}
	inserted, _ := result.RowsAffected()
	if inserted > 0 {
		_, err = tx.ExecContext(ctx, `
UPDATE app_conversations SET
  last_message = CASE WHEN ? >= last_message_at THEN ? ELSE last_message END,
  last_message_at = MAX(last_message_at, ?),
  unread_count = MAX(0, unread_count + ?)
WHERE id = ?`, msg.Timestamp, msg.Text, msg.Timestamp, unreadDelta, conversationID)
		if err != nil {
			return fmt.Errorf("update conversation summary: %w", err)
		}
	} else {
		_, err = tx.ExecContext(ctx, `
UPDATE app_messages SET
  status=?, text=?, sender_name=CASE WHEN ? != '' THEN ? ELSE sender_name END,
  media_type=CASE WHEN ? != '' THEN ? ELSE media_type END,
  media_mime=CASE WHEN ? != '' THEN ? ELSE media_mime END,
  media_width=CASE WHEN ? > 0 THEN ? ELSE media_width END,
  media_height=CASE WHEN ? > 0 THEN ? ELSE media_height END,
  media_duration=CASE WHEN ? > 0 THEN ? ELSE media_duration END,
  media_path=CASE WHEN ? != '' THEN ? ELSE media_path END
WHERE id=?`, msg.Status, msg.Text, msg.SenderName, msg.SenderName,
			msg.MediaType, msg.MediaType, msg.MediaMime, msg.MediaMime,
			msg.MediaWidth, msg.MediaWidth, msg.MediaHeight, msg.MediaHeight,
			msg.MediaDuration, msg.MediaDuration,
			msg.MediaPath, msg.MediaPath, msg.ID)
		if err != nil {
			return fmt.Errorf("update existing message: %w", err)
		}
	}
	return tx.Commit()
}

func (s *Store) SetMessageStatus(ctx context.Context, id, status string, timestamp int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE app_messages SET status=?, timestamp=CASE WHEN ? > 0 THEN ? ELSE timestamp END WHERE id=?`, status, timestamp, timestamp, id)
	return err
}

func (s *Store) MessageExists(ctx context.Context, id string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app_messages WHERE id=?)`, id).Scan(&exists)
	return exists, err
}

func (s *Store) Conversations(ctx context.Context) ([]Conversation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, display_name, kind, pinned, last_message, last_message_at, unread_count FROM app_conversations ORDER BY pinned DESC, last_message_at DESC, display_name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Conversation, 0)
	for rows.Next() {
		var item Conversation
		if err := rows.Scan(&item.ID, &item.DisplayName, &item.Kind, &item.Pinned, &item.LastMessage, &item.LastMessageAt, &item.UnreadCount); err != nil {
			return nil, err
		}
		if !s.visibleConversation(item) {
			continue
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) visibleConversation(conversation Conversation) bool {
	return conversation.Kind != "group" || s.policy.Includes(conversation.DisplayName, conversation.Pinned)
}

func (s *Store) ConversationByID(ctx context.Context, id string) (StoredConversation, error) {
	var item StoredConversation
	err := s.db.QueryRowContext(ctx, `SELECT id, wa_jid, display_name, kind, pinned, last_message, last_message_at, unread_count FROM app_conversations WHERE id=?`, id).Scan(
		&item.ID, &item.JID, &item.DisplayName, &item.Kind, &item.Pinned, &item.LastMessage, &item.LastMessageAt, &item.UnreadCount,
	)
	if err == nil && !s.visibleConversation(item.Conversation) {
		return StoredConversation{}, sql.ErrNoRows
	}
	return item, err
}

func (s *Store) Messages(ctx context.Context, conversationID string, before int64, limit int) ([]Message, error) {
	if _, err := s.ConversationByID(ctx, conversationID); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 200 {
		limit = 100
	}
	if before <= 0 {
		before = time.Now().Add(24 * time.Hour).Unix()
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, conversation_id, from_me, timestamp, text, status, sender_name, media_type, media_mime, media_width, media_height, media_duration, media_path
FROM app_messages WHERE conversation_id=? AND timestamp < ?
ORDER BY timestamp DESC, id DESC LIMIT ?`, conversationID, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Message, 0)
	for rows.Next() {
		var item Message
		if err := scanMessage(rows.Scan, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
	return items, nil
}

func (s *Store) OldestMessage(ctx context.Context, conversationID string) (StoredConversation, Message, error) {
	conversation, err := s.ConversationByID(ctx, conversationID)
	if err != nil {
		return StoredConversation{}, Message{}, err
	}
	var message Message
	err = s.db.QueryRowContext(ctx, `
SELECT id, conversation_id, from_me, timestamp, text, status, sender_name, media_type, media_mime, media_width, media_height, media_duration, media_path
FROM app_messages WHERE conversation_id=? ORDER BY timestamp ASC, id ASC LIMIT 1`, conversationID).Scan(
		&message.ID, &message.ConversationID, &message.FromMe, &message.Timestamp, &message.Text, &message.Status, &message.SenderName,
		&message.MediaType, &message.MediaMime, &message.MediaWidth, &message.MediaHeight, &message.MediaDuration, &message.MediaPath,
	)
	return conversation, message, err
}

func (s *Store) NewestMessage(ctx context.Context, conversationID string) (StoredConversation, Message, error) {
	conversation, err := s.ConversationByID(ctx, conversationID)
	if err != nil {
		return StoredConversation{}, Message{}, err
	}
	var message Message
	err = s.db.QueryRowContext(ctx, `
SELECT id, conversation_id, from_me, timestamp, text, status, sender_name, media_type, media_mime, media_width, media_height, media_duration, media_path
FROM app_messages WHERE conversation_id=? ORDER BY timestamp DESC, id DESC LIMIT 1`, conversationID).Scan(
		&message.ID, &message.ConversationID, &message.FromMe, &message.Timestamp, &message.Text, &message.Status, &message.SenderName,
		&message.MediaType, &message.MediaMime, &message.MediaWidth, &message.MediaHeight, &message.MediaDuration, &message.MediaPath,
	)
	return conversation, message, err
}

func (s *Store) MarkRead(ctx context.Context, conversationID string) error {
	if _, err := s.ConversationByID(ctx, conversationID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE app_conversations SET unread_count=0 WHERE id=?`, conversationID)
	return err
}

func (s *Store) GroupMessagesContainingMentions(ctx context.Context) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT m.id, m.conversation_id, m.from_me, m.timestamp, m.text, m.status, m.sender_name, m.media_type, m.media_mime, m.media_width, m.media_height, m.media_duration, m.media_path, c.display_name, c.pinned
FROM app_messages m
JOIN app_conversations c ON c.id = m.conversation_id
WHERE c.kind = 'group' AND m.text LIKE '%@%'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Message
	for rows.Next() {
		var item Message
		var displayName string
		var pinned bool
		if err := rows.Scan(
			&item.ID, &item.ConversationID, &item.FromMe, &item.Timestamp, &item.Text, &item.Status, &item.SenderName,
			&item.MediaType, &item.MediaMime, &item.MediaWidth, &item.MediaHeight, &item.MediaDuration, &item.MediaPath,
			&displayName, &pinned,
		); err != nil {
			return nil, err
		}
		if s.policy.Includes(displayName, pinned) {
			items = append(items, item)
		}
	}
	return items, rows.Err()
}

type scanFunc func(dest ...any) error

func scanMessage(scan scanFunc, item *Message) error {
	return scan(
		&item.ID, &item.ConversationID, &item.FromMe, &item.Timestamp, &item.Text, &item.Status, &item.SenderName,
		&item.MediaType, &item.MediaMime, &item.MediaWidth, &item.MediaHeight, &item.MediaDuration, &item.MediaPath,
	)
}

func (s *Store) MessageMedia(ctx context.Context, id string) (string, string, error) {
	var path, mime string
	var conversation Conversation
	err := s.db.QueryRowContext(ctx, `
SELECT m.media_path, m.media_mime, c.id, c.display_name, c.kind, c.pinned, c.last_message, c.last_message_at, c.unread_count
FROM app_messages m
JOIN app_conversations c ON c.id = m.conversation_id
WHERE m.id=? AND m.media_path!=''`, id).Scan(
		&path, &mime, &conversation.ID, &conversation.DisplayName, &conversation.Kind, &conversation.Pinned,
		&conversation.LastMessage, &conversation.LastMessageAt, &conversation.UnreadCount,
	)
	if err == nil && !s.visibleConversation(conversation) {
		return "", "", sql.ErrNoRows
	}
	return path, mime, err
}

func (s *Store) UpdateMessageText(ctx context.Context, id, oldText, newText string) error {
	if id == "" || oldText == newText {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE app_messages SET text=? WHERE id=?`, newText, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE app_conversations SET last_message=?
WHERE id=(SELECT conversation_id FROM app_messages WHERE id=?) AND last_message=?`, newText, id, oldText); err != nil {
		return err
	}
	return tx.Commit()
}
