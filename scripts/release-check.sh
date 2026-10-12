#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
version=$(sed -n 's/.*Version = "\([^"]*\)".*/\1/p' internal/buildinfo/version.go)
case "$version" in ''|*-dev) echo 'Set a real release version before preparing a release.' >&2; exit 1;; esac
[ -z "$(git status --porcelain)" ] || { echo 'Commit reviewed changes before release preparation.' >&2; exit 1; }
key=${WOL_RELEASE_SIGNING_KEY:-${XDG_CONFIG_HOME:-$HOME/.config}/wol-release/wol-release-private.pem}
if [ "${WOL_RELEASE_UNSIGNED:-}" != 1 ] && [ ! -f "$key" ]; then
 echo "Release signing key not found at $key. Set WOL_RELEASE_SIGNING_KEY, or WOL_RELEASE_UNSIGNED=1 for a dry run." >&2
 exit 1
fi
sh scripts/verify.sh
sh scripts/package.sh
out="dist/packages/$version"
if [ "${WOL_RELEASE_UNSIGNED:-}" = 1 ]; then
 echo 'WOL_RELEASE_UNSIGNED=1: SHA256SUMS is not signed; do not publish these archives.' >&2
else
 rm -f "$out/SHA256SUMS.sig"
 go run ./cmd/wol-release-sign sign --key "$key" --manifest "$out/SHA256SUMS" --output "$out/SHA256SUMS.sig"
 go run ./cmd/wol-release-sign verify --key docs/wol-release-public.pem --manifest "$out/SHA256SUMS" --signature "$out/SHA256SUMS.sig"
fi
printf 'Prepared %s. Test artifacts on supported operating systems before tagging and publishing.\n' "$version"
