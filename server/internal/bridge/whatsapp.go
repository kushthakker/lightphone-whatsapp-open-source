package bridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

type PairingStatus struct {
	State     string `json:"state"`
	Connected bool   `json:"connected"`
	Paired    bool   `json:"paired"`
	Error     string `json:"error,omitempty"`
	UpdatedAt int64  `json:"updatedAt"`
}

type WhatsApp struct {
	client   *whatsmeow.Client
	store    *Store
	mediaDir string

	mu     sync.RWMutex
	status PairingStatus
	qrPNG  []byte
	groups map[string]groupMetadata
	policy GroupPolicy
}

type groupMetadata struct {
	Name     string
	Pinned   bool
	Included bool
}

var numericMentionPattern = regexp.MustCompile(`@([0-9]{5,20})\b`)

func NewWhatsApp(ctx context.Context, dsn string, appStore *Store, mediaDir string, policy GroupPolicy) (*WhatsApp, error) {
	container, err := sqlstore.New(ctx, "sqlite3", dsn, waLog.Noop)
	if err != nil {
		return nil, err
	}
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		return nil, err
	}
	w := &WhatsApp{
		client:   whatsmeow.NewClient(device, waLog.Noop),
		store:    appStore,
		mediaDir: mediaDir,
		groups:   make(map[string]groupMetadata),
		policy:   policy,
	}
	w.client.AddEventHandler(w.handleEvent)
	w.setStatus("starting", false, device.ID != nil, "")
	return w, nil
}

func (w *WhatsApp) Start(ctx context.Context) error {
	if w.client.Store.ID == nil {
		qrChannel, err := w.client.GetQRChannel(ctx)
		if err != nil {
			return err
		}
		go w.consumeQR(qrChannel)
		w.setStatus("waiting_for_qr", false, false, "")
	} else {
		w.setStatus("connecting", false, true, "")
	}
	if err := w.client.Connect(); err != nil {
		w.setStatus("error", false, w.client.Store.ID != nil, "Unable to connect WhatsApp")
		return err
	}
	return nil
}

func (w *WhatsApp) Close() { w.client.Disconnect() }

func (w *WhatsApp) consumeQR(ch <-chan whatsmeow.QRChannelItem) {
	for item := range ch {
		switch item.Event {
		case whatsmeow.QRChannelEventCode:
			png, err := qrcode.Encode(item.Code, qrcode.Medium, 420)
			if err != nil {
				w.setStatus("error", false, false, "Unable to generate QR code")
				continue
			}
			w.mu.Lock()
			w.qrPNG = png
			w.status = PairingStatus{State: "waiting_for_scan", UpdatedAt: time.Now().Unix()}
			w.mu.Unlock()
		case "success":
			w.setStatus("paired", w.client.IsConnected(), true, "")
		case "timeout":
			w.setStatus("qr_expired", false, false, "Restart the service to generate a new QR code")
		default:
			message := ""
			if item.Error != nil {
				message = "WhatsApp pairing error"
			}
			w.setStatus(item.Event, w.client.IsConnected(), w.client.Store.ID != nil, message)
		}
	}
}

func (w *WhatsApp) Status() PairingStatus {
	w.mu.RLock()
	defer w.mu.RUnlock()
	status := w.status
	status.Connected = w.client.IsConnected() && w.client.IsLoggedIn()
	status.Paired = w.client.Store.ID != nil
	return status
}

func (w *WhatsApp) QRPNG() []byte {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return append([]byte(nil), w.qrPNG...)
}

func (w *WhatsApp) setStatus(state string, connected, paired bool, errText string) {
	w.mu.Lock()
	w.status = PairingStatus{State: state, Connected: connected, Paired: paired, Error: errText, UpdatedAt: time.Now().Unix()}
	w.mu.Unlock()
}

