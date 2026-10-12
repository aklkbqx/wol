#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
GO=${GO:-go}
STATICCHECK_VERSION=${STATICCHECK_VERSION:-2026.2.2}
"$GO" test -race ./...
"$GO" vet ./...
"$GO" run "honnef.co/go/tools/cmd/staticcheck@$STATICCHECK_VERSION" ./...
make build GO="$GO"
git diff --check
