#!/bin/sh
# proxmops installer — curl -fsSL https://raw.githubusercontent.com/prop4n/proxmops/main/packaging/install.sh | sh
# Downloads the latest release, installs the binary and a systemd service, and starts it.
# Uninstall with: ... | sh -s -- --uninstall
set -eu

REPO="prop4n/proxmops"
BIN_DIR="/usr/local/bin"
CONFIG_DIR="/etc/proxmops"
STATE_DIR="/var/lib/proxmops"
UNIT_PATH="/etc/systemd/system/proxmops.service"

info() { printf '\033[1;34m==>\033[0m %s\n' "$1"; }
err() { printf '\033[1;31merror:\033[0m %s\n' "$1" >&2; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }

# Privileged commands run through sudo unless we are already root.
SUDO=""
if [ "$(id -u)" -ne 0 ]; then
	have sudo || err "run as root or install sudo"
	SUDO="sudo"
fi

write_unit() {
	$SUDO tee "$UNIT_PATH" >/dev/null <<EOF
[Unit]
Description=proxmops — declarative GitOps for Proxmox VE
Documentation=https://github.com/$REPO
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=$BIN_DIR/proxmops daemon --config $CONFIG_DIR/config.yaml
Restart=on-failure
RestartSec=5

DynamicUser=yes
StateDirectory=proxmops
StateDirectoryMode=0700
WorkingDirectory=$STATE_DIR

NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
PrivateDevices=yes
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectControlGroups=yes
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
RestrictNamespaces=yes
LockPersonality=yes
MemoryDenyWriteExecute=yes
SystemCallFilter=@system-service
SystemCallErrorNumber=EPERM

[Install]
WantedBy=multi-user.target
EOF
}

write_config() {
	$SUDO mkdir -p "$CONFIG_DIR"
	[ -f "$CONFIG_DIR/config.yaml" ] && return 0
	$SUDO tee "$CONFIG_DIR/config.yaml" >/dev/null <<EOF
# The cluster and Git source are set from the web UI and stored encrypted; the
# encryption key is generated automatically next to the database (0600).
server:
  databasePath: $STATE_DIR/proxmops.db
  cookieSecure: false
EOF
}

uninstall() {
	info "Stopping and removing the proxmops service"
	if have systemctl; then
		$SUDO systemctl disable --now proxmops.service >/dev/null 2>&1 || true
		$SUDO rm -f "$UNIT_PATH"
		$SUDO systemctl daemon-reload >/dev/null 2>&1 || true
	fi
	$SUDO rm -f "$BIN_DIR/proxmops"
	info "Removed. State ($STATE_DIR) and config ($CONFIG_DIR) were kept."
	exit 0
}

[ "${1:-}" = "--uninstall" ] && uninstall

for cmd in curl tar; do have "$cmd" || err "missing required command: $cmd"; done

os=$(uname -s | tr '[:upper:]' '[:lower:]')
[ "$os" = linux ] || err "this installer supports Linux only (got $os)"
arch=$(uname -m)
case "$arch" in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	*) err "unsupported architecture: $arch" ;;
esac

version="${PROXMOPS_VERSION:-latest}"
if [ "$version" = latest ]; then
	version=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
		grep -m1 '"tag_name"' | cut -d'"' -f4)
	[ -n "$version" ] || err "could not resolve the latest version"
fi
num=${version#v}

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
tarball="proxmops_${num}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$version"

info "Downloading proxmops $version ($os/$arch)"
curl -fsSL "$base/$tarball" -o "$tmp/$tarball"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"

info "Verifying checksum"
if have sha256sum; then
	( cd "$tmp" && grep " $tarball\$" checksums.txt | sha256sum -c - >/dev/null ) ||
		err "checksum verification failed"
elif have shasum; then
	( cd "$tmp" && grep " $tarball\$" checksums.txt | shasum -a 256 -c - >/dev/null ) ||
		err "checksum verification failed"
else
	err "missing sha256sum or shasum for checksum verification"
fi

tar -xzf "$tmp/$tarball" -C "$tmp"
info "Installing binary to $BIN_DIR/proxmops"
$SUDO install -m 0755 "$tmp/proxmops" "$BIN_DIR/proxmops"

if have systemctl; then
	write_config
	write_unit
	$SUDO systemctl daemon-reload
	$SUDO systemctl enable --now proxmops.service
	info "proxmops is running. Open http://<this-host>:8080 to finish setup."
else
	info "No systemd found. Run it yourself: proxmops daemon"
fi
