package bridge

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/skip2/go-qrcode"
)

type Sender interface {
	Send(ctx context.Context, conversation StoredConversation, text string) (Message, error)
	SendVoice(ctx context.Context, conversation StoredConversation, audio []byte, durationSeconds int) (Message, error)
}

type StatusProvider interface {
	Status() PairingStatus
	QRPNG() []byte
}

type HistoryRequester interface {
	RequestHistory(ctx context.Context, conversationID string, count int, recent bool) error
}

type API struct {
	store         *Store
	sender        Sender
	status        StatusProvider
	history       HistoryRequester
	mediaDir      string
	publicBaseURL string
	apiToken      string
	setupToken    string
	setupEnabled  bool
	apiFailures   *authRateLimiter
	setupFailures *authRateLimiter
}

func NewAPI(store *Store, sender Sender, status StatusProvider, history HistoryRequester, mediaDir, publicBaseURL, apiToken, setupToken string, setupEnabled ...bool) http.Handler {
	enabled := true
	if len(setupEnabled) > 0 {
		enabled = setupEnabled[0]
	}
	api := &API{
		store: store, sender: sender, status: status, history: history, mediaDir: mediaDir,
		publicBaseURL: publicBaseURL, apiToken: apiToken, setupToken: setupToken, setupEnabled: enabled,
		apiFailures: newAuthRateLimiter(5, time.Minute), setupFailures: newAuthRateLimiter(5, time.Minute),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", api.health)
	mux.Handle("GET /setup", api.requireSetup(http.HandlerFunc(api.setupPage)))
	mux.Handle("GET /setup/status", api.requireSetup(http.HandlerFunc(api.setupStatus)))
	mux.Handle("GET /setup/qr.png", api.requireSetup(http.HandlerFunc(api.setupQR)))
	mux.Handle("GET /setup/config-qr.png", api.requireSetup(http.HandlerFunc(api.setupConfigQR)))
	mux.Handle("GET /api/v1/status", api.requireAPI(http.HandlerFunc(api.apiStatus)))
	mux.Handle("GET /api/v1/conversations", api.requireAPI(http.HandlerFunc(api.conversations)))
	mux.Handle("GET /api/v1/conversations/{id}/messages", api.requireAPI(http.HandlerFunc(api.messages)))
	mux.Handle("POST /api/v1/conversations/{id}/messages", api.requireAPI(http.HandlerFunc(api.sendMessage)))
	mux.Handle("POST /api/v1/conversations/{id}/voice", api.requireAPI(http.HandlerFunc(api.sendVoice)))
	mux.Handle("POST /api/v1/conversations/{id}/read", api.requireAPI(http.HandlerFunc(api.markRead)))
	mux.Handle("POST /api/v1/conversations/{id}/history", api.requireAPI(http.HandlerFunc(api.requestHistory)))
	mux.Handle("GET /api/v1/messages/{id}/media", api.requireAPI(http.HandlerFunc(api.messageMedia)))
	return securityHeaders(mux)
}

func (a *API) messageMedia(w http.ResponseWriter, r *http.Request) {
	path, mime, err := a.store.MessageMedia(r.Context(), r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "message media not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load message media")
		return
	}
	if filepath.Base(path) != path {
		writeError(w, http.StatusInternalServerError, "invalid media path")
		return
	}
	file, err := os.Open(filepath.Join(a.mediaDir, path))
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "message media not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load message media")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load message media")
		return
	}
	w.Header().Set("Content-Type", mime)
	http.ServeContent(w, r, path, info.ModTime(), file)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func (a *API) requireAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !constantTimeTokenEqual(provided, a.apiToken) {
			a.rejectUnauthorized(w, r, a.apiFailures)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *API) requireSetup(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.setupEnabled {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		if !constantTimeTokenEqual(r.URL.Query().Get("token"), a.setupToken) {
			a.rejectUnauthorized(w, r, a.setupFailures)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func constantTimeTokenEqual(provided, expected string) bool {
	providedHash := sha256.Sum256([]byte(provided))
	expectedHash := sha256.Sum256([]byte(expected))
	return expected != "" && subtle.ConstantTimeCompare(providedHash[:], expectedHash[:]) == 1
}

type authRateLimiter struct {
	mu      sync.Mutex
	entries map[string]authRateLimitEntry
	limit   int
	window  time.Duration
}

type authRateLimitEntry struct {
	attempts int
	resetAt  time.Time
}

func newAuthRateLimiter(limit int, window time.Duration) *authRateLimiter {
	return &authRateLimiter{entries: make(map[string]authRateLimitEntry), limit: limit, window: window}
}

func (l *authRateLimiter) allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry := l.entries[ip]
	if entry.resetAt.IsZero() || !now.Before(entry.resetAt) {
		entry = authRateLimitEntry{resetAt: now.Add(l.window)}
	}
	if entry.attempts >= l.limit {
		return false
	}
	entry.attempts++
	l.entries[ip] = entry
	return true
}

func (a *API) rejectUnauthorized(w http.ResponseWriter, r *http.Request, limiter *authRateLimiter) {
	if !limiter.allow(clientIP(r), time.Now()) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "too many unauthorized attempts")
		return
	}
	writeError(w, http.StatusUnauthorized, "unauthorized")
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	if r.RemoteAddr != "" {
		return r.RemoteAddr
	}
	return "unknown"
}

func (a *API) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) apiStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, a.status.Status())
}

func (a *API) setupStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.status.Status())
}

