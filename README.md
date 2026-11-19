# AddonBridge

A lightweight desktop manager for browser extensions, with automatic package downloads, updates and encrypted cross-device synchronization.

[简体中文](README.zh-CN.md)

### Features

- Manage Manifest V3 extensions distributed as GitHub Release ZIP packages.
- Use existing Chrome, Brave, Edge or Chromium profiles by default.
- Load extensions from a stable directory linked from the dashboard.
- Check releases automatically and keep extension identities stable across updates.
- Synchronize the extension catalog, update interval and selected extension settings through an encrypted Gist.
- Operate from a system tray and a local management dashboard.
- Detect the operating-system language, with English and Simplified Chinese overrides.

### Getting started

1. Download the archive for your operating system and architecture.
2. Open `AddonBridge.exe` on Windows, `AddonBridge.app` on macOS, or `./addonbridge` on Linux.
3. In **Browsers**, add a browser and select the existing profile you normally use.
4. In **Extensions**, enter a repository in `OWNER/REPOSITORY` format and an asset filename or glob pattern. The pattern must match exactly one compiled extension ZIP.
5. Select **Sync now** to prepare the package. In the extension card, copy the directory and select **Open extensions page**. Enable developer mode and choose **Load unpacked** with that directory. For an existing installation from the same directory, select **Reload** on its browser extension card.

The default release asset pattern is `*chrome*.zip`, which selects packages such as `name_version_chrome.zip`. Edit the repository or pattern from an extension card when a project uses another naming convention. Each card shows installed and latest versions. **Check updates** queries GitHub without installing anything. **Sync now** downloads and applies selected updates.

The dashboard opens in an installed browser. Closing the dashboard leaves the tray running. The tray menu provides immediate synchronization, pause/resume and exit. A separate managed-profile mode is available when explicitly selected.

Enable **Start AddonBridge with the system** from Sync settings or the tray. AddonBridge starts in the tray when you sign in.

Linux tray support requires a StatusNotifierItem-compatible desktop. GNOME may need an AppIndicator extension.

### Encrypted synchronization

In **Sync settings**, enter a GitHub token with Gist read/write permission and a shared encryption password of at least 12 characters. Create a secret Gist on the first computer. Connect other computers using the same Gist ID and password.

Credentials can be saved in the system credential store or kept in memory for the current session. Use **Copy sync password** when configuring another computer. Each device must be initialized separately to obtain its own device ID.

Each device writes an encrypted snapshot. AES-256-GCM protects the payload, with PBKDF2-SHA256 key derivation and random salts and nonces. Records merge using logical counters and deterministic device ordering. Deletion markers prevent stale devices from restoring removed records. Concurrent changes to one key resolve as a whole value.

Settings synchronization is opt-in per extension and covers `chrome.storage.local` and `chrome.storage.sync`. These areas may contain extension history or sign-in state. IndexedDB, localStorage and browser-level settings are outside this scope. Local state contains synchronized values in plaintext, while cloud snapshots are encrypted. Gist history retains previous encrypted revisions.

### Installation and operation

In existing profiles, the browser installs the unpacked extension. When the background bridge reconnects after an update, it requests a reload. If the bridge does not connect, use **Reload** on the browser’s extensions page.

Managed extension packages receive a background bridge and the `alarms`, `storage` and localhost permissions needed for synchronization. Original background workers are wrapped in their existing directory. The bridge checks pending changes once per minute. Release and Gist checks default to once per hour and can be configured in the dashboard.

Chromium dispatches `runtime.onInstalled` on installation and on unpacked extension reload. Extensions use that event to initialize features such as image context menus.

The manager listens only on `127.0.0.1`. Dashboard requests require a random access token and validated Host and Origin headers. Extension bridge tokens are scoped to their installation. Tokens and local browser paths are excluded from cloud snapshots.

Windows Chrome and Brave have been runtime-tested. macOS and Linux packages are cross-compiled, with runtime verification still pending. Edge runtime verification is also pending. Platform details and verification scope are recorded in `validation.json`.

### Command line

Replace uppercase placeholders with your repository and release asset pattern. On Windows, use `addonbridge-cli.exe` in place of `addonbridge`.

```sh
addonbridge init
addonbridge browser add chrome
addonbridge profile chrome existing Default
addonbridge add OWNER/REPOSITORY "ASSET_PATTERN"
addonbridge settings OWNER/REPOSITORY on
addonbridge gui
addonbridge sync
addonbridge status
```

Fresh installations store data in the operating system's user configuration directory under `AddonBridge`. Existing installations automatically reuse their data location and encrypted records. Set `ADDONBRIDGE_HOME` to choose a custom directory.

Console synchronization accepts `ADDONBRIDGE_GITHUB_TOKEN` and `ADDONBRIDGE_PASSWORD`. Existing-profile bridges require the tray application to be running. Changes made through the desktop are serialized with command-line operations.

### Build and test

Go 1.25 or newer is required. Python 3 packages the release archives.

```sh
go test ./...
go vet ./...
go build -trimpath -ldflags="-s -w" -o addonbridge .
python build_release.py
```

The build script produces desktop and console executables for Windows, macOS and Linux on amd64 and arm64, together with a source archive and SHA-256 checksums.

Set `ADDONBRIDGE_INTEGRATION=1` to run installed-browser tests in temporary profiles. Live release tests additionally require `ADDONBRIDGE_RELEASE_TEST=1`, `ADDONBRIDGE_TEST_REPOSITORY` and `ADDONBRIDGE_TEST_ASSET`. Cross-compilation alone does not establish runtime compatibility.