func (w *WhatsApp) handleEvent(raw any) {
	switch event := raw.(type) {
	case *events.Connected:
		w.setStatus("connected", true, w.client.Store.ID != nil, "")
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			_ = w.syncGroupMetadata(ctx)
			_ = w.backfillMentionNames(ctx)
		}()
	case *events.Disconnected:
		w.setStatus("disconnected", false, w.client.Store.ID != nil, "")
	case *events.LoggedOut:
		w.setStatus("logged_out", false, false, "WhatsApp logged out")
	case *events.Message:
		_ = w.persistEvent(context.Background(), event)
	case *events.HistorySync:
		w.persistHistory(event)
	}
}

func (w *WhatsApp) persistHistory(event *events.HistorySync) {
	if event.Data == nil {
		return
	}
	for _, conversation := range event.Data.GetConversations() {
		jid, err := types.ParseJID(conversation.GetID())
		if err != nil || !w.isIncludedChat(jid) {
			continue
		}
		for _, historyMessage := range conversation.GetMessages() {
			webMessage := historyMessage.GetMessage()
			if webMessage == nil {
				continue
			}
			parsed, err := w.client.ParseWebMessage(jid, webMessage)
			if err != nil {
				continue
			}
			_ = w.persistEvent(context.Background(), parsed)
		}
	}
}

func (w *WhatsApp) persistEvent(ctx context.Context, event *events.Message) error {
	if event == nil || event.Message == nil || !w.isIncludedChat(event.Info.Chat) {
		return nil
	}
	text := w.messageText(ctx, event.Message)
	media := w.messageMedia(ctx, string(event.Info.ID), event.Message)
	if text == "" && media.MediaType == "image" {
		text = "[Image]"
	}
	if text == "" && media.MediaType == "voice" {
		text = "[Voice note]"
	}
	if text == "" && media.MediaType == "audio" {
		text = "[Audio]"
	}
	if text == "" && media.MediaType == "" {
		return nil
	}
	kind := "direct"
	pinned := false
	name := w.contactName(ctx, event.Info.Chat, event.Info.PushName)
	senderName := ""
	if event.Info.IsGroup {
		kind = "group"
		metadata, ok := w.groupMetadata(event.Info.Chat)
		if !ok || !metadata.Included {
			return nil
		}
		name, pinned = metadata.Name, metadata.Pinned
		if event.Info.IsFromMe {
			senderName = "You"
		} else {
			sender := event.Info.SenderAlt
			if sender.IsEmpty() {
				sender = event.Info.Sender
			}
			senderName = w.contactName(ctx, sender, event.Info.PushName)
		}
	}
	unread := 0
	if !event.Info.IsFromMe {
		unread = 1
	}
	message := Message{
		ID:             string(event.Info.ID),
		ConversationID: ConversationID(event.Info.Chat.String()),
		FromMe:         event.Info.IsFromMe,
		Timestamp:      event.Info.Timestamp.Unix(),
		Text:           text,
		Status:         "sent",
		SenderName:     senderName,
		MediaType:      media.MediaType,
		MediaMime:      media.MediaMime,
		MediaWidth:     media.MediaWidth,
		MediaHeight:    media.MediaHeight,
		MediaDuration:  media.MediaDuration,
		MediaPath:      media.MediaPath,
	}
	if err := w.store.UpsertMessage(ctx, event.Info.Chat.String(), name, kind, pinned, message, unread); err != nil {
		return err
	}
	return nil
}

func (w *WhatsApp) Send(ctx context.Context, conversation StoredConversation, text string) (Message, error) {
	if !w.client.IsConnected() || !w.client.IsLoggedIn() {
		return Message{}, errors.New("WhatsApp is not connected")
	}
	jid, err := types.ParseJID(conversation.JID)
	if err != nil || !w.isIncludedChat(jid) {
		return Message{}, errors.New("invalid or unavailable chat recipient")
	}
	id := w.client.GenerateMessageID()
	now := time.Now().Unix()
	message := Message{ID: string(id), ConversationID: conversation.ID, FromMe: true, Timestamp: now, Text: text, Status: "sending"}
	if conversation.Kind == "group" {
		message.SenderName = "You"
	}
	if err := w.store.UpsertMessage(ctx, conversation.JID, conversation.DisplayName, conversation.Kind, conversation.Pinned, message, 0); err != nil {
		return Message{}, err
	}
	response, err := w.client.SendMessage(ctx, jid, &waE2E.Message{Conversation: proto.String(text)}, whatsmeow.SendRequestExtra{ID: id})
	if err != nil {
		message.Status = "failed"
		_ = w.store.SetMessageStatus(ctx, message.ID, message.Status, 0)
		return message, err
	}
	message.Status = "sent"
	message.Timestamp = response.Timestamp.Unix()
	if message.Timestamp <= 0 {
		message.Timestamp = now
	}
	_ = w.store.SetMessageStatus(ctx, message.ID, message.Status, message.Timestamp)
	return message, nil
}

