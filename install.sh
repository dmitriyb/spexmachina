#!/usr/bin/env bash
# spex install / self-update script.
#
# With no flags this is first-install mode: resolve the latest signed
# release, download it, verify it, and install it into an install
# directory. Passing --upgrade switches to upgrade mode: the destination
# becomes the exact path of an already-installed binary, and replacement is
# a staged atomic rename with a kept backup rather than a plain copy. The
# resolve/download/verify logic is identical between the two modes; upgrade
# mode only adds decisions around it (see spec/delivery/arch_self_update.md
# — this file is that leaf's contract in force).
#
# This exact file lives at the repository root and ships with each release
# as the verified download path the README documents. A byte-identical
# copy is embedded in the spex binary (delivery/install.sh) so `spex
# upgrade` drives precisely this script against its own running binary,
# with nothing fetched at upgrade time — see
# spec/delivery/arch_self_update.md#embedded-equals-released. A
# build-failing Go test pins the two copies together.
set -euo pipefail

# --- Trust anchor -----------------------------------------------------
#
# The public half of the Ed25519 release-signing key shared with the
# sibling tools faber and portitor (the private half is the
# SSH_SIGNING_KEY repository secret .goreleaser.yaml signs archives
# with). The same base64 appears in those repositories' READMEs and
# install scripts; it must stay byte-identical across all three.
# Deliberately not overridable by any environment variable or flag: a
# test harness that needs to exercise this script against a throwaway
# release bakes a throwaway key into its own copy of this file, exactly
# as a real release bakes the real one. Whatever origin serves a
# download, the archive must still verify against this key.
#
# Identity and key together form the allowed_signers line this script
# writes and verifies against — byte-identical to the line faber and
# portitor publish, so one pinned copy serves all three.
SPEX_TRUST_IDENTITY="dvbozhko@gmail.com"
SPEX_RELEASE_PUBKEY="ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIIhmCWVDP/Tcm3CqXNjTQTChbKxr223xMob9zc56Uuny release signing"

# --- Test-only origin hooks ---------------------------------------------
#
# Not trust-sensitive: they only decide where bytes are fetched from, and
# whatever serves them must still verify against SPEX_RELEASE_PUBKEY
# above. Default to the production endpoints; a fake-origin test harness
# overrides them to point at a local server shaped like the same routes.
SPEX_INSTALL_API_ORIGIN="${SPEX_INSTALL_API_ORIGIN:-https://api.github.com}"
SPEX_INSTALL_RELEASE_ORIGIN="${SPEX_INSTALL_RELEASE_ORIGIN:-https://github.com}"

SPEX_REPO_SLUG="dmitriyb/spexmachina"
SPEX_BIN_NAME="spex"

# --- Usage / arg parsing -------------------------------------------------

usage() {
  cat <<'EOF'
Usage:
  install.sh [--dir DIR]
  install.sh --upgrade --target PATH [--current-version VERSION]
  install.sh --upgrade --target PATH --check
  install.sh --upgrade --target PATH --rollback

Modes:
  (no --upgrade)          first-install mode: install the latest (or
                           pinned, via SPEX_INSTALL_VERSION) release into
                           DIR (default: $HOME/.local/bin).
  --upgrade --target PATH upgrade mode: replace the binary at PATH in
                           place with a staged atomic rename, keeping the
                           replaced binary as PATH.bak.

Flags:
  --dir DIR              first-install mode install directory.
  --target PATH           upgrade mode: the binary to replace.
  --current-version V     upgrade mode: the version currently at PATH.
                           If omitted, PATH is probed for one.
  --check, --dry-run       report the resolve/compare outcome and change
                           nothing; exits 0 even when the outcome is an
                           anomaly.
  --rollback               restore PATH.bak over PATH and exit.
  -h, --help                print this message and exit.

Environment:
  SPEX_INSTALL_VERSION         pin an exact version (e.g. v1.2.3) instead
                                of resolving the latest release. Installs
                                in any direction; unset selects the
                                forward-only latest path.
  SPEX_INSTALL_API_ORIGIN      override the release-metadata origin
                                (test-only; defaults to production).
  SPEX_INSTALL_RELEASE_ORIGIN  override the release-download origin
                                (test-only; defaults to production).
EOF
}

die() {
  printf 'install.sh: %s\n' "$*" >&2
  exit 1
}

die_usage() {
  printf 'install.sh: %s\n' "$*" >&2
  exit 2
}

warn() {
  printf 'install.sh: warning: %s\n' "$*" >&2
}

notice() {
  printf 'install.sh: %s\n' "$*"
}

