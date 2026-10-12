<p align="center">
  <img src="installer/linux/spout-remote-play.png" alt="Spout Remote Play" width="128">
</p>

<h1 align="center">SpoutRemotePlay Host</h1>

<p align="center">
  <b>Pick up your handheld, press play on a game from your PC, and go.</b><br>
  Works with Steam's built-in Remote Play: wakes your PC and keeps the stream on a virtual monitor, with your other displays off.
</p>

<p align="center">
  <a href="https://github.com/justjoseorg/SpoutRemotePlayHost/releases/latest"><img alt="Latest release" src="https://img.shields.io/github/v/release/justjoseorg/SpoutRemotePlayHost?label=latest&color=2ea44f"></a>
  <a href="https://github.com/justjoseorg/SpoutRemotePlayHost/releases"><img alt="Downloads" src="https://img.shields.io/github/downloads/justjoseorg/SpoutRemotePlayHost/total"></a>
  <img alt="Platforms" src="https://img.shields.io/badge/platform-Windows%20%7C%20Linux-blue">
  <img alt="Architecture" src="https://img.shields.io/badge/arch-x86__64-lightgrey">
  <a href="https://ko-fi.com/justjose"><img alt="Ko-fi" src="https://img.shields.io/badge/Ko--fi-support-FF5E5B?logo=ko-fi&logoColor=white"></a>
</p>

<p align="center">
  <a href="https://github.com/justjoseorg/SpoutRemotePlayHost/releases/latest"><b>⬇️ Download the latest release</b></a>
  &nbsp;·&nbsp;
  <a href="https://github.com/justjoseorg/SpoutRemotePlay">🎮 Decky plugin</a>
  &nbsp;·&nbsp;
  <a href="CHANGELOG.md">📝 Changelog</a>
</p>

---

## Contents