func (w *WhatsApp) SendVoice(ctx context.Context, conversation StoredConversation, audio []byte, durationSeconds int) (Message, error) {
	if !w.client.IsConnected() || !w.client.IsLoggedIn() {
		return Message{}, errors.New("WhatsApp is not connected")
	}
	jid, err := types.ParseJID(conversation.JID)
	if err != nil || !w.isIncludedChat(jid) {
		return Message{}, errors.New("invalid or unavailable chat recipient")
	}
	if len(audio) == 0 || len(audio) > maxVoiceNoteBytes || durationSeconds < 1 || durationSeconds > 120 {
		return Message{}, errors.New("invalid voice note")
	}
	uploaded, err := w.client.Upload(ctx, audio, whatsmeow.MediaAudio)
	if err != nil {
		return Message{}, fmt.Errorf("upload voice note: %w", err)
	}
	id := w.client.GenerateMessageID()
	now := time.Now().Unix()
	message := Message{
		ID: string(id), ConversationID: conversation.ID, FromMe: true, Timestamp: now,
		Text: "[Voice note]", Status: "sending", MediaType: "voice",
		MediaMime: "audio/ogg; codecs=opus", MediaDuration: durationSeconds,
	}
	sum := sha256.Sum256([]byte(message.ID))
	filename := hex.EncodeToString(sum[:]) + ".audio"
	if err := os.WriteFile(filepath.Join(w.mediaDir, filename), audio, 0600); err == nil {
		message.MediaPath = filename
	}
	if conversation.Kind == "group" {
		message.SenderName = "You"
	}
	if err := w.store.UpsertMessage(ctx, conversation.JID, conversation.DisplayName, conversation.Kind, conversation.Pinned, message, 0); err != nil {
		return Message{}, err
	}
	response, err := w.client.SendMessage(ctx, jid, &waE2E.Message{AudioMessage: newVoiceMessage(uploaded, durationSeconds)}, whatsmeow.SendRequestExtra{ID: id})
	if err != nil {
		message.Status = "failed"
		_ = w.store.SetMessageStatus(ctx, message.ID, message.Status, 0)
		return message, err
	}
	message.Status = "sent"
	message.Timestamp = response.Timestamp.Unix()
	if message.Timestamp <= 0 {
		message.Timestamp = now
	}
	_ = w.store.SetMessageStatus(ctx, message.ID, message.Status, message.Timestamp)
	return message, nil
}

func newVoiceMessage(uploaded whatsmeow.UploadResponse, durationSeconds int) *waE2E.AudioMessage {
	return &waE2E.AudioMessage{
		URL:               proto.String(uploaded.URL),
		DirectPath:        proto.String(uploaded.DirectPath),
		MediaKey:          uploaded.MediaKey,
		FileEncSHA256:     uploaded.FileEncSHA256,
		FileSHA256:        uploaded.FileSHA256,
		FileLength:        proto.Uint64(uploaded.FileLength),
		Mimetype:          proto.String("audio/ogg; codecs=opus"),
		PTT:               proto.Bool(true),
		Seconds:           proto.Uint32(uint32(durationSeconds)),
		MediaKeyTimestamp: proto.Int64(time.Now().Unix()),
	}
}

