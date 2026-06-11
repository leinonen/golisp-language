#!/bin/sh
# glisp installer — downloads a prebuilt release binary for your platform.
#
#   curl -fsSL https://raw.githubusercontent.com/leinonen/golisp-language/main/install.sh | sh
#
# Environment overrides:
#   GLISP_VERSION      version tag to install (default: latest release)
#   GLISP_INSTALL_DIR  install directory (default: /usr/local/bin, else ~/.local/bin)
set -eu

REPO="leinonen/golisp-language"

err() { printf 'glisp install: %s\n' "$1" >&2; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }

# --- detect platform -------------------------------------------------------
os=$(uname -s)
case "$os" in
  Linux)  os=linux ;;
  Darwin) os=darwin ;;
  *) err "unsupported OS '$os' — for Windows, download the .zip from https://github.com/$REPO/releases" ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) err "unsupported architecture '$arch'" ;;
esac

# --- pick a downloader -----------------------------------------------------
if have curl; then
  dl() { curl -fsSL "$1"; }
  dlo() { curl -fsSL -o "$2" "$1"; }
elif have wget; then
  dl() { wget -qO- "$1"; }
  dlo() { wget -qO "$2" "$1"; }
else
  err "need curl or wget installed"
fi

# --- resolve version -------------------------------------------------------
version="${GLISP_VERSION:-}"
if [ -z "$version" ]; then
  version=$(dl "https://api.github.com/repos/$REPO/releases/latest" \
    | grep '"tag_name"' | head -n1 | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')
  [ -n "$version" ] || err "could not determine latest version — set GLISP_VERSION explicitly"
fi

# --- resolve install dir ---------------------------------------------------
install_dir="${GLISP_INSTALL_DIR:-}"
if [ -z "$install_dir" ]; then
  if [ -w /usr/local/bin ] 2>/dev/null; then
    install_dir=/usr/local/bin
  else
    install_dir="$HOME/.local/bin"
  fi
fi
mkdir -p "$install_dir" || err "cannot create install dir '$install_dir'"

# --- download + verify + extract ------------------------------------------
asset="glisp_${version}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$version"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

printf 'glisp install: downloading %s (%s)\n' "$asset" "$version"
dlo "$base/$asset" "$tmp/$asset" || err "download failed: $base/$asset"

# Verify checksum when SHA256SUMS is published and a checksum tool is available.
if dlo "$base/SHA256SUMS" "$tmp/SHA256SUMS" 2>/dev/null; then
  sum=$(grep " $asset\$" "$tmp/SHA256SUMS" | awk '{print $1}')
  if [ -n "$sum" ]; then
    if have sha256sum; then got=$(sha256sum "$tmp/$asset" | awk '{print $1}')
    elif have shasum; then got=$(shasum -a 256 "$tmp/$asset" | awk '{print $1}')
    else got=""; fi
    if [ -n "$got" ] && [ "$got" != "$sum" ]; then
      err "checksum mismatch for $asset (expected $sum, got $got)"
    fi
    [ -n "$got" ] && printf 'glisp install: checksum verified\n'
  fi
fi

tar -C "$tmp" -xzf "$tmp/$asset" || err "extract failed"
src="$tmp/glisp_${version}_${os}_${arch}"

for bin in glisp glisp-lsp; do
  install -m 0755 "$src/$bin" "$install_dir/$bin" 2>/dev/null \
    || { cp "$src/$bin" "$install_dir/$bin" && chmod 0755 "$install_dir/$bin"; } \
    || err "failed to install $bin to $install_dir (try sudo, or set GLISP_INSTALL_DIR)"
done

printf 'glisp install: installed glisp and glisp-lsp to %s\n' "$install_dir"
case ":$PATH:" in
  *":$install_dir:"*) ;;
  *) printf 'glisp install: add %s to your PATH:\n  export PATH="%s:$PATH"\n' "$install_dir" "$install_dir" ;;
esac
"$install_dir/glisp" version 2>/dev/null || true
