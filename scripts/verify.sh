#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
GO=${GO:-go}
"$GO" test -race ./...
"$GO" vet ./...
make build GO="$GO"
git diff --check

