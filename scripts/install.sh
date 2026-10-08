#!/bin/sh
# Envis installer for macOS and Linux.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/umairx25/Envisible/main/scripts/install.sh | sh
#
# Environment overrides:
#   ENVIS_VERSION   install a specific tag (e.g. v1.0.0); default: latest
#   ENVIS_INSTALL_DIR   target bin dir; default: $HOME/.local/bin
set -eu

REPO="umairx25/Envisible"
INSTALL_DIR="${ENVIS_INSTALL_DIR:-$HOME/.local/bin}"

err() { printf 'error: %s\n' "$1" >&2; exit 1; }
info() { printf '%s\n' "$1" >&2; }

# --- detect OS -------------------------------------------------------------
os="$(uname -s)"
case "$os" in
  Darwin) os="darwin" ;;
  Linux)  os="linux" ;;
  *) err "unsupported OS: $os (use the PowerShell installer on Windows)" ;;
esac

# --- detect architecture ---------------------------------------------------
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) err "unsupported architecture: $arch" ;;
esac

# --- required tools --------------------------------------------------------
if command -v curl >/dev/null 2>&1; then
  dl() { curl -fsSL "$1" -o "$2"; }
  fetch() { curl -fsSL "$1"; }
elif command -v wget >/dev/null 2>&1; then
  dl() { wget -qO "$2" "$1"; }
  fetch() { wget -qO- "$1"; }
else
  err "need curl or wget to download"
fi

# --- resolve version -------------------------------------------------------
version="${ENVIS_VERSION:-}"
if [ -z "$version" ]; then
  info "Resolving latest release..."
  # Follow the GitHub "latest" redirect to read the tag, no API token needed.
  version="$(fetch "https://api.github.com/repos/${REPO}/releases/latest" \
    | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
  [ -n "$version" ] || err "could not determine latest version (set ENVIS_VERSION)"
fi

name="envis-${os}-${arch}"
archive="${name}.tar.gz"
base="https://github.com/${REPO}/releases/download/${version}"

info "Installing envis ${version} for ${os}/${arch}..."

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# --- download archive + checksums -----------------------------------------
dl "${base}/${archive}" "${tmp}/${archive}" || err "download failed: ${base}/${archive}"

# Verify the checksum when a verifier is available (best-effort).
if dl "${base}/SHA256SUMS" "${tmp}/SHA256SUMS" 2>/dev/null; then
  expected="$(grep " ${archive}\$" "${tmp}/SHA256SUMS" | awk '{print $1}' | head -n1)"
  if [ -n "$expected" ]; then
    if command -v sha256sum >/dev/null 2>&1; then
      actual="$(sha256sum "${tmp}/${archive}" | awk '{print $1}')"
    elif command -v shasum >/dev/null 2>&1; then
      actual="$(shasum -a 256 "${tmp}/${archive}" | awk '{print $1}')"
    else
      actual=""
    fi
    if [ -n "$actual" ] && [ "$actual" != "$expected" ]; then
      err "checksum mismatch for ${archive} (expected ${expected}, got ${actual})"
    fi
    [ -n "$actual" ] && info "Checksum verified."
  fi
else
  info "Warning: could not fetch SHA256SUMS; skipping checksum verification."
fi

# --- extract and install ---------------------------------------------------
tar -xzf "${tmp}/${archive}" -C "$tmp"
[ -f "${tmp}/envis" ] || err "archive did not contain the envis binary"

mkdir -p "$INSTALL_DIR"
install -m 0755 "${tmp}/envis" "${INSTALL_DIR}/envis" 2>/dev/null \
  || { cp "${tmp}/envis" "${INSTALL_DIR}/envis" && chmod 0755 "${INSTALL_DIR}/envis"; }

info "Installed envis to ${INSTALL_DIR}/envis"

# --- PATH hint -------------------------------------------------------------
case ":${PATH}:" in
  *":${INSTALL_DIR}:"*) : ;;
  *)
    info ""
    info "Add ${INSTALL_DIR} to your PATH, e.g.:"
    info "  echo 'export PATH=\"${INSTALL_DIR}:\$PATH\"' >> ~/.zshrc && source ~/.zshrc"
    ;;
esac

info ""
info "Run 'envis version' to confirm, then 'envis init' in a project."