- [✨ What is this?](#-what-is-this)
- [📥 Install, step by step](#-install-step-by-step)
- [🔐 Pairing your handheld](#-pairing-your-handheld)
- [⚙️ Settings and features](#️-settings-and-features)
- [🔎 How it works](#-how-it-works)
- [🔧 Build it yourself](#-build-it-yourself)
- [🙏 Credits](#-credits)

## ✨ What is this?

This is the PC half of SpoutRemotePlay. It does **not** stream anything itself. Steam still does the streaming (capture, encode, input and Steam Link on the handheld). The host watches for a Remote Play session and makes sure the stream lands on a virtual monitor, with your other displays off. When the session ends, it puts everything back.

- **Windows:** Steam makes the virtual display itself (using the SudoVDA driver) and sizes it for your device. The host waits for it, turns your other displays off, and restores them afterwards.
- **Linux:** Steam doesn't make a virtual display, so the host makes one that matches your device and removes it afterwards.

| | |
|---|---|
| 🖥️ **Virtual monitor per device** | On Linux, resolution and refresh rate come from that device's settings. On Windows, Steam sizes its own display. |
| ⭐ **Primary while streaming** | The virtual monitor is your main display during the session, so Steam streams it. |
| 🌑 **Other displays off** | Every other display turns off during the session and comes back when it ends. On Windows you can choose displays to keep on. |
| 🔐 **Paired devices only** | Unpaired clients never get a monitor. |
| 🧩 **Apps** | Add programs to Steam from the web page, with optional artwork. |
| ⏻ **Shut down** | A paired device can shut the PC down, for example from the Decky plugin. |
| ⏰ **Wake relay** | A paired device away from home can ask this PC to wake another PC on its network. |

Together with the [Decky plugin](https://github.com/justjoseorg/SpoutRemotePlay), the flow is: **wake the PC → press play in Steam → stream.**

> **Why "Spout"?** It's a play on **Valve**. A valve controls the flow of steam, and a spout is where the steam comes out. Steam does the streaming. Spout gives it somewhere to pour: a virtual monitor shaped for your handheld.

## 📥 Install, step by step

Install the host on your PC, then [install the Decky plugin](https://github.com/justjoseorg/SpoutRemotePlay#-install-step-by-step) on your handheld.

> **Decky Plugin Store:** the plugin is submitted to the store in [decky-plugin-database PR #1146](https://github.com/SteamDeckHomebrew/decky-plugin-database/pull/1146), which is still open. **Once it is merged, the plugin will be available straight from the Decky store.** Until then, install it from a ZIP file as described in the plugin README.

Files are on the **[latest release](https://github.com/justjoseorg/SpoutRemotePlayHost/releases/latest)**:

| System | File | Notes |
|---|---|---|
| 🪟 Windows | `SpoutRemotePlayHost-Setup-vX.Y.Z.exe` | Installer (recommended) |
| 🪟 Windows | `spout-host-vX.Y.Z-windows-amd64.exe` | Portable, no driver or autostart |
| 🐧 Linux | `spout-host-vX.Y.Z-linux-amd64.tar.gz` | Run `./install.sh` |

### 🪟 Windows

1. Download `SpoutRemotePlayHost-Setup-vX.Y.Z.exe` (not the bare `spout-host-*.exe`) and run it.
2. Pick your options (explained below) and finish the wizard. It installs the host, a tray icon and a Start Menu shortcut.
3. The host's web page opens. Keep it handy for [pairing](#-pairing-your-handheld).
4. Do the [Steam setup](#-steam-setup-windows) below.

Installer options:

- **SudoVDA virtual display driver.** If SudoVDA is already installed (for example by [ArtLight](https://github.com/onaiaku/ArtLight) or Apollo), it is reused and left alone, so both apps work side by side. Uninstalling SpoutRemotePlayHost never removes it. Otherwise the installer adds a SudoVDA built from [SudoMaker/SudoVDA](https://github.com/SudoMaker/SudoVDA) (MIT) in CI and signed with a self-signed certificate. That certificate goes into the Windows Trusted Root and Trusted Publishers stores, and both are removed on uninstall unless another app replaced the driver since. Untick this to install the host only.
- **Start automatically when I sign in.** A logon task runs the host, also on battery and with no time limit.
- **Allow Decky to scan this PC on the local network.** The host listens on `0.0.0.0:47995`, and a firewall rule allows it on all network profiles (home networks are often marked Public). Other machines still need a token.
- **Spout Sign-In** (off by default). Lets a paired device type your PIN at the Windows sign-in screen. See [Sign in after Wake-on-LAN](#-sign-in-after-wake-on-lan-windows).

Good to know:

- Starting the host while it already runs just opens its web page.
- The exe has no console. Errors show in a message box and go to `%APPDATA%\SpoutRemotePlayHost\spout-host.log`.
- **Ctrl+Alt+Shift+Q** (Moonlight's quit-stream shortcut) ends the session's display handling and brings your displays back. If another app already uses it, the hotkey is turned off (and logged) and everything else runs normally.

#### 🎮 Steam setup (Windows)

Steam makes its own virtual display for Remote Play using the SudoVDA driver. The menu names follow public descriptions of the Steam client beta, so Steam's wording may differ.

1. Install the host with **SudoVDA virtual display driver** ticked (or keep an existing SudoVDA).
2. In Steam, join the client beta: **Steam > Settings > Interface > Client Beta Participation**. Then restart Steam.
3. In **Steam > Settings > Remote Play > Advanced Host Options**, pick SudoVDA as the virtual display device if Steam offers it.
4. Stream from your handheld. The host waits up to 20 seconds for Steam's display, turns the other displays off, and restores them when the session ends. If Steam's display never shows up, your displays are left on.

Tested on Windows 11 with an Odin 2 Portal on the Steam client beta: the other displays turned off during the stream and came back afterwards.

#### 🔑 Sign in after Wake-on-LAN (Windows)

Steam and the host only run once someone is signed in. So a PC that was just woken sits at the sign-in screen and can't stream. With **Spout Sign-In** ticked in Setup, the Decky plugin shows a numpad when the PC is on but nobody is signed in (or it's locked). Type your Windows PIN and the PC signs in. If Steam isn't running 5 seconds later, the host starts it minimized. Run the host with `-start-steam=false` to turn that off.

How it stays safe:

- Setup installs the `SpoutSignIn` system service. It starts at boot and listens on `0.0.0.0:47994`, with a firewall rule.
- Setup also turns on the Windows **"Do not display the lock screen"** policy, so the PIN box shows right away. Removing Sign-In (or the host) turns it back off, unless it was already on.
- It only accepts devices paired with this host, and allows 5 tries per 5 minutes on top of Windows' own PIN lockout.
- It only types when nobody is signed in or the session is locked, and only while the sign-in screen has the keyboard. It never types into a signed-in desktop or a UAC prompt.
- It trusts the paired devices in `%APPDATA%\SpoutRemotePlayHost\paired.json`. Programs running as you can edit that file. At worst, one could type guesses at your lock screen, and Windows' PIN lockout limits that.
- The PIN is typed once and never stored or logged. Only digits work, so you need a numeric Windows Hello PIN, not a password.
- The service logs to `%ProgramData%\SpoutRemotePlayHost\spout-host.log`.

### 🐧 Linux

1. Download `spout-host-vX.Y.Z-linux-amd64.tar.gz` and extract it.
2. Open a terminal in the extracted folder and run `./install.sh`. It sets up a user systemd service. Run `./install.sh --uninstall` to remove it.
3. Enter your sudo password when asked. The script also sets up the driver with DKMS:
   - It installs `dkms` and kernel headers for you (Fedora, Arch, Debian, Ubuntu, openSUSE).
   - On Fedora it gets headers for the exact running kernel from Koji, so no reboot is needed. Only as a last resort does it build for the newest installed kernel and ask for a reboot.
   - It adds a sudoers rule limited to `/usr/local/libexec/spout-vdisplay`. Pass `--no-driver` to skip the driver.
4. The service listens on `0.0.0.0:47995` so the Decky plugin can reach it. Other machines still need a token.

Requirements: **KDE Plasma on Wayland** and **Linux 6.16 or newer**. The Linux backend uses the `vibeshine_drm` kernel module (the same driver ArtLight uses) and `kscreen-doctor`.

Tested by streaming from a Linux PC to an AYN Odin 2 Portal: the host made a virtual display for the stream and removed it afterwards.

## 🔐 Pairing your handheld

1. In the Decky plugin, pick this PC.
2. The handheld shows a 4-digit PIN, and the host shows a notification (Windows and Linux).
3. Type the PIN into the host's web page.

Requests expire after 2 minutes and are cancelled after 5 wrong PINs. Confirming and revoking can only be done on the host PC.

## ⚙️ Settings and features

### 📺 Monitor config

The **Monitor config** tab of the web page has:

- **Defaults:** the virtual monitor for devices without their own settings (Linux only, since Steam sizes its own display on Windows). Each device can override this from the Devices tab or from the plugin.
- **Displays during a session** (Windows): tick the displays to keep on while streaming. Unticked ones turn off and come back afterwards. If Windows turns one back on mid-session (for example a monitor that wakes from sleep), the host turns it off again.

There is no codec setting. Steam picks the codec itself, and PyroWave isn't available with Steam streaming.

### 📱 Android and other Steam Link devices

You don't need an Android app, because Steam Link already streams. The Devices tab has an **Other Steam devices** section listing devices Steam knows about (read from the running Steam client). Set a virtual monitor for each one (resolution, refresh, auto-create). When a stream starts, the host matches the client name in Steam's log to that device.

- Pair the device in Steam first.
- The list also shows other PCs Steam can see. Ignore those.
- The device list is verified against real Steam on Linux. An actual Android stream hasn't been tried yet.

### ⏰ Wake from away

Wake-on-LAN packets stay on your local network, so a handheld on a VPN such as WireGuard can't wake a PC by itself. The host can relay it. A paired device calls `POST /api/wake` with `{"mac": "aa:bb:cc:dd:ee:ff"}` and its token, and the host sends the magic packet on every network it's on. The Decky plugin does this automatically: when you wake a PC, it also asks every other online paired PC to send the packet. Something at home has to be on, such as a second PC running the host.

### ⏻ Shut down

A paired device can send `POST /api/power` with `{"action": "shutdown"}`. Windows does a full shutdown after 5 seconds (so Wake-on-LAN still works), though apps with unsaved work can block it. Linux runs `systemctl poweroff`.

### 🧩 Apps

The **Apps** tab adds programs to this PC's Steam library as non-Steam shortcuts, so you can stream them like any game. The Decky plugin can list them.

- **Steam integration:** Steam is controlled through its local debug port. Click **Enable Steam integration** once, then restart Steam.
- **Local only:** adding, removing and deleting apps only works from this PC, because it runs programs on the host. Paired devices can only read the list.
- **Artwork (optional):** save a free [SteamGridDB](https://www.steamgriddb.com/profile/preferences/api) API key and new shortcuts get a cover, hero, logo and icon. Setting artwork in Steam is verified on Linux. The SteamGridDB lookup is only unit tested against a fake server.
- Add and remove are verified on Linux Steam. Whether the Steam Link client lists them isn't verified yet.

## 🔎 How it works

Two independent signals tell the host a stream started, so it doesn't rely on one fragile hook:

- **From the handheld:** the plugin reports Steam's Remote Play start and stop to the PC you're streaming from (`POST /api/session`, using the device's own token).
- **From the PC:** the host reads Steam's `streaming_log.txt`. A start looks like "Streaming started to <device>…". The end is "PipeWire: Deinitializing streaming" on Linux and "Encoding complete" on Windows.

Only **paired** devices get a virtual monitor. A session from an unpaired client (or a plugin call using the API token instead of a device token) is logged and ignored. The monitor uses that device's settings, survives a stream restart or reconnect (90 s grace on Windows, 5 s on Linux), and is only removed by the device that owns it.

## 🔧 Build it yourself

```bash
go build ./cmd/spout-host
GOOS=windows GOARCH=amd64 go build ./cmd/spout-host
```

Releases are built by CI when a pull request is merged into `main`.

## 🙏 Credits

This project builds on the ideas and work of others, and I'm grateful to them:

- [Moonlight](https://moonlight-stream.org/) – the open-source game streaming client that started it all.
- [StreamLight](https://github.com/FoggyBytes/StreamLight) – a Moonlight fork with deeper host integration.
- [ArtMoon](https://github.com/onaiaku/ArtMoon) and [ArtLight](https://github.com/onaiaku/ArtLight) – the gamepad-first client and its one-installer host. The virtual-monitor design, the pairing flow and the Linux driver choice follow ArtLight.
- [SudoVDA](https://github.com/SudoMaker/SudoVDA) (MIT) – the Windows virtual display driver.
- [libvirtualdisplay](https://github.com/Nonary/libvirtualdisplay) (MIT, GPL-2.0 module) – the Linux `vibeshine_drm` virtual display module, derived from the Linux kernel's VKMS.

SpoutRemotePlayHost is an independent project and is not affiliated with any of them or with Valve.

---

<p align="center">
  If this is useful to you, you can <a href="https://ko-fi.com/justjose">☕ buy me a coffee on Ko-fi</a>.
</p>
