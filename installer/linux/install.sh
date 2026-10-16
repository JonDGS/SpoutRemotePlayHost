#!/usr/bin/env bash
# Installs spout-host for the current user with a systemd user service.
# Usage: ./install.sh [path/to/spout-host]   |   ./install.sh --uninstall
# Also installs the vibeshine_drm virtual display kernel module (DKMS, built from
# Nonary/libvirtualdisplay, MIT/GPL-2.0) and a root helper that only the installing
# user may run via sudo. Needs Linux 6.16+ and KDE Plasma (kscreen-doctor); dkms and kernel headers are installed
# automatically (Fedora/Arch/Debian/Ubuntu/openSUSE). If headers for the running kernel aren't
# available (e.g. Arch after a system update) it builds for the newest installed kernel and asks you to reboot.
# Use --no-driver to skip that part.
# On KDE Plasma it also makes Steam start with -pipewire (Steam's Wayland desktop capture) through
# a user copy of steam.desktop. Use --no-steam-pipewire to skip that.
set -euo pipefail

bin_dir="${HOME}/.local/bin"
unit_dir="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
data_dir="${XDG_DATA_HOME:-$HOME/.local/share}"
icon_file="$data_dir/icons/hicolor/256x256/apps/spout-remote-play.png"
desktop_file="$data_dir/applications/spout-remote-play.desktop"
here="$(cd "$(dirname "$0")" && pwd)"

helper=/usr/local/libexec/spout-vdisplay
sudoers=/etc/sudoers.d/spout-remote-play
drm_src=3083d210b55fba4b886e5c634e6abee0942201d4 # Nonary/libvirtualdisplay
drm_ver=1.0.0
drm_dir=/usr/src/vibeshine-drm-$drm_ver

say() { printf '==> %s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }

# Fedora's repos only carry headers for the newest kernel; older exact versions live in Koji.
fedora_koji_headers() {
  local kver="$1" nvr arch ver rel
  arch="${kver##*.}"; nvr="${kver%.*}"; ver="${nvr%%-*}"; rel="${nvr#*-}"
  sudo dnf install -y "https://kojipkgs.fedoraproject.org/packages/kernel/$ver/$rel/$arch/kernel-devel-$ver-$rel.$arch.rpm"
}

# Installs dkms and kernel headers using the distro's package manager.
install_build_deps() {
  local kver="$1" id like pkgbase
  id="$(. /etc/os-release 2>/dev/null; echo "${ID:-}")"; like="$(. /etc/os-release 2>/dev/null; echo "${ID_LIKE:-}")"
  case " $id $like " in
    *" fedora "*|*" rhel "*)
      sudo dnf install -y dkms
      if ! has_headers "$kver"; then
        sudo dnf install -y "kernel-devel-$kver" 2>/dev/null || fedora_koji_headers "$kver" || true
      fi ;;
    *" arch "*)
      pkgbase="$(pacman -Qqo "/usr/lib/modules/$kver/vmlinuz" 2>/dev/null || true)"
      sudo pacman -S --needed --noconfirm dkms "${pkgbase:-linux}-headers" ;;
    *" debian "*|*" ubuntu "*)
      sudo apt-get install -y dkms "linux-headers-$kver" ;;
    *" suse "*)
      sudo zypper --non-interactive install dkms kernel-default-devel ;;
    *) warn "unknown distribution; install dkms and the kernel headers for $kver yourself" ; return 1 ;;
  esac
}

has_headers() { [[ -e "/lib/modules/$1/build" ]]; }

