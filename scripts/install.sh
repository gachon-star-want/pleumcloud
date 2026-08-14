#!/usr/bin/env bash
# PleumCloud one-line installer.
#
#   curl -fsSL https://pleumcloud.dev/install.sh | bash
#
# Downloads the latest release binary for the detected OS/arch, verifies its
# checksum, and installs it. PleumCloud is a single static binary — there are
# no runtime dependencies to install.
set -euo pipefail

REPO="${PLEUMCLOUD_REPO:-pleumcloud/pleumcloud}"
INSTALL_DIR_HINT="${PLEUMCLOUD_INSTALL_DIR:-}"

bold() { printf '\033[1m%s\033[0m\n' "$*"; }
die() { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

# ---- detect platform ------------------------------------------------------
OS="$(uname -s)"
ARCH="$(uname -m)"
case "$OS" in
  Darwin) GOOS=darwin ;;
  Linux)  GOOS=linux ;;
  *) die "Unsupported OS '$OS'. Windows: download the .zip from https://github.com/$REPO/releases" ;;
esac
case "$ARCH" in
  arm64|aarch64)      GOARCH=arm64 ;;
  x86_64|amd64|i686)  GOARCH=amd64 ;;
  *) die "Unsupported architecture '$ARCH'" ;;
esac

bold "☁️  Installing PleumCloud for ${GOOS}/${GOARCH}…"

# ---- resolve latest release ------------------------------------------------
ASSET="pleumcloud_${GOOS}_${GOARCH}.tar.gz"
URL_BASE="https://github.com/${REPO}/releases/latest/download"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

if ! curl -fsSL -o "$TMP/$ASSET" "$URL_BASE/$ASSET"; then
  die "Download failed. No release asset '$ASSET' — is there a published release at github.com/$REPO/releases?"
fi
curl -fsSL -o "$TMP/checksums.txt" "$URL_BASE/checksums.txt" || true

if [[ -f "$TMP/checksums.txt" ]]; then
  WANT="$(grep " $ASSET\$" "$TMP/checksums.txt" | awk '{print $1}' || true)"
  if [[ -n "$WANT" ]]; then
    if command -v shasum >/dev/null 2>&1; then
      GOT="$(shasum -a 256 "$TMP/$ASSET" | awk '{print $1}')"
      [[ "$GOT" = "$WANT" ]] || die "Checksum mismatch (want $WANT, got $GOT)"
      bold "✅ checksum verified"
    fi
  fi
fi

tar -xzf "$TMP/$ASSET" -C "$TMP"

# ---- choose install location ----------------------------------------------
BIN="$TMP/pleumcloud"
[[ -f "$BIN" ]] || die "Release archive did not contain a 'pleumcloud' binary"

if [[ -n "$INSTALL_DIR_HINT" ]]; then
  BINDIR="$INSTALL_DIR_HINT"
  mkdir -p "$BINDIR"
  install -m 0755 "$BIN" "$BINDIR/pleumcloud"
elif [[ -w /usr/local/bin ]]; then
  install -m 0755 "$BIN" /usr/local/bin/pleumcloud
  BINDIR=/usr/local/bin
elif command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null; then
  sudo install -m 0755 "$BIN" /usr/local/bin/pleumcloud
  BINDIR=/usr/local/bin
else
  BINDIR="$HOME/.local/bin"
  mkdir -p "$BINDIR"
  install -m 0755 "$BIN" "$BINDIR/pleumcloud"
fi

# ---- done ------------------------------------------------------------------
bold "✅ installed: $BINDIR/pleumcloud"

if [[ "$GOOS" == darwin ]] && command -v xattr >/dev/null 2>&1; then
  # Unsigned release binaries get quarantined by Gatekeeper; clear it.
  xattr -d com.apple.quarantine "$BINDIR/pleumcloud" 2>/dev/null || true
fi

if [[ ":$PATH:" != *":$BINDIR:"* ]]; then
  echo "⚠️  $BINDIR is not on your PATH. Add it with:"
  echo "    echo 'export PATH=\"$BINDIR:\$PATH\"' >> ~/.zshrc && source ~/.zshrc"
fi

bold "☁️  Done! Run \`pleumcloud\` and open http://localhost:7777"
