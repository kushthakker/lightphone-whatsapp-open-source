package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skip2/go-qrcode"
)

type fakeWhatsApp struct{}
type disconnectedWhatsApp struct{ fakeWhatsApp }

func (fakeWhatsApp) Status() PairingStatus {
	return PairingStatus{State: "connected", Connected: true, Paired: true}
}

func (disconnectedWhatsApp) Status() PairingStatus {
	return PairingStatus{State: "waiting_for_scan", Connected: false}
}

func TestMessageMediaRequiresAuthAndServesStoredImage(t *testing.T) {
	store, err := OpenStore("file:"+filepath.Join(t.TempDir(), "media.db")+"?_foreign_keys=on", DefaultGroupPolicy())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	mediaDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(mediaDir, "message.image"), []byte("image-bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertMessage(context.Background(), "person@example.test", "Person", "direct", false, Message{
		ID: "image-1", Timestamp: 100, Text: "[Image]", Status: "sent", MediaType: "image", MediaMime: "image/jpeg", MediaPath: "message.image",
	}, 1); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("a", 32)
	handler := NewAPI(store, fakeWhatsApp{}, fakeWhatsApp{}, fakeWhatsApp{}, mediaDir, "https://bridge.example.test", token, strings.Repeat("b", 32))

	request := httptest.NewRequest(http.MethodGet, "/api/v1/messages/image-1/media", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "image-bytes" {
		t.Fatalf("unexpected media response %d: %q", response.Code, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("unexpected media content type: %s", response.Header().Get("Content-Type"))
	}
}
func (fakeWhatsApp) QRPNG() []byte                                           { return []byte("png") }
func (fakeWhatsApp) RequestHistory(context.Context, string, int, bool) error { return nil }
func (fakeWhatsApp) Send(_ context.Context, conversation StoredConversation, text string) (Message, error) {
	return Message{ID: "sent-1", ConversationID: conversation.ID, FromMe: true, Timestamp: 200, Text: text, Status: "sent"}, nil
}
func (fakeWhatsApp) SendVoice(_ context.Context, conversation StoredConversation, _ []byte, duration int) (Message, error) {
	return Message{ID: "voice-1", ConversationID: conversation.ID, FromMe: true, Timestamp: 200, Text: "[Voice note]", Status: "sent", MediaType: "voice", MediaDuration: duration}, nil
}

func TestAPIAuthorizationAndConversationFlow(t *testing.T) {
	store, err := OpenStore("file:"+filepath.Join(t.TempDir(), "api.db")+"?_foreign_keys=on", DefaultGroupPolicy())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	jid := "person@example.test"
	if err := store.UpsertMessage(context.Background(), jid, "Person", "direct", false, Message{ID: "received-1", Timestamp: 100, Text: "hello", Status: "sent"}, 1); err != nil {
		t.Fatal(err)
	}
	handler := NewAPI(store, fakeWhatsApp{}, fakeWhatsApp{}, fakeWhatsApp{}, t.TempDir(), "https://bridge.example.test", strings.Repeat("a", 32), strings.Repeat("b", 32))

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/conversations", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", unauthorized.Code)
	}

	listRequest := httptest.NewRequest(http.MethodGet, "/api/v1/conversations", nil)
	listRequest.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
	list := httptest.NewRecorder()
	handler.ServeHTTP(list, listRequest)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "Person") {
		t.Fatalf("unexpected list response %d: %s", list.Code, list.Body.String())
	}

	sendRequest := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/"+ConversationID(jid)+"/messages", strings.NewReader(`{"text":"from Light"}`))
	sendRequest.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
	sendRequest.Header.Set("Content-Type", "application/json")
	sent := httptest.NewRecorder()
	handler.ServeHTTP(sent, sendRequest)
	if sent.Code != http.StatusCreated {
		t.Fatalf("unexpected send response %d: %s", sent.Code, sent.Body.String())
	}
	var message Message
	if err := json.Unmarshal(sent.Body.Bytes(), &message); err != nil || message.Text != "from Light" {
		t.Fatalf("unexpected sent message: %#v, %v", message, err)
	}
}

func TestSendVoiceValidatesAndForwardsOggOpus(t *testing.T) {
	store, err := OpenStore("file:"+filepath.Join(t.TempDir(), "voice.db")+"?_foreign_keys=on", DefaultGroupPolicy())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	jid := "person@example.test"
	if err := store.UpsertConversation(context.Background(), jid, "Person", "direct", false); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("a", 32)
	handler := NewAPI(store, fakeWhatsApp{}, fakeWhatsApp{}, fakeWhatsApp{}, t.TempDir(), "https://bridge.example.test", token, strings.Repeat("b", 32))

	request := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/"+ConversationID(jid)+"/voice", strings.NewReader("OggSvoice"))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "audio/ogg; codecs=opus")
	request.Header.Set("X-Voice-Duration-Seconds", "7")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), `"mediaType":"voice"`) || !strings.Contains(response.Body.String(), `"mediaDuration":7`) {
		t.Fatalf("unexpected voice response %d: %s", response.Code, response.Body.String())
	}
}

func TestSetupPageReadsTokenFromCurrentURL(t *testing.T) {
	store, err := OpenStore("file:"+filepath.Join(t.TempDir(), "setup.db")+"?_foreign_keys=on", DefaultGroupPolicy())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	setupToken := strings.Repeat("b", 32)
	apiToken := strings.Repeat("a", 32)
	handler := NewAPI(store, fakeWhatsApp{}, fakeWhatsApp{}, fakeWhatsApp{}, t.TempDir(), "https://bridge.example.test", apiToken, setupToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/setup?token="+setupToken, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("expected setup page, got %d: %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, "new URLSearchParams(window.location.search).get('token')") || strings.Contains(body, setupToken) || strings.Contains(body, apiToken) {
		t.Fatalf("setup page does not safely read its token: %s", body)
	}
}

