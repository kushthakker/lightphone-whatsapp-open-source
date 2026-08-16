# Learnings

Project-specific implementation notes belong here. Keep entries concise, dated, and free of personal data, credentials, message content, hostnames, IPs, or device identifiers.

## 2026-08-15 — clean-room public repository

- The public repository must be created from an explicit source allowlist. Never initialize Git in, archive, or publish the private deployment workspace.
- The Light SDK is MIT licensed and its public builder accepts only `tool/build.gradle.kts`, `tool/lighttool.toml`, and `tool/src/main/**`; the public repo therefore does not need to vendor SDK source or its development signing key.
- Runtime provisioning uses a setup-token-protected configuration QR. The stock Light SDK QR scanner reads the bridge URL and API token; Preferences DataStore persists encrypted configuration without compiling secrets into the APK.
- The bridge uses `mattn/go-sqlite3`, so release containers require CGO and a matching glibc builder/runtime. Persist the database parent directory so SQLite WAL/SHM files, whatsmeow session tables, media, and push state survive recreation.
- Keep the server configuration-QR schema and the Android parser contract identical; both use the versioned `org.lp3bridge.whatsapp.config` payload with `baseUrl` and `apiToken` fields.
- The pinned public SDK exposes push delivery bytes but no sanctioned system-notification UI. This release must claim foreground polling and resume refresh only.
- The pinned SDK resolves from public JitPack repositories, so source builds do not need GitHub Packages credentials. Local scripts should auto-detect Homebrew Java 17 and the Android command-line SDK.
- Debug APKs include SDK development assets and are much larger than minified signed releases; validate storage impact against the signed release artifact, which is currently about 28 MB.
- A cold ARM64 Docker build downloads roughly 309 MB of Debian/Go base layers. Preserve BuildKit cache; switching libc or removing validation tasks is not a safe latency fix.
- Persist current metadata for excluded groups too. Otherwise a group renamed after being allowlisted can retain its old visible name and expose archived messages.
- Never embed private deny-list values in tests, even as reversible byte arrays. Load private patterns only from an ignored external file.
- `whatsmeow` links `go.mau.fi/libsignal` under GPL-3.0. Keep project source under a compatible license, publish the dependency report, and do not distribute a compiled server/container without satisfying GPLv3 corresponding-source obligations.
- `android-actions/setup-android` installs build tools but does not guarantee `apksigner` is on `PATH`; release automation must resolve it from the newest `$ANDROID_HOME/build-tools/*/apksigner`.
- Limit CI pushes to the main branch so release tags do not start a duplicate full CI run, and mark hyphenated release-candidate tags as GitHub prereleases.
- Never `source` the user-edited Compose `.env` in diagnostics. Read only required keys as literal text so group names or other values cannot execute shell syntax.
- `docker compose restart` does not reload values changed in `.env`; use `docker compose up -d --force-recreate bridge` when toggling setup routes or other runtime configuration.
- A release workflow must reject tags not pointing to current `main` and rerun its own validation before publishing; a green branch run alone does not bind evidence to the tag.
- The Compose smoke test should derive its expected setup-route status from `SETUP_ENABLED`, so the same command proves both enabled setup and post-pairing 404 behavior.
- Secret-bearing `curl` diagnostics must start with `--disable` so a user `.curlrc` cannot enable verbose or trace output and print credentials.
- When reachable Git metadata must be sanitized, create one new root commit from the tracked publication tree, preserve repository-level signing secrets, and remove old releases, tags, and workflow runs before changing visibility.

## 2026-08-16

- Rewriting a private repository does not guarantee that old objects are immediately unreachable by known SHA. For a strict no-history publication boundary, create a new private repository from the sanitized tree, use a new signing key and generic commit/tag identity, validate its sole commit, and keep the original repository private.
- On the physical LP3, rendering the same action as both an inline `LightText` button and a `LightBottomBar` item produces two prominent controls. Use the bottom bar as the sole first-run setup action.
- Test fixtures that look like opaque synthetic hashes can still be copied from live WhatsApp records. Before publication, compare every runtime conversation ID, display name, sender name, and sufficiently long message string against the candidate tree; replace matches with unmistakably synthetic values and start a new object database.
- Git commit objects include author and committer timezone offsets. For a strict no-personal-metadata public history, create the publication commit with explicit UTC author and committer dates, then verify the raw commit object before changing visibility.
- Portable hosting documentation must state the minimum Compose version implied by `compose.yaml` and treat the persistent Docker volume plus ignored `.env` as one recoverable deployment state.
