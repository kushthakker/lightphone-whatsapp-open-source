# LP3 WhatsApp Bridge

An unofficial, self-hosted WhatsApp companion for the Light Phone III. A Go
bridge stays connected as a WhatsApp linked device and exposes a small HTTPS
API. One generic Light SDK APK connects to any bridge by scanning a protected
configuration QR code; the server URL and API token are never compiled into the
APK.

> [!WARNING]
> This project is not affiliated with Light or WhatsApp. It uses the unofficial
> `whatsmeow` linked-device implementation. WhatsApp may change its protocol or
> restrict accounts. Use a dedicated test account first and keep backups.

## Implemented; release validation pending

- QR linking from a browser using WhatsApp **Linked devices**.
- Direct chats and WhatsApp-pinned groups; optional server-side group allowlist.
- Conversation search and history.
- Sending and receiving text, images, and Ogg/Opus voice notes.
- Runtime bridge provisioning on the LP3 by QR, with encrypted token storage.
- Docker Compose deployment behind Cloudflare Tunnel or Caddy HTTPS.

These paths have automated and local test coverage, but the exact public release
artifact has not yet passed the dedicated-account and physical-LP3 matrix.

The pinned public Light SDK can deliver push bytes to a tool but currently has
no sanctioned API for a tool to display a system notification. This release
therefore refreshes while open and on resume; background message alerts are not
claimed as supported.

The repository is under active release validation. Automated and manual gate
status is recorded in [RELEASING.md](docs/RELEASING.md); do not interpret source
compilation alone as end-to-end proof.

## Architecture

```text
Primary WhatsApp phone
        │ WhatsApp linked-device protocol
        ▼
Go + whatsmeow bridge ─── SQLite/WAL + media on your server
        │ HTTPS + API bearer token
        ▼
Generic Light SDK tool on Light Phone III
```

Raw WhatsApp JIDs are retained only in the server database. The LP3 API uses
opaque conversation IDs. The server URL and API token are configured at first
run and are not present in the downloadable APK.

## Quick start

### 1. Choose a public HTTPS route

The LP3 must reach the bridge through a stable HTTPS URL.

- **Cloudflare Tunnel:** works behind NAT without inbound firewall rules. It
  requires a Cloudflare account, a domain in Cloudflare, and a remotely managed
  tunnel whose public hostname routes to `http://bridge:8080`.
- **Caddy:** requires a public DNS record pointing at the host and inbound TCP
  ports 80 and 443. Caddy obtains and renews the certificate automatically.

See [SELF_HOSTING.md](docs/SELF_HOSTING.md) before choosing. Plain HTTP is not
supported for real messages.

### 2. Generate local secrets

```bash
git clone YOUR_REPOSITORY_CLONE_URL
cd lightphone-whatsapp
./scripts/init-env.sh https://whatsapp.example.com
```

Copy the clone URL from this repository's **Code** button.

This creates a mode-`0600` `.env` with independent random API and setup tokens.
It refuses to overwrite an existing file. Keep the printed setup URL private.

For Cloudflare Tunnel, add its tunnel token to `.env`, configure the tunnel
hostname to route to `http://bridge:8080`, then run:

```bash
docker compose --profile cloudflare up -d --build
```

For a public-DNS Caddy deployment:

```bash
docker compose --profile caddy up -d --build
```

Verify:

```bash
curl --fail https://whatsapp.example.com/healthz
docker compose ps
./scripts/smoke-compose.sh
```

### 3. Link WhatsApp

Open the private setup URL printed by `init-env.sh`. On the primary phone open:

**WhatsApp → Settings → Linked devices → Link a device**

Scan the WhatsApp QR. When the page reports that the bridge is connected, it
shows a second QR for configuring the LP3 app. Do not photograph or share that
second QR: it contains the bridge API credential.

After the LP3 is configured, set `SETUP_ENABLED=false` in `.env` and recreate
the bridge so Compose reloads the environment:

```bash
docker compose up -d --force-recreate bridge
```

Set it back to `true` only when linking or configuring a device. A plain
`docker compose restart bridge` does not reload `.env`.

### 4. Install and configure the LP3 app

Download the signed APK from the GitHub Release, enable ADB on the LP3, connect
the phone by USB, and install:

```bash
adb devices -l
adb install -r lp3-whatsapp-bridge.apk
```

Open **WhatsApp** on the Light Phone, choose **Scan setup QR**, and scan the
second QR displayed by the server. The app validates both the HTTPS origin and
API token before saving them.

Full ADB activation, installation, recovery, and test steps are in
[LP3_INSTALL.md](docs/LP3_INSTALL.md).

## Group visibility

Direct chats are always visible. By default, a group is visible only when it is
pinned in WhatsApp. Optional configuration:

```dotenv
GROUP_MODE=pinned
GROUP_ALLOWLIST=["Example Family","Project Team"]
```

`GROUP_MODE=all` exposes every joined group. Allowlist matching is exact after
case and whitespace normalization. It never force-pins a group in the LP3 UI.

## Data and backups

The `bridge-data` Docker volume holds the WhatsApp device identity/session,
message archive, SQLite WAL/SHM files, and downloaded media.
Deleting that volume deletes bridge history and normally requires linking
again. Stop the bridge before a filesystem-level backup, or use SQLite's online
backup mechanism while it is running.

## Build from source

The public repository contains only the Light tool module. The official Light
builder overlays it onto a pinned Light SDK checkout and creates an unsigned
release artifact for the separate signing job. For local development, use:

```bash
./scripts/build-apk-local.sh
```

The pinned SDK resolves its dependencies from public repositories; this build
does not require a GitHub token. It requires Java 17 and an Android SDK; the
script auto-detects Homebrew `openjdk@17` and `android-commandlinetools`.

Server checks:

```bash
cd server
go test ./...
go vet ./...
```

## Security

Read [SECURITY.md](SECURITY.md) before exposing the bridge. The short version:

- use HTTPS;
- keep API and setup tokens independent;
- treat the setup/configuration page as a secret;
- encrypt and back up the server disk;
- never commit `.env`, WhatsApp databases, media, APK signing keys, or logs.

## License

Project-owned source is MIT licensed. Dependencies retain their own licenses;
see [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md). In particular, the
server's transitive Signal implementation is GPL-3.0: distributing the
statically linked server binary, including inside a container, requires GPLv3
compliance and corresponding source for the covered work. The APK does not
contain the Go server dependencies.
