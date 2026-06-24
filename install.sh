#!/usr/bin/env bash
set -euo pipefail

OWNER="wagneripjr"
REPO="kubectl-tessera"
BINARY="kubectl-tessera"
BASE_URL="https://github.com/${OWNER}/${REPO}/releases/download"
API_LATEST="https://api.github.com/repos/${OWNER}/${REPO}/releases/latest"
OIDC_ISSUER="https://token.actions.githubusercontent.com"
IDENTITY_REGEXP="^https://github.com/${OWNER}/${REPO}/\\.github/workflows/release\\.yaml@.+$"

VERSION="${TESSERA_VERSION:-}"
BIN_DIR=""
NO_VERIFY="false"
DRY_RUN="false"
work=""

err() { printf 'tessera-install: %s\n' "$*" >&2; }
info() { printf '==> %s\n' "$*" >&2; }
die() { err "$*"; exit 1; }
need_cmd() { command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"; }

usage() {
  cat >&2 <<EOF
Install kubectl-tessera (the 'kubectl tessera' plugin) from signed GitHub releases.

Usage: install.sh [options]

Options:
  -v, --version <vX.Y.Z>   Release tag to install (default: latest).
  -b, --bin-dir <dir>      Install directory (default: /usr/local/bin, else ~/.local/bin).
      --no-verify          Skip sha256 + cosign verification (not recommended).
      --dry-run            Print what would happen; download nothing.
  -h, --help               Show this help.

Environment:
  TESSERA_VERSION          Same as --version.

The archive's SHA-256 is verified against the release checksums.txt (always).
When cosign is on PATH, the keyless signature of checksums.txt is also verified;
a present-but-failing cosign aborts the install.
EOF
}

parse_args() {
  while [ $# -gt 0 ]; do
    case "$1" in
      -v|--version) [ $# -ge 2 ] || die "--version needs a value"; VERSION="$2"; shift 2 ;;
      -b|--bin-dir) [ $# -ge 2 ] || die "--bin-dir needs a value"; BIN_DIR="$2"; shift 2 ;;
      --no-verify) NO_VERIFY="true"; shift ;;
      --dry-run) DRY_RUN="true"; shift ;;
      -h|--help) usage; exit 0 ;;
      *) die "unknown argument: $1 (try --help)" ;;
    esac
  done
}

detect_os() {
  local os
  os="$(uname -s)"
  case "$os" in
    Linux) printf 'linux' ;;
    Darwin) printf 'darwin' ;;
    *) die "unsupported OS '$os' — use krew, or grab the archive from ${BASE_URL}" ;;
  esac
}

detect_arch() {
  local arch
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64) printf 'amd64' ;;
    aarch64|arm64) printf 'arm64' ;;
    *) die "unsupported architecture '$arch'" ;;
  esac
}

resolve_version() {
  if [ -n "$VERSION" ]; then printf '%s' "$VERSION"; return; fi
  local tag
  tag="$(curl -fsSL "$API_LATEST" | grep '"tag_name"' | head -n1 \
    | sed -E 's/.*"tag_name"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/' || true)"
  [ -n "$tag" ] || die "could not resolve the latest release tag from ${API_LATEST}"
  printf '%s' "$tag"
}

resolve_bin_dir() {
  if [ -n "$BIN_DIR" ]; then printf '%s' "$BIN_DIR"; return; fi
  if [ -d /usr/local/bin ]; then printf '%s' /usr/local/bin; return; fi
  printf '%s' "${HOME}/.local/bin"
}

fetch() {
  curl -fsSL "$1" -o "$2" || die "download failed: $1"
}

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    die "need sha256sum or shasum to verify the download"
  fi
}

verify_sha256() {
  local archive="$1" checksums="$2" name="$3" want got
  want="$(awk -v f="$name" '$2 == f {print $1}' "$checksums" | head -n1)"
  [ -n "$want" ] || die "no checksum for ${name} in checksums.txt"
  got="$(sha256_of "$archive")"
  [ "$want" = "$got" ] || die "sha256 mismatch for ${name} (want ${want}, got ${got})"
  info "sha256 OK (${name})"
}

verify_cosign() {
  cosign verify-blob \
    --bundle "$2" \
    --certificate-oidc-issuer "$OIDC_ISSUER" \
    --certificate-identity-regexp "$IDENTITY_REGEXP" \
    "$1" >/dev/null 2>&1 \
    || die "cosign signature verification failed for checksums.txt"
  info "cosign signature OK (checksums.txt)"
}

install_binary() {
  local src="$1" dir="$2" sudo_cmd=""
  if [ ! -d "$dir" ]; then
    mkdir -p "$dir" 2>/dev/null || sudo mkdir -p "$dir" || die "cannot create $dir"
  fi
  [ -w "$dir" ] || sudo_cmd="sudo"
  ${sudo_cmd} install -m 0755 "$src" "${dir}/${BINARY}" || die "failed to install into $dir"
  info "installed ${BINARY} -> ${dir}/${BINARY}"
}

check_path() {
  case ":${PATH}:" in
    *":$1:"*) : ;;
    *) err "note: $1 is not on your PATH — add it, e.g. export PATH=\"$1:\$PATH\"" ;;
  esac
}

main() {
  parse_args "$@"
  need_cmd curl
  need_cmd uname
  need_cmd tar

  local os arch version archive url bin_dir
  os="$(detect_os)"
  arch="$(detect_arch)"
  version="$(resolve_version)"
  archive="${BINARY}_${version}_${os}_${arch}.tar.gz"
  url="${BASE_URL}/${version}/${archive}"
  bin_dir="$(resolve_bin_dir)"

  if [ "$DRY_RUN" = "true" ]; then
    info "version : ${version}"
    info "platform: ${os}/${arch}"
    info "archive : ${url}"
    info "bin dir : ${bin_dir}"
    info "(dry-run — nothing downloaded)"
    exit 0
  fi

  work="$(mktemp -d)"
  trap 'rm -rf "${work:-}"' EXIT

  info "downloading ${archive} (${version})"
  fetch "$url" "${work}/${archive}"

  if [ "$NO_VERIFY" = "true" ]; then
    err "WARNING: --no-verify set — skipping integrity and signature checks"
  else
    fetch "${BASE_URL}/${version}/checksums.txt" "${work}/checksums.txt"
    verify_sha256 "${work}/${archive}" "${work}/checksums.txt" "${archive}"
    if command -v cosign >/dev/null 2>&1; then
      fetch "${BASE_URL}/${version}/checksums.txt.bundle" "${work}/checksums.txt.bundle"
      verify_cosign "${work}/checksums.txt" "${work}/checksums.txt.bundle"
    else
      info "cosign not found — skipping signature check (sha256 still enforced)"
    fi
  fi

  tar -xzf "${work}/${archive}" -C "$work" || die "failed to extract ${archive}"
  [ -f "${work}/${BINARY}" ] || die "binary ${BINARY} not found in archive"

  install_binary "${work}/${BINARY}" "$bin_dir"
  check_path "$bin_dir"

  if [ -x "${bin_dir}/${BINARY}" ]; then
    info "verifying install:"
    "${bin_dir}/${BINARY}" version || true
  fi
  info "done — run: kubectl tessera version"
}

main "$@"