UPGRADE=false
TARGET=""
INSTALL_DIR=""
CURRENT_VERSION=""
CHECK=false
ROLLBACK=false

# Preserved for the first-install sudo re-exec below, since the parsing
# loop shifts "$@" away as it goes.
ORIG_ARGS=("$@")

while [ $# -gt 0 ]; do
  case "$1" in
    --upgrade)
      UPGRADE=true
      shift
      ;;
    --target)
      [ $# -ge 2 ] || die_usage "--target requires a value"
      TARGET="$2"
      shift 2
      ;;
    --dir)
      [ $# -ge 2 ] || die_usage "--dir requires a value"
      INSTALL_DIR="$2"
      shift 2
      ;;
    --current-version)
      [ $# -ge 2 ] || die_usage "--current-version requires a value"
      CURRENT_VERSION="$2"
      shift 2
      ;;
    --check|--dry-run)
      CHECK=true
      shift
      ;;
    --rollback)
      ROLLBACK=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die_usage "unknown option: $1"
      ;;
  esac
done

if $UPGRADE && [ -z "$TARGET" ]; then
  die_usage "--upgrade requires --target PATH"
fi

# --- Downloader selection -------------------------------------------------

if command -v curl >/dev/null 2>&1; then
  SPEX_DOWNLOADER=curl
elif command -v wget >/dev/null 2>&1; then
  SPEX_DOWNLOADER=wget
else
  die "curl or wget is required"
fi

fetch_stdout() {
  local url="$1"
  if [ "$SPEX_DOWNLOADER" = curl ]; then
    curl -fsSL "$url"
  else
    wget -qO- "$url"
  fi
}

fetch_file() {
  local url="$1"
  local out="$2"
  if [ "$SPEX_DOWNLOADER" = curl ]; then
    if ! curl -fsSL -o "$out" "$url"; then
      rm -f "$out"
      return 1
    fi
  else
    if ! wget -qO "$out" "$url"; then
      rm -f "$out"
      return 1
    fi
  fi
}

# --- Platform detection ----------------------------------------------------

detect_platform() {
  case "$(uname -s)" in
    Linux) SPEX_GOOS=linux ;;
    Darwin) SPEX_GOOS=darwin ;;
    *) die "unsupported operating system: $(uname -s)" ;;
  esac
  case "$(uname -m)" in
    x86_64|amd64) SPEX_GOARCH=amd64 ;;
    arm64|aarch64) SPEX_GOARCH=arm64 ;;
    *) die "unsupported architecture: $(uname -m)" ;;
  esac
}

# --- Version handling --------------------------------------------------

# strip_v removes a leading "v" from a version string, matching
# GoReleaser's .Version template variable (used in archive names) versus
# its .Tag variable (used in the release/download URLs).
strip_v() {
  local v="$1"
  printf '%s' "${v#v}"
}

# orderable prints the normalized (no leading "v") form of $1 and returns
# success if it is shaped like a plain release version (X.Y.Z, digits
# only). Anything else - "dev", "unknown", an unstamped empty string, a
# prerelease suffix - is not orderable against another version: a
# signature proves authenticity, not a place in a sequence.
orderable() {
  local v
  v="$(strip_v "$1")"
  case "$v" in
    ''|*[!0-9.]*) return 1 ;;
  esac
  printf '%s' "$v"
}

# compare_versions prints one of: lt, eq, gt, unordered — ordering $1
# (current) against $2 (resolved).
compare_versions() {
  local cur="$1"
  local res="$2"
  local ncur nres first
  if [ "$cur" = "$res" ]; then
    printf 'eq'
    return
  fi
  ncur="$(orderable "$cur")" || { printf 'unordered'; return; }
  nres="$(orderable "$res")" || { printf 'unordered'; return; }
  if [ "$ncur" = "$nres" ]; then
    printf 'eq'
    return
  fi
  first="$(printf '%s\n%s\n' "$ncur" "$nres" | sort -V | head -n1)"
  if [ "$first" = "$ncur" ]; then
    printf 'lt'
  else
    printf 'gt'
  fi
}

# probe_version runs "$1 version" and extracts the second field of its
# first line (spex version prints "spex vX.Y.Z" first) — the fallback the
# compiled-in --current-version stamp exists to avoid.
probe_version() {
  local target="$1"
  local out v
  if [ -x "$target" ]; then
    out="$("$target" version 2>/dev/null)" || { printf 'dev'; return; }
    v="$(printf '%s\n' "$out" | head -n1 | awk '{print $2}')"
    if [ -n "$v" ]; then
      printf '%s' "$v"
      return
    fi
  fi
  printf 'dev'
}