func (w *WhatsApp) RequestHistory(ctx context.Context, conversationID string, count int, recent bool) error {
	if count < 1 || count > 100 {
		count = 50
	}
	conversation, message, err := w.store.OldestMessage(ctx, conversationID)
	if recent {
		conversation, message, err = w.store.NewestMessage(ctx, conversationID)
	}
	if err != nil {
		return err
	}
	jid, err := types.ParseJID(conversation.JID)
	if err != nil || !w.isIncludedChat(jid) {
		return errors.New("invalid or unavailable chat")
	}
	info := &types.MessageInfo{
		MessageSource: types.MessageSource{Chat: jid, IsFromMe: message.FromMe, IsGroup: conversation.Kind == "group"},
		ID:            types.MessageID(message.ID),
		Timestamp:     time.Unix(message.Timestamp, 0),
	}
	_, err = w.client.SendPeerMessage(ctx, w.client.BuildHistorySyncRequest(info, count))
	return err
}

func (w *WhatsApp) syncGroupMetadata(ctx context.Context) error {
	groups, err := w.client.GetJoinedGroups(ctx)
	if err != nil {
		return err
	}
	if err := w.store.ResetGroupPins(ctx); err != nil {
		return err
	}
	metadata := make(map[string]groupMetadata, len(groups))
	for _, group := range groups {
		settings, settingsErr := w.client.Store.ChatSettings.GetChatSettings(ctx, group.JID)
		pinned := settingsErr == nil && settings.Pinned
		included := w.policy.Includes(group.Name, pinned)
		item := groupMetadata{Name: group.Name, Pinned: pinned, Included: included}
		metadata[group.JID.String()] = item
		// Always persist current metadata. Otherwise an excluded group renamed
		// from an allowlisted name could remain visible through its stale row.
		if err := w.store.UpsertConversation(ctx, group.JID.String(), group.Name, "group", pinned); err != nil {
			return err
		}
	}
	w.mu.Lock()
	w.groups = metadata
	w.mu.Unlock()
	return nil
}

func (w *WhatsApp) groupMetadata(jid types.JID) (groupMetadata, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	metadata, ok := w.groups[jid.String()]
	return metadata, ok
}

func (w *WhatsApp) isIncludedChat(jid types.JID) bool {
	if isDirectJID(jid) {
		return true
	}
	if jid.Server != types.GroupServer {
		return false
	}
	metadata, ok := w.groupMetadata(jid)
	return ok && metadata.Included
}

func (w *WhatsApp) contactName(ctx context.Context, jid types.JID, fallback string) string {
	if name, ok := w.lookupContactName(ctx, jid); ok {
		return name
	}
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	return jid.User
}

func (w *WhatsApp) lookupContactName(ctx context.Context, jid types.JID) (string, bool) {
	candidates := []types.JID{jid}
	if jid.Server == types.HiddenUserServer {
		if phoneJID, err := w.client.Store.LIDs.GetPNForLID(ctx, jid); err == nil && !phoneJID.IsEmpty() {
			candidates = append([]types.JID{phoneJID}, candidates...)
		}
	}
	for _, candidateJID := range candidates {
		if contact, err := w.client.Store.Contacts.GetContact(ctx, candidateJID); err == nil {
			for _, name := range []string{contact.FullName, contact.FirstName, contact.BusinessName, contact.PushName} {
				if strings.TrimSpace(name) != "" {
					return name, true
				}
			}
		}
	}
	return "", false
}

func isDirectJID(jid types.JID) bool {
	return jid.Server == types.DefaultUserServer || jid.Server == types.HiddenUserServer
}

func rawMessageText(message *waE2E.Message) string {
	if message.GetConversation() != "" {
		return message.GetConversation()
	}
	if extended := message.GetExtendedTextMessage(); extended != nil {
		return extended.GetText()
	}
	if image := message.GetImageMessage(); image != nil {
		return image.GetCaption()
	}
	return ""
}

