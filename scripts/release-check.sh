#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
version=$(sed -n 's/.*Version = "\([^"]*\)".*/\1/p' internal/buildinfo/version.go)
case "$version" in ''|*-dev) echo 'Set a real release version before preparing a release.' >&2; exit 1;; esac
[ -z "$(git status --porcelain)" ] || { echo 'Commit reviewed changes before release preparation.' >&2; exit 1; }
sh scripts/verify.sh
sh scripts/package.sh
printf 'Prepared %s. Test artifacts on supported operating systems before tagging and publishing.\n' "$version"