resolve_latest() {
  local api_url body tag
  api_url="${SPEX_INSTALL_API_ORIGIN%/}/repos/${SPEX_REPO_SLUG}/releases/latest"
  body="$(fetch_stdout "$api_url")" || die "resolve latest release: request failed"
  tag="$(printf '%s' "$body" | grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' | head -n1 | sed -E 's/.*"([^"]*)"$/\1/')"
  [ -n "$tag" ] || die "resolve latest release: could not read tag_name from response"
  printf '%s' "$tag"
}

# --- Rollback ------------------------------------------------------------

do_rollback() {
  local backup target_dir
  backup="${TARGET}.bak"
  [ -e "$backup" ] || die "rollback: no backup found at $backup"
  target_dir="$(dirname "$TARGET")"
  if [ ! -w "$target_dir" ]; then
    die "target directory not writable; re-run elevated and try again"
  fi
  mv -f "$backup" "$TARGET"
  notice "rolled back $TARGET from $backup"
}

# --- Safe self-replace -----------------------------------------------------

# stage_and_verify downloads the archive and signature for $1 (a resolved
# version tag) into a fresh temp directory and verifies the archive
# against SPEX_RELEASE_PUBKEY. On success it echoes the temp directory
# (containing the verified archive); on failure it cleans up after itself
# and returns non-zero — fail-closed, nothing on disk changed by this
# function ever touches the install/target directory.
stage_and_verify() {
  local tag ver archive_name base_url dl_dir archive sig allowed_signers
  tag="$1"
  ver="$(strip_v "$tag")"
  archive_name="${SPEX_BIN_NAME}_${ver}_${SPEX_GOOS}_${SPEX_GOARCH}.tar.gz"
  base_url="${SPEX_INSTALL_RELEASE_ORIGIN%/}/${SPEX_REPO_SLUG}/releases/download/${tag}"

  dl_dir="$(mktemp -d)"
  archive="${dl_dir}/${archive_name}"
  sig="${archive}.sig"

  if ! fetch_file "${base_url}/${archive_name}" "$archive"; then
    rm -rf "$dl_dir"
    return 2
  fi
  if ! fetch_file "${base_url}/${archive_name}.sig" "$sig"; then
    rm -rf "$dl_dir"
    return 2
  fi

  allowed_signers="${dl_dir}/allowed_signers"
  printf '%s %s\n' "$SPEX_TRUST_IDENTITY" "$SPEX_RELEASE_PUBKEY" > "$allowed_signers"

  if ! ssh-keygen -Y verify -f "$allowed_signers" -I "$SPEX_TRUST_IDENTITY" -n file -s "$sig" < "$archive" >/dev/null 2>&1; then
    rm -rf "$dl_dir"
    return 3
  fi

  printf '%s %s\n' "$dl_dir" "$archive"
}

# fetch_and_verify_or_die calls stage_and_verify for $1 and, on failure,
# dies with a message specific to how it failed (download vs. signature).
# It must run at top level, never inside a command substitution of its
# own, so that die's exit actually stops the script. On success it sets
# dl_dir and archive in the caller's shell.
fetch_and_verify_or_die() {
  local result rc
  set +e
  result="$(stage_and_verify "$1")"
  rc=$?
  set -e
  case $rc in
    2) die "download failed for $1" ;;
    3) die "signature verification failed for $1: refusing to install" ;;
  esac
  read -r dl_dir archive <<<"$result"
}

extract_binary() {
  local archive="$1"
  local dest_dir="$2"
  local extracted
  tar -xzf "$archive" -C "$dest_dir"
  extracted="${dest_dir}/${SPEX_BIN_NAME}"
  [ -f "$extracted" ] || die "archive did not contain ${SPEX_BIN_NAME}"
  chmod +x "$extracted"
  printf '%s' "$extracted"
}

# replace_target stages $1 (an extracted, verified binary) into TARGET's
# own directory, moves the running binary aside as TARGET.bak, and
# renames the staged binary into TARGET — same filesystem throughout, so
# the swap that matters (staged file -> TARGET) is one atomic rename. On
# any failure during the swap the backup is restored before returning
# non-zero, so TARGET is never left missing or half-updated.
replace_target() {
  local new_bin="$1"
  local target_dir staged backup
  target_dir="$(dirname "$TARGET")"
  staged="${target_dir}/.$(basename "$TARGET").new.$$"
  backup="${TARGET}.bak"

  if ! cp "$new_bin" "$staged"; then
    rm -f "$staged"
    return 1
  fi
  if ! chmod +x "$staged"; then
    rm -f "$staged"
    return 1
  fi

  if [ -e "$TARGET" ]; then
    if ! mv -f "$TARGET" "$backup"; then
      rm -f "$staged"
      return 1
    fi
  fi

  if ! mv -f "$staged" "$TARGET"; then
    if [ -e "$backup" ]; then
      mv -f "$backup" "$TARGET" || true
    fi
    rm -f "$staged"
    return 1
  fi
}