func TestSetupRoutesCanBeDisabled(t *testing.T) {
	store, err := OpenStore("file:"+filepath.Join(t.TempDir(), "setup-disabled.db")+"?_foreign_keys=on", DefaultGroupPolicy())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := NewAPI(store, fakeWhatsApp{}, fakeWhatsApp{}, fakeWhatsApp{}, t.TempDir(), "https://bridge.example.test", strings.Repeat("a", 32), strings.Repeat("b", 32), false)

	request := httptest.NewRequest(http.MethodGet, "/setup?token="+strings.Repeat("b", 32), nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("disabled setup status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestSetupConfigQRRequiresSetupTokenAndContainsAppConfiguration(t *testing.T) {
	store, err := OpenStore("file:"+filepath.Join(t.TempDir(), "config.db")+"?_foreign_keys=on", DefaultGroupPolicy())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	apiToken := strings.Repeat("a", 32)
	setupToken := strings.Repeat("b", 32)
	api := &API{publicBaseURL: "https://bridge.example.test", apiToken: apiToken}
	payload, err := api.configQRPayload()
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Type     string `json:"type"`
		Version  int    `json:"version"`
		BaseURL  string `json:"baseUrl"`
		APIToken string `json:"apiToken"`
	}
	if err := json.Unmarshal(payload, &config); err != nil {
		t.Fatal(err)
	}
	if config.Type != "org.lp3bridge.whatsapp.config" || config.Version != 1 || config.BaseURL != "https://bridge.example.test" || config.APIToken != apiToken {
		t.Fatalf("unexpected configuration payload: %#v", config)
	}
	handler := NewAPI(store, fakeWhatsApp{}, fakeWhatsApp{}, fakeWhatsApp{}, t.TempDir(), config.BaseURL, apiToken, setupToken)
	disconnected := NewAPI(store, disconnectedWhatsApp{}, disconnectedWhatsApp{}, disconnectedWhatsApp{}, t.TempDir(), config.BaseURL, apiToken, setupToken)
	notConnected := httptest.NewRecorder()
	disconnected.ServeHTTP(notConnected, httptest.NewRequest(http.MethodGet, "/setup/config-qr.png?token="+setupToken, nil))
	if notConnected.Code != http.StatusConflict {
		t.Fatalf("configuration QR was available before WhatsApp connected: %d", notConnected.Code)
	}
	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/setup/config-qr.png?token="+apiToken, nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected setup-token-only protection, got %d", unauthorized.Code)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/setup/config-qr.png?token="+setupToken, nil))
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("unexpected config QR response %d, %s", response.Code, response.Header().Get("Content-Type"))
	}
	if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("config QR lost privacy headers: %#v", response.Header())
	}
	if _, err := png.Decode(strings.NewReader(response.Body.String())); err != nil {
		t.Fatalf("config QR was not a PNG: %v", err)
	}
	expectedQR, err := qrcode.Encode(string(payload), qrcode.Medium, 420)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(response.Body.Bytes(), expectedQR) {
		t.Fatal("config QR does not encode the expected configuration payload")
	}
}

func TestSetupAndAPIAuthenticationAreSeparatedAndRateLimited(t *testing.T) {
	store, err := OpenStore("file:"+filepath.Join(t.TempDir(), "auth.db")+"?_foreign_keys=on", DefaultGroupPolicy())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	apiToken := strings.Repeat("a", 32)
	setupToken := strings.Repeat("b", 32)
	handler := NewAPI(store, fakeWhatsApp{}, fakeWhatsApp{}, fakeWhatsApp{}, t.TempDir(), "https://bridge.example.test", apiToken, setupToken)

	setupAsAPI := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	request.Header.Set("Authorization", "Bearer "+setupToken)
	handler.ServeHTTP(setupAsAPI, request)
	if setupAsAPI.Code != http.StatusUnauthorized {
		t.Fatalf("setup token authenticated API: %d", setupAsAPI.Code)
	}
	apiAsSetup := httptest.NewRecorder()
	handler.ServeHTTP(apiAsSetup, httptest.NewRequest(http.MethodGet, "/setup/status?token="+apiToken, nil))
	if apiAsSetup.Code != http.StatusUnauthorized {
		t.Fatalf("API token authenticated setup: %d", apiAsSetup.Code)
	}

	for attempt := 0; attempt < 4; attempt++ {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/setup/status?token=wrong", nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("unauthorized setup attempt %d returned %d", attempt, response.Code)
		}
	}
	limited := httptest.NewRecorder()
	handler.ServeHTTP(limited, httptest.NewRequest(http.MethodGet, "/setup/status?token=wrong", nil))
	if limited.Code != http.StatusTooManyRequests || limited.Header().Get("Retry-After") == "" {
		t.Fatalf("expected rate limit, got %d with retry-after %q", limited.Code, limited.Header().Get("Retry-After"))
	}
	validAPI := httptest.NewRecorder()
	validRequest := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	validRequest.Header.Set("Authorization", "Bearer "+apiToken)
	handler.ServeHTTP(validAPI, validRequest)
	if validAPI.Code != http.StatusOK {
		t.Fatalf("setup failures affected API authentication: %d", validAPI.Code)
	}
}
