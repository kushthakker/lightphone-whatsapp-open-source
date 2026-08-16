# Agent instructions

This repository must remain reproducible, self-hosted, and safe to publish.
These instructions are authoritative for coding agents working here.

## Before changing anything

1. Read `LEARNINGS.md`, `README.md`, `SECURITY.md`, and the relevant file under
   `docs/`.
2. Inspect `git status`; preserve user changes and work on the current branch.
3. Never import files from a private deployment workspace wholesale. Copy only
   reviewed source files and run `scripts/privacy-scan.sh` before staging them.
4. Do not deploy, link a real WhatsApp account, install on a physical phone,
   publish a release, or change repository visibility unless the user requested
   that external action.

## Repository map

- `server/`: Go bridge, WhatsApp session, HTTP API, SQLite archive, Dockerfile.
- `tool/`: the only source accepted by Light's public tool builder.
- `compose.yaml`: self-hosted bridge plus optional HTTPS profiles.
- `deploy/`: portable reverse-proxy configuration only; never personal infra.
- `scripts/`: secret generation, local builds, smoke tests, privacy checks.
- `docs/`: architecture, self-hosting, LP3 installation, and release gates.

## Privacy invariants

The repository, its history, CI output, Docker layers, APKs, and release assets
must never contain:

- personal names, messages, contacts, group titles, phone numbers, JIDs, email
  addresses, hostnames, IP addresses, cloud account/resource IDs, or device IDs;
- API/setup/tunnel tokens, WhatsApp session databases, media, logs, `.env`, or
  QR screenshots;
- APK signing keys, the Light SDK development key, or GitHub credentials;
- private deployment scripts, rendered templates, command transcripts, or
  proof screenshots.

Use fictional fixtures such as `Example Group`, `Example Person`, and syntactic
test identifiers. Maintain personal deny patterns only in an ignored external
file and invoke:

```bash
PRIVATE_DENYLIST_FILE=/absolute/private-denylist.txt ./scripts/privacy-scan.sh .
```

Do not print matched secret values; scanners should report filenames only.

## Server contract

Required environment variables:

- `API_TOKEN`: at least 32 characters; authenticates `/api/v1/*`.
- `SETUP_TOKEN`: distinct, at least 32 characters; authenticates `/setup*`.
- `SETUP_ENABLED`: strictly `true` or `false`; disable setup after provisioning.
- `PUBLIC_BASE_URL`: the externally reachable HTTPS origin encoded in the app
  configuration QR.

Runtime paths derive from `DATABASE_PATH` (normally `/data/bridge.db`). Keep its
parent volume intact because it contains SQLite DB/WAL/SHM, whatsmeow device
state, media, and push state. Do not add a schema migration unless the task
explicitly requires it.

Direct chats are always included. Groups follow the immutable policy configured
by `GROUP_MODE` and `GROUP_ALLOWLIST`; policy must be injected consistently into
ingestion and API listing so archived groups cannot bypass it.

Run after server changes:

```bash
cd server
gofmt -w ./cmd ./internal
go test ./...
go vet ./...
cd ..
docker compose config
```

## Tool contract

- Tool ID is `org.lp3bridge.whatsapp`.
- No URL, token, personal fallback, or release key may be compiled into the APK.
- First run scans the versioned configuration QR. Manual URL/token entry is a
  fallback, not the primary path.
- Require HTTPS, validate `/healthz` and authenticated `/api/v1/status`, then
  save URL and encrypted token atomically through `SealedLightContext.dataStore`.
- Never render a saved token. A 401 must lead to a recoverable reconfiguration
  path without clearing unrelated app state.
- `LightEntryPoint` has no sanctioned DataStore context. Cache push endpoint
  there and register it after the foreground screen loads configuration.
- Preserve portrait orientation, bounded media memory, temporary recording
  cleanup, and message-ID reconciliation.

Run after tool changes:

```bash
./scripts/build-apk-local.sh
```

Then scan the APK and generated files with `scripts/privacy-scan.sh` and verify
that no bridge URL/token exists in `BuildConfig` or resources.

## End-to-end proof gates

Do not collapse these into one claim:

1. Local Go tests and Android unit/lint/build pass.
2. Fresh Compose volume starts; auth/setup matrix passes; graceful and abrupt
   recreation preserve the database and linked session.
3. A dedicated non-personal WhatsApp test account links and sends/receives text,
   image, and voice in both directions after a server restart.
4. A physical LP3 installs the signed APK, scans config QR, survives process
   death/reboot, recovers from invalid credentials, reaches the supported HTTPS
   route, and repeats all six message-direction cases.
5. Two consecutively signed APK versions install-over successfully and preserve
   DataStore configuration.
6. A clean clone and AGENTS dry-run reproduce the build before public release.

Keep manual evidence private and record only sanitized pass/fail receipts.

## Release and publication

Release APKs are signed only with the stable protected CI keystore. Never ship a
debug-signed APK as a release. Loss of the release key breaks install-over
upgrades and must be documented.

Before any push intended for public visibility:

1. Run all tests, Compose smoke checks, gitleaks, privacy scan, APK strings scan,
   binary scan, Docker history scan, and dependency-license review.
2. Inspect the entire staged diff and full Git history, not only the worktree.
3. Build from a clean clone and follow README/AGENTS without private context.
4. Publish privately first, require green CI, inspect rendered docs/assets, then
   make public only when explicitly authorized.
5. If the post-public clone/download smoke fails, make the repository private
   and revoke the affected release before fixing it.

## After each work session

Append concise, dated, non-personal findings to `LEARNINGS.md`. Include quirks,
decisions, failed approaches, and exact verification boundaries.