func (w *WhatsApp) imageMedia(ctx context.Context, messageID string, message *waE2E.Message) Message {
	image := message.GetImageMessage()
	if image == nil {
		return Message{}
	}
	media := Message{
		MediaType:   "image",
		MediaMime:   strings.TrimSpace(image.GetMimetype()),
		MediaWidth:  int(image.GetWidth()),
		MediaHeight: int(image.GetHeight()),
	}
	if media.MediaMime == "" {
		media.MediaMime = "image/jpeg"
	}
	data := image.GetJPEGThumbnail()
	if image.GetFileLength() <= 15*1024*1024 {
		downloadCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		full, err := w.client.Download(downloadCtx, image)
		cancel()
		if err == nil && len(full) > 0 && len(full) <= 15*1024*1024 {
			data = full
		}
	}
	if len(data) == 0 {
		return media
	}
	sum := sha256.Sum256([]byte(messageID))
	filename := hex.EncodeToString(sum[:]) + ".image"
	if err := os.WriteFile(filepath.Join(w.mediaDir, filename), data, 0600); err != nil {
		return media
	}
	media.MediaPath = filename
	return media
}

func (w *WhatsApp) messageMedia(ctx context.Context, messageID string, message *waE2E.Message) Message {
	if message.GetImageMessage() != nil {
		return w.imageMedia(ctx, messageID, message)
	}
	return w.audioMedia(ctx, messageID, message)
}

func (w *WhatsApp) audioMedia(ctx context.Context, messageID string, message *waE2E.Message) Message {
	audio := message.GetAudioMessage()
	if audio == nil {
		return Message{}
	}
	mediaType := "audio"
	if audio.GetPTT() {
		mediaType = "voice"
	}
	media := Message{
		MediaType: mediaType, MediaMime: strings.TrimSpace(audio.GetMimetype()),
		MediaDuration: int(audio.GetSeconds()),
	}
	if media.MediaMime == "" {
		media.MediaMime = "audio/ogg; codecs=opus"
	}
	if audio.GetFileLength() == 0 || audio.GetFileLength() > 10*1024*1024 {
		return media
	}
	downloadCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	data, err := w.client.Download(downloadCtx, audio)
	cancel()
	if err != nil || len(data) == 0 || len(data) > 10*1024*1024 {
		return media
	}
	sum := sha256.Sum256([]byte(messageID))
	filename := hex.EncodeToString(sum[:]) + ".audio"
	if err := os.WriteFile(filepath.Join(w.mediaDir, filename), data, 0600); err != nil {
		return media
	}
	media.MediaPath = filename
	return media
}

func mentionedJIDs(message *waE2E.Message) []string {
	if extended := message.GetExtendedTextMessage(); extended != nil {
		return extended.GetContextInfo().GetMentionedJID()
	}
	return nil
}

func (w *WhatsApp) messageText(ctx context.Context, message *waE2E.Message) string {
	text := rawMessageText(message)
	for _, rawJID := range mentionedJIDs(message) {
		jid, err := types.ParseJID(rawJID)
		if err != nil {
			continue
		}
		if name, ok := w.lookupContactName(ctx, jid); ok {
			text = replaceMentionUser(text, jid.User, name)
		}
	}
	return text
}

func replaceMentionUser(text, user, name string) string {
	if user == "" || name == "" || name == user {
		return text
	}
	pattern := regexp.MustCompile(`@` + regexp.QuoteMeta(user) + `\b`)
	return pattern.ReplaceAllStringFunc(text, func(string) string { return "@" + name })
}

func (w *WhatsApp) backfillMentionNames(ctx context.Context) error {
	messages, err := w.store.GroupMessagesContainingMentions(ctx)
	if err != nil {
		return err
	}
	for _, message := range messages {
		updated := numericMentionPattern.ReplaceAllStringFunc(message.Text, func(token string) string {
			user := strings.TrimPrefix(token, "@")
			for _, server := range []string{types.HiddenUserServer, types.DefaultUserServer} {
				if name, ok := w.lookupContactName(ctx, types.NewJID(user, server)); ok {
					return "@" + name
				}
			}
			return token
		})
		if err := w.store.UpdateMessageText(ctx, message.ID, message.Text, updated); err != nil {
			return err
		}
	}
	return nil
}
