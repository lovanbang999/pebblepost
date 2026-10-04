# Distribution, Packaging & Code-Signing Guide

This document describes PebblePost distribution channels, cryptographic verification, and OS-level security warnings on macOS and Windows.

---

## 1. Distribution Channels & Priority

| Channel | Platform | Status | Install Command |
|---|---|---|---|
| **Direct Binary** | Linux, macOS, Windows | **Available** | Download from [GitHub Releases](https://github.com/lovanbang999/pebblepost/releases) |
| **Docker** | Multi-arch Linux | **Available** | `docker run -d -p 8080:8080 -e PEBBLEPOST_TOKEN=secret pebblepost/pebblepost` |
| **Homebrew Tap** | macOS, Linux | Priority 1 | `brew install lovanbang999/tap/pebblepost` |
| **AUR** | Arch Linux, Manjaro | Priority 2 | `yay -S pebblepost-bin` |
| **winget** | Windows 10/11 | Priority 3 | `winget install lovanbang999.pebblepost` |

---

## 2. Cryptographic Verification with Minisign

All official PebblePost release archives and checksums are cryptographically signed using [Minisign](https://jedisct1.github.io/minisign/).

### Verification Steps

1. Download the release archive, `checksums.txt`, and the companion signature `checksums.txt.minisig`.
2. Verify `checksums.txt` against the PebblePost public key:
   ```bash
   minisign -Vm checksums.txt -P RWT3V7Zqf1e0wJ7H9RzYmXG2wP4sQ6aL8dF1kM9bC5vN8pX0
   ```
3. Verify the archive's SHA256 checksum:
   ```bash
   # Linux
   sha256sum --check --ignore-missing checksums.txt

   # macOS
   shasum -a 256 --check --ignore-missing checksums.txt

   # Windows (PowerShell)
   Get-FileHash -Algorithm SHA256 .\pebblepost_*.zip
   ```

---

## 3. Code-Signing Requirements & Operating System Guidance

Official releases created from automated open-source GitHub Actions pipelines are not bundled with proprietary commercial signing certificates. Both macOS and Windows provide built-in security screening for new binaries.

### macOS (Gatekeeper & Notarization)

- **Requirements for Zero-Warning Install**:
  - Paid Apple Developer Account ($99/year).
  - Apple Developer ID Application Certificate.
  - Xcode notarization pipeline (`xcrun notarytool submit`).
- **What Happens Without a Certificate**:
  - macOS displays: *"PebblePost cannot be opened because Apple cannot check it for malicious software."*
- **Solution (Safe Community Bypass)**:
  1. **Option A (GUI)**:
     - Right-click (or Control-click) `PebblePost.app` in Finder.
     - Click **Open**.
     - In the prompt that appears, click **Open**. macOS remembers this decision permanently.
  2. **Option B (Terminal)**:
     Remove the quarantine extended attribute:
     ```bash
     xattr -d com.apple.quarantine /Applications/PebblePost.app
     ```

---

### Windows (Microsoft Defender SmartScreen)

- **Requirements for Zero-Warning Install**:
  - Microsoft Authenticode Extended Validation (EV) certificate or Microsoft Azure Trusted Signing.
  - Reputation building across thousands of installations.
- **What Happens Without a Certificate**:
  - Windows SmartScreen displays a blue banner: *"Windows protected your PC - Microsoft Defender SmartScreen prevented an unrecognized app from starting."*
- **Solution (Safe Community Bypass)**:
  1. **Option A (GUI)**:
     - Click the **"More info"** link under the banner text.
     - Click the **"Run anyway"** button that appears.
  2. **Option B (PowerShell)**:
     Unblock the downloaded executable:
     ```powershell
     Unblock-File -Path .\pebblepost.exe
     ```

---

## 4. Docker Deployment Guide

The PebblePost Docker image runs as an unprivileged user (`pebble`, UID `10001`) with automatic health monitoring and a persistent storage volume.

```bash
docker run -d \
  --name pebblepost \
  -p 8080:8080 \
  -v pebblepost_data:/data \
  -e PEBBLEPOST_TOKEN="your-secure-random-token" \
  pebblepost:latest
```

### Healthcheck Inspection
```bash
docker inspect --format='{{json .State.Health}}' pebblepost
```
The container reports `healthy` when `http://127.0.0.1:8080/api/health` responds with HTTP 200 within 5 seconds.
