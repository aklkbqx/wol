#!/bin/sh
# Installs the latest WOL release on macOS or Linux.
#
#   curl -fsSL https://raw.githubusercontent.com/aklkbqx/wol/main/install.sh | sh
#
# WOL_VERSION=0.4.20 picks a release; WOL_INSTALL_DIR changes the target
# directory (default ~/.local/bin). The archive is checked against the
# release's SHA256SUMS before anything is installed.
set -eu

repo=https://github.com/aklkbqx/wol
dir=${WOL_INSTALL_DIR:-$HOME/.local/bin}

fail() { echo "wol install: $*" >&2; exit 1; }

case $(uname -s) in
Darwin) os=darwin ;;
Linux) os=linux ;;
*) fail "unsupported system $(uname -s); see $repo#install" ;;
esac
case $(uname -m) in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) fail "unsupported CPU $(uname -m); see $repo#install" ;;
esac

version=${WOL_VERSION:-}
if [ -z "$version" ]; then
	latest=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$repo/releases/latest") ||
		fail "could not reach GitHub"
	version=${latest##*/v}
fi
version=${version#v}
file="wol_${version}_${os}_${arch}.tar.gz"
base="$repo/releases/download/v$version"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cd "$tmp"

echo "Downloading wol $version for $os/$arch"
curl -fsSL -o "$file" "$base/$file" || fail "download failed: $base/$file"
curl -fsSL -o SHA256SUMS "$base/SHA256SUMS" || fail "download failed: $base/SHA256SUMS"

want=$(awk -v f="$file" '$2 == f { print $1 }' SHA256SUMS)
[ -n "$want" ] || fail "$file is not listed in SHA256SUMS"
if command -v sha256sum >/dev/null 2>&1; then
	got=$(sha256sum "$file" | awk '{ print $1 }')
else
	got=$(shasum -a 256 "$file" | awk '{ print $1 }')
fi
[ "$got" = "$want" ] || fail "checksum mismatch for $file"

tar -xzf "$file" wol
mkdir -p "$dir"
install -m 755 wol "$dir/wol"
"$dir/wol" version >/dev/null || fail "the installed wol does not run"

echo "Installed wol $version to $dir/wol"
case ":$PATH:" in
*":$dir:"*) echo "Next: run wol doctor" ;;
*) echo "Add $dir to your PATH, then run wol doctor" ;;
esac