func (a *API) setupQR(w http.ResponseWriter, r *http.Request) {
	qr := a.status.QRPNG()
	if len(qr) == 0 {
		writeError(w, http.StatusNotFound, "QR code is not ready")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(qr)
}

func (a *API) setupConfigQR(w http.ResponseWriter, _ *http.Request) {
	if !a.status.Status().Connected {
		writeError(w, http.StatusConflict, "WhatsApp is not connected")
		return
	}
	payload, err := a.configQRPayload()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to generate app configuration QR code")
		return
	}
	qr, err := qrcode.Encode(string(payload), qrcode.Medium, 420)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to generate app configuration QR code")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(qr)
}

func (a *API) configQRPayload() ([]byte, error) {
	return json.Marshal(struct {
		Type     string `json:"type"`
		Version  int    `json:"version"`
		BaseURL  string `json:"baseUrl"`
		APIToken string `json:"apiToken"`
	}{
		Type: "org.lp3bridge.whatsapp.config", Version: 1, BaseURL: a.publicBaseURL, APIToken: a.apiToken,
	})
}

var setupTemplate = template.Must(template.New("setup").Parse(`<!doctype html>
<html><head><meta name="viewport" content="width=device-width,initial-scale=1"><title>Light WhatsApp setup</title>
<style>body{font:18px system-ui;max-width:620px;margin:40px auto;padding:0 18px;color:#151515}img{width:min(420px,100%);image-rendering:crisp-edges}.box{border:1px solid #bbb;padding:20px;border-radius:12px;margin:16px 0}</style></head>
<body><h1>Link WhatsApp</h1><div class="box"><p id="state">Waiting for bridge…</p><img id="whatsapp-qr" alt="WhatsApp linking QR code" hidden></div>
<p>On your primary phone open <strong>WhatsApp → Settings → Linked devices → Link a device</strong>, then scan this code.</p>
<section id="app-config" class="box" hidden><h2>Configure your Light Phone</h2><p>Scan this code in the Light WhatsApp app.</p><img id="config-qr" alt="Light WhatsApp app configuration QR code" hidden></section>
<script>const token=new URLSearchParams(window.location.search).get('token')||'',state=document.querySelector('#state'),whatsappQR=document.querySelector('#whatsapp-qr'),appConfig=document.querySelector('#app-config'),configQR=document.querySelector('#config-qr');
async function poll(){try{const r=await fetch('/setup/status?token='+encodeURIComponent(token));const s=await r.json();state.textContent='Status: '+s.state+(s.error?' — '+s.error:'');if(s.connected){whatsappQR.hidden=true;appConfig.hidden=false;configQR.hidden=false;configQR.src='/setup/config-qr.png?token='+encodeURIComponent(token);state.textContent='Connected. Scan the app configuration QR code below.'}else if(s.state==='waiting_for_scan'){appConfig.hidden=true;whatsappQR.hidden=false;whatsappQR.src='/setup/qr.png?token='+encodeURIComponent(token)}}catch(e){state.textContent='Bridge unavailable'}setTimeout(poll,2000)}poll()</script></body></html>`))

func (a *API) setupPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = setupTemplate.Execute(w, nil)
}

func (a *API) conversations(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.Conversations(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load conversations")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversations": items})
}

func (a *API) messages(w http.ResponseWriter, r *http.Request) {
	before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	conversation, err := a.store.ConversationByID(r.Context(), r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "conversation not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load conversation")
		return
	}
	items, err := a.store.Messages(r.Context(), conversation.ID, before, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load messages")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversation": conversation.Conversation, "messages": items})
}

func (a *API) sendMessage(w http.ResponseWriter, r *http.Request) {
	conversation, err := a.store.ConversationByID(r.Context(), r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "conversation not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load conversation")
		return
	}
	var request struct {
		Text string `json:"text"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	request.Text = strings.TrimSpace(request.Text)
	if request.Text == "" || len([]rune(request.Text)) > 4096 {
		writeError(w, http.StatusBadRequest, "text must be between 1 and 4096 characters")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	message, err := a.sender.Send(ctx, conversation, request.Text)
	if err != nil {
		writeError(w, http.StatusBadGateway, "send failed")
		return
	}
	writeJSON(w, http.StatusCreated, message)
}

const maxVoiceNoteBytes = 5 * 1024 * 1024

func (a *API) sendVoice(w http.ResponseWriter, r *http.Request) {
	conversation, err := a.store.ConversationByID(r.Context(), r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "conversation not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load conversation")
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "audio/ogg" {
		writeError(w, http.StatusUnsupportedMediaType, "voice note must be Ogg/Opus audio")
		return
	}
	duration, err := strconv.Atoi(r.Header.Get("X-Voice-Duration-Seconds"))
	if err != nil || duration < 1 || duration > 120 {
		writeError(w, http.StatusBadRequest, "voice note duration must be between 1 and 120 seconds")
		return
	}
	if r.ContentLength > maxVoiceNoteBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "voice note exceeds 5 MiB")
		return
	}
	audio, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxVoiceNoteBytes))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "voice note exceeds 5 MiB")
		return
	}
	if len(audio) == 0 {
		writeError(w, http.StatusBadRequest, "voice note is empty")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	message, err := a.sender.SendVoice(ctx, conversation, audio, duration)
	if err != nil {
		writeError(w, http.StatusBadGateway, "send failed")
		return
	}
	writeJSON(w, http.StatusCreated, message)
}

func (a *API) markRead(w http.ResponseWriter, r *http.Request) {
	if err := a.store.MarkRead(r.Context(), r.PathValue("id")); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "unable to update conversation")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) requestHistory(w http.ResponseWriter, r *http.Request) {
	if _, err := a.store.ConversationByID(r.Context(), r.PathValue("id")); errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "conversation not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load conversation")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	recent := r.URL.Query().Get("mode") == "recent"
	count := 50
	if recent {
		count = 100
	}
	if err := a.history.RequestHistory(ctx, r.PathValue("id"), count, recent); err != nil {
		writeError(w, http.StatusBadGateway, "history request failed")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "requested"})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
