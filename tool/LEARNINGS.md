# Learnings

## 2026-08-15

- Public Light SDK commit `522f94d5e862bd8824b43f3dfc76221105b720d5` exposes the permission-aware `com.thelightphone.sdk.LightQrCodeScanner` wrapper and `SealedLightContext.dataStore`; entry points still have no context.
- Runtime bridge setup must keep URL and token out of Gradle/BuildConfig. The SDK Preferences DataStore supports atomic `edit` writes; store only the normalized URL and AES/GCM ciphertext.
