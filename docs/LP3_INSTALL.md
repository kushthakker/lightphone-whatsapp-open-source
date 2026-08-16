# Light Phone III installation

## Requirements

- LightOS v568 or newer.
- Dashboard Developer Mode enabled for the device.
- A USB data cable and a wired USB keyboard/adapter for the current community
  Android-settings method.
- Android platform-tools (`adb`) on the computer.

The Android-access steps are unofficial and may change with LightOS updates.

## Enable USB debugging

1. Connect the keyboard to the LP3 and press **Windows+B** to open Chromium.
2. Hold **Alt+Tab**, tap Chromium's icon, and open **App info**.
3. Invoke **Alt+Tab** again, select **Settings → App info → Open**.
4. In Android Settings open **About phone** and tap **Build number** seven times.
5. Open **System → Developer options**, enable **USB debugging**, then connect
   the LP3 to the computer.
6. Unlock the phone, accept the RSA prompt, choose **Always allow**, and verify:

```bash
adb devices -l
```

The device must show `device`, not `unauthorized` or an empty list.

## Install

Download `lp3-whatsapp-bridge.apk` and
`lp3-whatsapp-bridge.apk.sha256` from the same
[GitHub Release](https://github.com/kushthakker/lightphone-whatsapp-open-source/releases).
Verify the download before installing:

```bash
# Linux
sha256sum -c lp3-whatsapp-bridge.apk.sha256

# macOS
shasum -a 256 -c lp3-whatsapp-bridge.apk.sha256
```

Run the command for your operating system, not both.

If Android build-tools are installed, also verify the signature:

```bash
apksigner verify --verbose --print-certs lp3-whatsapp-bridge.apk
```

The signer SHA-256 digest must be:

```text
ec76cab6275c57f04e5471930b20a025e9cab63de0551a5a5a0397e37e952465
```

```bash
adb install -r lp3-whatsapp-bridge.apk
```

If an older build with a different signing key is installed, Android rejects
the update. Back up what you need, uninstall that package, then install the
release. Uninstalling removes app-local bridge configuration but does not touch
the server archive.

The public package ID is `org.lp3bridge.whatsapp`. An older private or community
build may use another package ID, so Android can install both and LightOS may
show two WhatsApp tools. Inspect packages before removing either one:

```bash
adb shell pm list packages | grep -E 'whatsapp|lightwhatsapp|lp3bridge'
adb shell dumpsys package org.lp3bridge.whatsapp | grep -E 'versionCode|versionName'
```

Remove only the package you identified. For example,
`adb uninstall org.lp3bridge.whatsapp` removes this public tool and its local
configuration; it does not remove another package ID or the hosted bridge.

## First run

1. Finish WhatsApp linking on the bridge setup page.
2. Open **WhatsApp** on the LP3 and choose **Scan setup QR**.
3. Scan the app-configuration QR shown by the setup page.
4. Wait for both the HTTPS health and authenticated status checks to pass.
5. Confirm the conversation list loads. The stored token is never displayed
   again; Settings allows replacement or clearing.

Manual URL/token entry is available if scanning fails. It is slower and exposes
the token while typing, so use it only in private.

## Update or roll back

Download and verify the newer release, then use the same install-over command:

```bash
adb install -r lp3-whatsapp-bridge.apk
```

An update signed by the project release key preserves the app's encrypted
bridge configuration. Android does not normally allow installing an older
version over a newer one. Before a rollback, keep the current APK, confirm the
hosted bridge is backed up, and expect an uninstall/reinstall to require
scanning the app-configuration QR again. A signature-mismatch error means the
installed APK came from another signing key; verify package IDs before
uninstalling anything.

## ADB troubleshooting

- **No device:** unlock the LP3, use a USB data cable, reconnect it, and rerun
  `adb devices -l`.
- **`unauthorized`:** accept the RSA prompt on the LP3. If it never appears,
  revoke USB debugging authorizations in Android Developer options and
  reconnect.
- **`INSTALL_FAILED_UPDATE_INCOMPATIBLE`:** the same package ID is signed by a
  different key. Do not uninstall until you identify which build and local
  configuration would be removed.
- **Two WhatsApp tools:** inspect both package IDs and versions as shown above,
  then uninstall only the unwanted package.

## Physical verification

Use dedicated non-personal test accounts and keep screenshots/logs private.
Verify inbound and outbound text, image, and voice independently. Then force
stop/reopen, reboot the LP3, and send again. Replace the server API token to
trigger a 401 and confirm Settings can recover without reinstalling.
