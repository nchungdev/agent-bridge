#!/bin/sh
# Agent Bridge installer (Linux, macOS).
#   curl -fsSL https://raw.githubusercontent.com/nchungdev/agent-bridge/main/install.sh | sh
# Environment: AGENT_BRIDGE_VERSION=v1.0.1 pins a release; AGENT_BRIDGE_BIN_DIR sets the
# install folder (default ~/.local/bin); AGENT_BRIDGE_NO_SERVICE=1 skips the service.
set -eu

REPO="nchungdev/agent-bridge"
BIN_DIR="${AGENT_BRIDGE_BIN_DIR:-$HOME/.local/bin}"

say() { printf '%s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }

have curl || die "curl is required"
have tar || die "tar is required"

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) die "unsupported OS: $(uname -s) (Linux and macOS only)" ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) die "unsupported CPU: $(uname -m)" ;;
esac

tag="${AGENT_BRIDGE_VERSION:-}"
if [ -z "$tag" ]; then
  tag=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
    sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n 1)
  [ -n "$tag" ] || die "could not find the latest release"
fi

asset="agent-bridge_${tag}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$tag"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

say "Downloading Agent Bridge $tag ($os/$arch)..."
curl -fsSL -o "$tmp/$asset" "$base/$asset" || die "no build for $os/$arch in $tag"
curl -fsSL -o "$tmp/$asset.sha256" "$base/$asset.sha256" || die "checksum file missing"

want=$(cut -d ' ' -f 1 "$tmp/$asset.sha256")
if have sha256sum; then
  got=$(sha256sum "$tmp/$asset" | cut -d ' ' -f 1)
else
  got=$(shasum -a 256 "$tmp/$asset" | cut -d ' ' -f 1)
fi
[ "$want" = "$got" ] || die "checksum mismatch, nothing installed"

mkdir -p "$tmp/x"
tar -xzf "$tmp/$asset" -C "$tmp/x"
# the archive holds agent-bridge_<tag>_<os>_<arch>/agent-bridge
bin=$(find "$tmp/x" -type f -name agent-bridge | head -n 1)
[ -n "$bin" ] || die "archive does not contain agent-bridge"

mkdir -p "$BIN_DIR"
chmod +x "$bin"
# Stage in the target folder, then rename: a running copy keeps working until it restarts.
cp "$bin" "$BIN_DIR/agent-bridge.new"
mv -f "$BIN_DIR/agent-bridge.new" "$BIN_DIR/agent-bridge"
say "Installed $BIN_DIR/agent-bridge"

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) say "Note: add $BIN_DIR to your PATH." ;;
esac

if [ "${AGENT_BRIDGE_NO_SERVICE:-}" = 1 ]; then
  say "Skipped the service. Run: $BIN_DIR/agent-bridge"
  exit 0
fi

"$BIN_DIR/agent-bridge" service install
say "Done. Open http://127.0.0.1:8088"
