#!/bin/sh
# Install kaxa from a GitHub release on Linux or macOS.
#
#   curl -fsSL https://github.com/perylemke/kaxa/releases/latest/download/install.sh | sh
#
# Environment:
#   VERSION      Release to install, without the leading v (default: latest).
#   INSTALL_DIR  Where to put the binary (default: $HOME/.local/bin).
#
# The archive is checked against the release checksums.txt before anything is
# installed. Nothing here needs root.

set -eu

REPO_URL="https://github.com/perylemke/kaxa"
TMP=""

err() {
	echo "kaxa install: $*" >&2
	exit 1
}

cleanup() {
	if [ -n "$TMP" ]; then
		rm -rf "$TMP"
	fi
}

need() {
	command -v "$1" >/dev/null 2>&1 || err "missing required command: $1"
}

detect_os() {
	case "$(uname -s)" in
	Linux) echo linux ;;
	Darwin) echo darwin ;;
	*) err "unsupported operating system: $(uname -s) (on Windows use install.ps1)" ;;
	esac
}

detect_arch() {
	case "$(uname -m)" in
	x86_64 | amd64) echo amd64 ;;
	arm64 | aarch64) echo arm64 ;;
	*) err "unsupported architecture: $(uname -m)" ;;
	esac
}

# resolve_version prints the version without the leading v. The value ends up
# in a URL, so only characters that can appear in a release tag are accepted.
resolve_version() {
	version="${VERSION:-}"
	if [ -z "$version" ]; then
		url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$REPO_URL/releases/latest") ||
			err "could not reach $REPO_URL/releases/latest"
		version="${url##*/v}"
		[ "$version" != "$url" ] || err "could not determine the latest version"
	fi
	version="${version#v}"
	case "$version" in
	"" | *[!0-9A-Za-z.+-]*) err "invalid version: $version" ;;
	esac
	echo "$version"
}

sha256_of() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		err "missing required command: sha256sum or shasum"
	fi
}

# verify fails closed: a missing entry or a different hash stops the install.
verify() {
	dir="$1"
	file="$2"
	expected=$(awk -v f="$file" '$2 == f {print $1}' "$dir/checksums.txt")
	[ -n "$expected" ] || err "no checksum for $file in checksums.txt"
	actual=$(sha256_of "$dir/$file")
	[ "$expected" = "$actual" ] || err "checksum mismatch for $file"
	echo "$file: OK"
}

main() {
	need curl
	need tar
	need awk

	os=$(detect_os)
	arch=$(detect_arch)
	version=$(resolve_version)
	install_dir="${INSTALL_DIR:-$HOME/.local/bin}"
	file="kaxa_${version}_${os}_${arch}.tar.gz"
	base="$REPO_URL/releases/download/v${version}"

	TMP=$(mktemp -d)
	trap cleanup EXIT

	echo "Installing kaxa $version ($os/$arch) to $install_dir"
	curl -fsSL -o "$TMP/$file" "$base/$file" || err "could not download $base/$file"
	curl -fsSL -o "$TMP/checksums.txt" "$base/checksums.txt" || err "could not download $base/checksums.txt"
	verify "$TMP" "$file"

	tar -xzf "$TMP/$file" -C "$TMP" kaxa
	mkdir -p "$install_dir"
	# Copy next to the target, then rename, so a running kaxa is replaced atomically.
	cp "$TMP/kaxa" "$install_dir/.kaxa.$$"
	chmod 755 "$install_dir/.kaxa.$$"
	mv -f "$install_dir/.kaxa.$$" "$install_dir/kaxa"

	echo "Installed $install_dir/kaxa"
	case ":$PATH:" in
	*":$install_dir:"*) ;;
	*) echo "Add it to your PATH: export PATH=\"$install_dir:\$PATH\"" ;;
	esac
}

main "$@"
