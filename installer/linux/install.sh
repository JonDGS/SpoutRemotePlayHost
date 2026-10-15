#!/usr/bin/env bash
# Installs spout-host for the current user with a systemd user service.
# Usage: ./install.sh [path/to/spout-host]   |   ./install.sh --uninstall
# Also installs the vibeshine_drm virtual display kernel module (DKMS, built from
# Nonary/libvirtualdisplay, MIT/GPL-2.0) and a root helper that only the installing
# user may run via sudo. Needs Linux 6.16+ and KDE Plasma (kscreen-doctor); dkms and kernel headers are installed
# automatically (Fedora/Arch/Debian/Ubuntu/openSUSE). If headers for the running kernel aren't
# available (e.g. Arch after a system update) it builds for the newest installed kernel and asks you to reboot.
# Use --no-driver to skip that part.
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
  # Another app (e.g. ArtLight) may already ship vibeshine_drm. Reuse it instead of building ours:
  # our DKMS copy installs to /updates, which takes precedence and would replace theirs.
  local existing
  existing="$(foreign_driver "$kver")" && {
    say "Reusing the installed virtual display driver ($existing); not building our own"
    install_helper
    load_driver
    return
  }
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
  install_helper || return 1

  if [[ "$build_for" != "$kver" ]]; then
    warn "REBOOT into kernel $build_for to enable the virtual display (the host works now; the monitor will appear after the reboot)."
  else
    load_driver
  fi
}

# Prints the version of a vibeshine_drm for kernel $1 that this installer did not build, if any.
foreign_driver() {
  local kver="$1" file ver
  file="$(modinfo -k "$kver" -F filename vibeshine_drm 2>/dev/null)" || return 1
  [[ -n "$file" ]] || return 1
  # Ours is the DKMS build in /updates/dkms; anything else came from another package.
  [[ "$file" != */updates/dkms/* ]] || return 1
  ver="$(modinfo -k "$kver" -F version vibeshine_drm 2>/dev/null)"
  echo "vibeshine_drm ${ver:-unknown version}"
}

install_helper() {
  sudo install -D -m 0755 -o root -g root "$here/spout-vdisplay" "$helper"
  printf '%s ALL=(root) NOPASSWD: %s\n' "$(id -un)" "$helper" | sudo tee "$sudoers" >/dev/null
  sudo chmod 0440 "$sudoers"
  sudo visudo -cf "$sudoers" >/dev/null || { sudo rm -f "$sudoers"; warn "invalid sudoers entry removed"; return 1; }
}

load_driver() {
  if [[ -d /sys/module/vibeshine_drm ]]; then
    say "Virtual display driver already loaded"
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

if [[ "${1:-}" == "--uninstall" ]]; then
  systemctl --user disable --now spout-host.service 2>/dev/null || true
  rm -f "$unit_dir/spout-host.service" "$bin_dir/spout-host" "$icon_file" "$desktop_file"
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

no_driver=0
if [[ "${1:-}" == "--no-driver" ]]; then no_driver=1; shift; fi
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
[[ $no_driver == 1 ]] || install_driver || warn "Virtual display is not set up; the host still runs. Re-run ./install.sh after fixing the above."
systemctl --user daemon-reload
systemctl --user enable --now spout-host.service
echo "Installed. UI: http://127.0.0.1:47995"