# Newest installed kernel that has headers (used when the running one has none).
newest_kernel_with_headers() {
  local d v
  for d in $(ls -1d /lib/modules/*/ 2>/dev/null | sort -V -r); do
    v="$(basename "$d")"
    has_headers "$v" && { echo "$v"; return 0; }
  done
  return 1
}

install_driver() {
  local kver build_for
  kver="$(uname -r)"
  sudo -v || { warn "sudo is required to install the driver"; return 1; }
  if [[ "$(printf '%s\n6.16\n' "${kver%%[-+]*}" | sort -V | head -1)" != 6.16 ]]; then
    warn "the virtual display driver needs Linux 6.16 or newer (running $kver); skipping it"
    return 1
  fi
  if ! command -v kscreen-doctor >/dev/null; then
    warn "kscreen-doctor not found: the virtual display needs KDE Plasma. Continuing."
  fi
  say "Installing the virtual display driver (needs sudo; builds a kernel module with DKMS)"
  if ! command -v dkms >/dev/null || ! has_headers "$kver"; then
    say "Installing build dependencies (dkms, kernel headers)"
    install_build_deps "$kver" || true
  fi
  command -v dkms >/dev/null || { warn "dkms is not installed; skipping the virtual display driver"; return 1; }
  build_for="$kver"
  if ! has_headers "$kver"; then
    # The repos may only carry headers for a newer kernel than the one running.
    build_for="$(newest_kernel_with_headers)" || { warn "no kernel headers found for $kver; skipping the driver"; return 1; }
    warn "headers for the running kernel ($kver) are unavailable; building for $build_for instead"
  fi

  local tmp; tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' RETURN
  curl -fsSL "https://github.com/Nonary/libvirtualdisplay/archive/${drm_src}.tar.gz" | tar xz -C "$tmp" --strip-components=1
  sudo rm -rf "$drm_dir"
  sudo cp -r "$tmp/linux/vibeshine-drm" "$drm_dir"
  sudo sed "s/@PROJECT_VERSION_NUMERIC@/$drm_ver/" "$drm_dir/dkms.conf.in" | sudo tee "$drm_dir/dkms.conf" >/dev/null
  # Upstream generates this header at packaging time; the raw source does not ship it.
  printf '#define VIBESHINE_DRM_VERSION "%s"\n' "$drm_ver" | sudo tee "$drm_dir/vibeshine_drm_version.h" >/dev/null
  sudo dkms remove "vibeshine-drm/$drm_ver" --all >/dev/null 2>&1 || true
  sudo dkms install "vibeshine-drm/$drm_ver" -k "$build_for" --force || { warn "driver build failed"; return 1; }
  sudo install -D -m 0755 -o root -g root "$here/spout-vdisplay" "$helper"
  printf '%s ALL=(root) NOPASSWD: %s\n' "$(id -un)" "$helper" | sudo tee "$sudoers" >/dev/null
  sudo chmod 0440 "$sudoers"
  sudo visudo -cf "$sudoers" >/dev/null || { sudo rm -f "$sudoers"; warn "invalid sudoers entry removed"; return 1; }

  if [[ "$build_for" != "$kver" ]]; then
    warn "REBOOT into kernel $build_for to enable the virtual display (the host works now; the monitor will appear after the reboot)."
  elif ! sudo modprobe vibeshine_drm create_default_dev=0; then
    if command -v mokutil >/dev/null && mokutil --sb-state 2>/dev/null | grep -qi enabled; then
      warn "Secure Boot rejected the driver's signing key. One-time setup: choose a password now, reboot, then pick Enroll MOK > Continue > Yes and enter it."
      sudo mokutil --import /var/lib/dkms/mok.pub || warn "key enrollment failed; run: sudo mokutil --import /var/lib/dkms/mok.pub"
    else
      warn "driver built but could not be loaded (modprobe failed); check dmesg."
    fi
    return 1
  else
    say "Virtual display driver loaded"
  fi
}

# Steam only captures the Wayland desktop when started with -pipewire; without it Remote Play
# streams a black screen. A user copy of steam.desktop (and of Steam's autostart entry, if any)
# adds the flag, so Steam launched from the menu or at login gets it. Marked so uninstall only
# removes our copies. A copy the user made themselves is left alone.
steam_marker="X-SpoutRemotePlay-Pipewire=true"

steam_desktop_src() {
  local d
  for d in /usr/local/share /usr/share; do
    [[ -f "$d/applications/steam.desktop" ]] && { echo "$d/applications/steam.desktop"; return 0; }
  done
  return 1
}

# Writes $2 from $1 with -pipewire added to every Exec line that runs steam.
write_steam_pipewire() {
  local src="$1" dst="$2"
  # Steam's own autostart entry is edited in place ($src == $dst); any other existing copy is the user's.
  if [[ "$src" != "$dst" && -f "$dst" ]] && ! grep -qx "$steam_marker" "$dst"; then
    grep -q -- '-pipewire' "$dst" || warn "$dst is your own copy; add -pipewire to its Exec lines yourself"
    return 0
  fi
  mkdir -p "$(dirname "$dst")"
  awk -v marker="$steam_marker" '
    /^Exec=/ && $0 !~ /-pipewire/ { sub(/^Exec=[^ ]*steam/, "& -pipewire") }
    { print }
    /^\[Desktop Entry\]$/ && !done { print marker; done = 1 }
  ' "$src" > "$dst.tmp" && mv "$dst.tmp" "$dst"
}

setup_steam_pipewire() {
  local src autostart="${XDG_CONFIG_HOME:-$HOME/.config}/autostart/steam.desktop"
  src="$(steam_desktop_src)" || { say "Steam's desktop entry not found (Flatpak Steam needs -pipewire added by hand)"; return 0; }
  write_steam_pipewire "$src" "$data_dir/applications/steam.desktop"
  [[ -f "$autostart" ]] && write_steam_pipewire "$autostart" "$autostart"
  command -v update-desktop-database >/dev/null && update-desktop-database -q "$data_dir/applications" 2>/dev/null || true
  say "Steam will start with -pipewire from now on (restart Steam if it is running)"
}

remove_steam_pipewire() {
  local f
  for f in "$data_dir/applications/steam.desktop" "${XDG_CONFIG_HOME:-$HOME/.config}/autostart/steam.desktop"; do
    [[ -f "$f" ]] && grep -qx "$steam_marker" "$f" || continue
    if [[ "$f" == */autostart/* ]]; then
      # Steam's own autostart entry: put it back as it was, minus our flag and marker.
      sed -i -e "/^$steam_marker\$/d" -e 's/ -pipewire//' "$f"
    else
      rm -f "$f"
    fi
  done
}

if [[ "${1:-}" == "--uninstall" ]]; then
  systemctl --user disable --now spout-host.service 2>/dev/null || true
  rm -f "$unit_dir/spout-host.service" "$bin_dir/spout-host" "$icon_file" "$desktop_file"
  remove_steam_pipewire
  if [[ -e "$helper" ]]; then
    sudo "$helper" destroy || true
    sudo rm -f "$helper" "$sudoers"
    sudo dkms remove "vibeshine-drm/$drm_ver" --all 2>/dev/null || true
    sudo rm -rf "$drm_dir"
  fi
  systemctl --user daemon-reload
  echo "Removed."
  exit 0
fi

no_driver=0 steam_pipewire=1
while [[ "${1:-}" == --no-* ]]; do
  case "$1" in
    --no-driver) no_driver=1 ;;
    --no-steam-pipewire) steam_pipewire=0 ;;
    *) echo "unknown option $1" >&2; exit 1 ;;
  esac
  shift
done
src="${1:-$here/spout-host}"
[[ -x "$src" ]] || { echo "spout-host binary not found at $src" >&2; exit 1; }

mkdir -p "$bin_dir" "$unit_dir"
install -m 0755 "$src" "$bin_dir/spout-host"
install -m 0644 "$here/spout-host.service" "$unit_dir/spout-host.service"
if [[ -f "$here/spout-remote-play.png" ]]; then
  install -D -m 0644 "$here/spout-remote-play.png" "$icon_file"
  install -D -m 0644 "$here/spout-remote-play.desktop" "$desktop_file"
  command -v gtk-update-icon-cache >/dev/null && gtk-update-icon-cache -q -t "$data_dir/icons/hicolor" 2>/dev/null || true
  command -v update-desktop-database >/dev/null && update-desktop-database -q "$data_dir/applications" 2>/dev/null || true
fi
[[ $steam_pipewire == 0 ]] || ! command -v kscreen-doctor >/dev/null || setup_steam_pipewire
[[ $no_driver == 1 ]] || install_driver || warn "Virtual display is not set up; the host still runs. Re-run ./install.sh after fixing the above."
systemctl --user daemon-reload
systemctl --user enable --now spout-host.service
echo "Installed. UI: http://127.0.0.1:47995"
