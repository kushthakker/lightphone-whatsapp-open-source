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

## Where to host the bridge

The bridge is not an Android app or a serverless function. It is a long-running
WhatsApp linked device with a persistent SQLite database, so its host must stay
online and keep its Docker volume between upgrades.

| Host | Recommended HTTPS route | Notes |
| --- | --- | --- |
| Linux VPS or cloud VM | Caddy or Cloudflare Tunnel | Simplest always-on option. Caddy needs public DNS and inbound ports 80/443; Tunnel needs only outbound connectivity. |
| Home server, NAS, or Raspberry Pi-class Linux host | Cloudflare Tunnel | Avoids port forwarding, changing residential IPs, and carrier-grade NAT. The machine must not sleep. |
| Always-on desktop with Docker Desktop | Cloudflare Tunnel | Suitable for evaluation; messages stop syncing whenever the computer sleeps or Docker stops. |

A dedicated EC2 instance is **not** required. Any persistent 64-bit Linux host
that runs Docker Engine and Docker Compose 2.24.0 or newer can be used. The
project is built on both `amd64` and `arm64`. Shared hosting and
request-based/serverless platforms are unsuitable because they do not preserve
a continuously connected process and Docker volume.

The host needs outbound internet access and enough persistent disk for its
message archive and downloaded media. Do not expose bridge port 8080 publicly;
only the HTTPS proxy or tunnel should be internet-facing.

## Quick start

### 1. Choose a public HTTPS route

The LP3 must reach the bridge through a stable HTTPS URL. Choose the host first,
then use one of these routes:

- **Cloudflare Tunnel:** works behind NAT without inbound firewall rules. It
  requires a Cloudflare account, a domain in Cloudflare, and a remotely managed
  tunnel whose public hostname routes to `http://bridge:8080`.
- **Caddy:** requires a public DNS record pointing at the host and inbound TCP
  ports 80 and 443. Caddy obtains and renews the certificate automatically.

See [SELF_HOSTING.md](docs/SELF_HOSTING.md) for host preparation, firewall,
deployment, update, backup, and troubleshooting steps. Plain HTTP is not
supported for real messages.

### 2. Generate local secrets

```bash
git clone https://github.com/kushthakker/lightphone-whatsapp-open-source.git
cd lightphone-whatsapp-open-source
./scripts/init-env.sh https://whatsapp.example.com
```

Replace `whatsapp.example.com` with the hostname that will reach this bridge.
Use the same exact origin in Cloudflare or DNS; do not include `/setup` or any
other path.

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
docker compose ps
./scripts/smoke-compose.sh
curl --fail https://whatsapp.example.com/healthz
```

The smoke script verifies the private same-host API and both authorization
boundaries without printing credentials. Run the final `curl` from another
network to prove that an LP3 can reach the public HTTPS route.

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

Download the signed APK and checksum from
[GitHub Releases](https://github.com/kushthakker/lightphone-whatsapp-open-source/releases),
enable ADB on the LP3, connect the phone by USB, verify the checksum, and
install:

Run the checksum command for your computer's operating system, then run the ADB
commands:

```bash
# Linux
sha256sum -c lp3-whatsapp-bridge.apk.sha256

# macOS
shasum -a 256 -c lp3-whatsapp-bridge.apk.sha256
adb devices -l
adb install -r lp3-whatsapp-bridge.apk
```

Open **WhatsApp** on the Light Phone, choose **Scan setup QR**, and scan the
second QR displayed by the server. The app validates both the HTTPS origin and
API token before saving them.

Full checksum/signature verification, ADB activation, installation, update,
recovery, and test steps are in
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
backup mechanism while it is running. Exact backup, restore, host-move, and
upgrade procedures are in [SELF_HOSTING.md](docs/SELF_HOSTING.md).

## Build from source

The public repository contains the Go bridge and the developer-owned Light tool
module; it does not vendor the Light SDK. The build script overlays `tool/` onto
a pinned public Light SDK checkout and creates an APK locally. Release signing
is a separate protected CI job. For local development, use:

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