# --- Main ------------------------------------------------------------------

detect_platform

if $ROLLBACK; then
  do_rollback
  exit 0
fi

if $UPGRADE; then
  if [ -n "$CURRENT_VERSION" ]; then
    current="$CURRENT_VERSION"
  else
    current="$(probe_version "$TARGET")"
  fi
else
  INSTALL_DIR="${INSTALL_DIR:-${SPEX_INSTALL_DIR:-$HOME/.local/bin}}"
fi

pin="${SPEX_INSTALL_VERSION:-}"
if [ -n "$pin" ]; then
  explicit=true
  resolved="$pin"
else
  explicit=false
  resolved="$(resolve_latest)"
fi

if $UPGRADE; then
  cmp="unordered"
  if ! $explicit; then
    cmp="$(compare_versions "$current" "$resolved")"
  fi

  if $CHECK; then
    notice "current version:  $current"
    notice "resolved version: $resolved"
    if $explicit; then
      notice "comparison: explicit (as explicitly requested)"
    else
      case "$cmp" in
        eq) notice "comparison: up to date" ;;
        lt) notice "comparison: newer available" ;;
        gt) notice "comparison: anomaly (resolved release is older than installed — forward-only guard would refuse this)" ;;
        unordered) notice "comparison: unordered (current version cannot be ordered)" ;;
      esac
    fi
    exit 0
  fi

  if $explicit; then
    notice "installing $resolved as explicitly requested"
  else
    case "$cmp" in
      eq)
        notice "already up to date at $current"
        exit 0
        ;;
      gt)
        die "refusing to install $resolved: older than installed $current (forward-only guard)"
        ;;
      unordered)
        warn "current version \"$current\" cannot be ordered; proceeding with $resolved"
        ;;
      lt) : ;;
    esac
  fi

  target_dir="$(dirname "$TARGET")"
  if [ ! -w "$target_dir" ]; then
    die "target directory not writable; re-run elevated (e.g. with sudo) and try again"
  fi

  work_dir="$(mktemp -d)"
  trap 'rm -rf "$work_dir"' EXIT

  fetch_and_verify_or_die "$resolved"

  extracted="$(extract_binary "$archive" "$work_dir")"
  rm -rf "$dl_dir"

  if ! replace_target "$extracted"; then
    die "self-replace failed; backup restored"
  fi

  notice "upgraded to $resolved at $TARGET (previous binary kept as ${TARGET}.bak)"
  exit 0
fi

# First-install mode.
mkdir -p "$INSTALL_DIR" 2>/dev/null || true
if [ ! -w "$INSTALL_DIR" ]; then
  if [ "$(id -u)" != "0" ] && command -v sudo >/dev/null 2>&1; then
    # sudo's env_reset strips SPEX_INSTALL_VERSION/SPEX_INSTALL_API_ORIGIN/
    # SPEX_INSTALL_RELEASE_ORIGIN by default, and "$@" was already consumed
    # by the arg-parsing loop above — replay the original arguments and
    # forward the resolved install dir and pinned version explicitly so the
    # elevated run installs exactly where and what the caller asked for.
    exec sudo env \
      SPEX_INSTALL_VERSION="${SPEX_INSTALL_VERSION:-}" \
      SPEX_INSTALL_API_ORIGIN="$SPEX_INSTALL_API_ORIGIN" \
      SPEX_INSTALL_RELEASE_ORIGIN="$SPEX_INSTALL_RELEASE_ORIGIN" \
      "$0" "${ORIG_ARGS[@]}" --dir "$INSTALL_DIR"
  fi
  die "install directory $INSTALL_DIR is not writable"
fi

work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

fetch_and_verify_or_die "$resolved"

extracted="$(extract_binary "$archive" "$work_dir")"
rm -rf "$dl_dir"

mkdir -p "$INSTALL_DIR"
install_path="${INSTALL_DIR}/${SPEX_BIN_NAME}"
cp "$extracted" "$install_path"
chmod +x "$install_path"

notice "installed $resolved at $install_path"
