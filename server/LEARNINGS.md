# Learnings

## 2026-08-15 — Public bridge server

- Group visibility is runtime policy, not a database migration: direct chats are always visible; group inclusion uses actual WhatsApp pin state, `GROUP_MODE`, and normalized exact allowlist names.
- The setup URL carries only `SETUP_TOKEN`; the app API credential is contained only in the app-configuration QR payload after WhatsApp connects.
