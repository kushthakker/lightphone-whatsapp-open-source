# Architecture

The bridge is the only durable state holder. Its one data directory contains
the whatsmeow linked-device tables, app conversation/message tables, SQLite WAL
files, and media. Caddy or Cloudflare Tunnel terminates public HTTPS and
forwards to the bridge's private port 8080.

The LP3 tool stores only bridge configuration and transient UI/media state. It
polls conversations while visible, downloads bounded media into memory, and
uses one temporary Ogg/Opus file while recording. It never stores the WhatsApp
archive on the phone.

The pinned public SDK does not expose sanctioned system-notification UI to
tools. This release does not register or call a remote push endpoint. The UI
polls while visible and refreshes on resume.

```text
WhatsApp network
      │
      ▼
whatsmeow ─ SQLite/WAL ─ media files
      │
Go HTTP API (private :8080)
      │
HTTPS proxy/tunnel
      │
LP3 tool (API bearer token)
```
