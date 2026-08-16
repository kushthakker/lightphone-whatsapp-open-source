# Releasing

No public release is complete until every gate below has authoritative proof.

## CI gates

- Go unit/race tests and vet.
- Server container build.
- Android unit tests, Light lint, debug assembly, and APK scan.
- Repository privacy scan, full-history gitleaks scan, shell syntax, and Compose
  syntax.
- Tagged release builds must point to current `main`, rerun server tests, server
  container build, privacy/history scans, and syntax checks, then add release
  lint/assembly, signing verification, APK scan, and checksum publication.

## Pre-publication local gates

- Fresh Compose startup and setup/API authorization matrix.
- Graceful and abrupt container recreation with persistent state.
- Generated-file, Docker-layer, binary, APK, archive, workflow, and dependency
  privacy/license scans.

These are release-operator checks, not claims about CI coverage.

## Manual private gates

- Dedicated test account: QR linking and text/image/voice both directions after
  bridge restart.
- Physical LP3: configuration QR, Cloudflare HTTPS path, process death, reboot,
  invalid-credential recovery, and all six message-direction cases.
- Two consecutive builds signed by the same release key install-over while
  retaining encrypted DataStore configuration.
- A clean clone follows README and AGENTS without private workspace context.

## Current release-candidate evidence

| Gate | Status |
| --- | --- |
| Sanitized clean source history and local build | Passed on 2026-08-16; clean root commit and Android/Go/privacy checks passed |
| Clean-repository CI and signed prerelease | Passed on 2026-08-16; CI run `31932407322` and signed `v0.11.2-rc.1` prerelease |
| Local Compose/auth/persistence and privacy scans | Passed on 2026-08-15 |
| Clean clone from the final repository | Passed privately and anonymously after publication on 2026-08-16 |
| Signed APK verification | Passed for `v0.11.2-rc.1`: checksum, package/version, V3 certificate, and privacy scan |
| Dedicated test-account message matrix | Pending |
| Physical LP3 lifecycle | Passed: configuration QR, HTTPS path, process restart, bridge restart, and reboot |
| Physical LP3 six-direction message matrix | Pending |
| Signed install-over test | Passed for two same-package, same-key prerelease builds; final clean-repository APK still requires verification |
| Public clone and public release download | Passed anonymously on 2026-08-16 |

## Signing

GitHub Actions receives the Android keystore, alias, and passwords only through
protected encrypted secrets. The keystore is never committed or uploaded as an
artifact. Back it up offline. Losing it requires users to uninstall before
installing future builds.

Required repository secrets:

- `ANDROID_KEYSTORE_BASE64`: base64 of the stable keystore, without line wraps.
- `ANDROID_STORE_PASSWORD`: keystore password.
- `ANDROID_KEY_ALIAS`: `lightphone-whatsapp-bridge` for the project-maintained
  key.
- `ANDROID_KEY_PASSWORD`: private-key password.

The project-maintained release certificate has SHA-256 fingerprint:

```text
EC:76:CA:B6:27:5C:57:F0:4E:54:71:93:0B:20:A0:25:E9:CA:B6:3D:E0:55:1A:5A:5A:03:97:E3:7E:95:24:65
```

Confirm every release with `apksigner verify --verbose --print-certs` and the
published SHA-256 checksum. Two consecutive tagged versions must install over
one another before the first public release.

## Publication

Create the GitHub repository privately first. Require green CI and complete all
pre-publication scans and manual gates. After explicit authorization, change
visibility to public and download/inspect the release from a clean environment.
If the public smoke test fails or exposes private data, immediately make the
repository private and remove the affected release.

The project does not publish a prebuilt server container. Before adding one,
review `docs/GO_DEPENDENCY_LICENSES.md` and satisfy GPLv3 corresponding-source
obligations for the linked Signal implementation.
