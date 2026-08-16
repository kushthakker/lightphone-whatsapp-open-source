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

```bash
adb install -r lp3-whatsapp-bridge.apk
```

If an older build with a different signing key is installed, Android rejects
the update. Back up what you need, uninstall that package, then install the
release. Uninstalling removes app-local bridge configuration but does not touch
the server archive.

## First run

1. Finish WhatsApp linking on the bridge setup page.
2. Open **WhatsApp** on the LP3 and choose **Scan setup QR**.
3. Scan the app-configuration QR shown by the setup page.
4. Wait for both the HTTPS health and authenticated status checks to pass.
5. Confirm the conversation list loads. The stored token is never displayed
   again; Settings allows replacement or clearing.

Manual URL/token entry is available if scanning fails. It is slower and exposes
the token while typing, so use it only in private.

## Physical verification

Use dedicated non-personal test accounts and keep screenshots/logs private.
Verify inbound and outbound text, image, and voice independently. Then force
stop/reopen, reboot the LP3, and send again. Replace the server API token to
trigger a 401 and confirm Settings can recover without reinstalling.
