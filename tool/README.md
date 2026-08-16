# LP3 WhatsApp bridge tool

The app is configured at runtime. It does not contain a bridge URL or API token at build time.

## Setup QR format

The bridge server must emit exactly one JSON object for the QR payload:

```json
{
  "type": "org.lp3bridge.whatsapp.config",
  "version": 1,
  "baseUrl": "https://bridge.example",
  "apiToken": "a-token-of-at-least-32-characters"
}
```

Only `version: 1` and the exact `type` are accepted. The URL must be an HTTPS origin and cannot include credentials, a path, a query, or a fragment. The tool performs an unauthenticated `GET /healthz` followed by an authenticated `GET /api/v1/status` before atomically saving the URL and Android-Keystore AES/GCM ciphertext in the SDK Preferences DataStore.

Settings display the bridge URL and a `Saved` token marker only; they never decrypt the token simply to render it. Clearing or replacing setup is available from Home.
