package bridge

import (
	"testing"

	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func TestMentionedJIDs(t *testing.T) {
	message := &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
		Text: proto.String("@123456 hello"),
		ContextInfo: &waE2E.ContextInfo{
			MentionedJID: []string{"123456@lid"},
		},
	}}
	got := mentionedJIDs(message)
	if len(got) != 1 || got[0] != "123456@lid" {
		t.Fatalf("unexpected mentioned JIDs: %#v", got)
	}
}

func TestNewVoiceMessageUsesWhatsAppVoiceNoteFields(t *testing.T) {
	uploaded := whatsmeow.UploadResponse{
		URL: "https://example.test/audio", DirectPath: "/audio", MediaKey: []byte("key"),
		FileEncSHA256: []byte("enc"), FileSHA256: []byte("plain"), FileLength: 123,
	}
	message := newVoiceMessage(uploaded, 8)
	if !message.GetPTT() || message.GetMimetype() != "audio/ogg; codecs=opus" || message.GetSeconds() != 8 || message.GetFileLength() != 123 {
		t.Fatalf("unexpected voice message: %#v", message)
	}
}

func TestReplaceMentionUser(t *testing.T) {
	got := replaceMentionUser("@123456 send photo", "123456", "Alex")
	if got != "@Alex send photo" {
		t.Fatalf("unexpected resolved mention: %q", got)
	}
}

func TestReplaceMentionUserDoesNotReplacePartialNumber(t *testing.T) {
	got := replaceMentionUser("@1234567 send photo", "123456", "Alex")
	if got != "@1234567 send photo" {
		t.Fatalf("partial numeric mention was replaced: %q", got)
	}
}
