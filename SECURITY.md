# Security policy

## Reporting

Do not open a public issue containing a token, WhatsApp identifier, message,
database, media file, or signing key. Use the repository's private security
advisory channel once the GitHub repository is published.

## Trust boundaries

The bridge has two independent credentials:

- `SETUP_TOKEN` protects WhatsApp QR state and the app-configuration QR.
- `API_TOKEN` authorizes conversation, message, media, send, read, history, and
  push-registration API calls.

Never reuse the values. Both must be random and at least 32 characters. The
configuration QR contains the API token, so anyone who can view or photograph
it can read and send messages through the bridge. Keep the setup URL private,
rotate or disable setup access after linking, and rotate the API token if the QR
or LP3 is exposed.

## Transport

Use a publicly trusted HTTPS certificate. The Go process intentionally serves
plain HTTP only on its private container/loopback network. Do not expose port
8080 to other hosts. The LP3 client rejects non-HTTPS production configuration.

## Storage

The bridge volume contains the linked-device identity, message archive, raw
WhatsApp identifiers, downloaded media, and push endpoint. Use encrypted host
storage, restrict backups, and stop the container before copying SQLite files
unless using an online SQLite backup. Deleting the volume may require relinking.

The LP3 stores the public bridge URL and an Android-Keystore-encrypted API token
in app-private Preferences DataStore. Uninstalling or clearing app data removes
that configuration.

## Logs

Production mode must not log tokens, message bodies, phone numbers, or JIDs.
`DEBUG=true` is for isolated diagnosis only; inspect and remove logs afterward.
CI must never echo `.env` or release-signing secrets.

## Release signing

The stable Android release key lives only in protected GitHub Actions secrets.
It must be backed up offline by the maintainer. Losing it means future APKs
cannot update existing installs; users must uninstall and lose app-local
configuration before installing a differently signed build.
