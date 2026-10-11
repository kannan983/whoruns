#!/bin/sh
# Install whoruns on Linux or macOS:
#   curl -fsSL https://raw.githubusercontent.com/kannan983/whoruns/main/install.sh | sh
# Options (environment): WHORUNS_VERSION=v0.1.0  WHORUNS_INSTALL_DIR=~/bin
set -eu

repo=kannan983/whoruns

die() { echo "whoruns install: $*" >&2; exit 1; }

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  linux | darwin) ;;
  *) die "unsupported OS '$os' (on Windows use install.ps1)" ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) die "unsupported CPU '$(uname -m)'" ;;
esac

version=${WHORUNS_VERSION:-latest}
if [ "$version" = latest ]; then
  base="https://github.com/$repo/releases/latest/download"
else
  base="https://github.com/$repo/releases/download/$version"
fi
file="whoruns_${os}_${arch}.tar.gz"

download() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$1" -o "$2"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$2" "$1"
  else
    die "need curl or wget"
  fi
}

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading $file ($version)..."
download "$base/$file" "$tmp/$file" || die "download failed: $base/$file"
download "$base/checksums.txt" "$tmp/checksums.txt" || die "download failed: checksums.txt"

expected=$(grep "  $file\$" "$tmp/checksums.txt" | cut -d' ' -f1)
if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$tmp/$file" | cut -d' ' -f1)
else
  actual=$(shasum -a 256 "$tmp/$file" | cut -d' ' -f1)
fi
[ -n "$expected" ] && [ "$expected" = "$actual" ] || die "checksum mismatch for $file"

tar -xzf "$tmp/$file" -C "$tmp" whoruns

dir=${WHORUNS_INSTALL_DIR:-/usr/local/bin}
if [ -w "$dir" ] || { [ -n "${WHORUNS_INSTALL_DIR:-}" ] && mkdir -p "$dir"; }; then
  install -m 755 "$tmp/whoruns" "$dir/whoruns"
elif command -v sudo >/dev/null 2>&1; then
  echo "Installing to $dir (needs sudo)..."
  sudo install -m 755 "$tmp/whoruns" "$dir/whoruns"
else
  dir="$HOME/.local/bin"
  mkdir -p "$dir"
  install -m 755 "$tmp/whoruns" "$dir/whoruns"
fi

echo "Installed $("$dir/whoruns" --version) to $dir/whoruns"
case ":$PATH:" in
  *":$dir:"*) ;;
  *) echo "Note: $dir is not in your PATH. Add it to your shell profile." ;;
esac
