# Development and release

Go 1.26.9 or newer is required. The preferred release toolchain is Go 1.27.2.
Versions are pinned in go.mod/go.sum; use a current security patch release.
The TUI uses Bubble Tea, Lip Gloss and Bubbles v2 (`charm.land/*/v2`).

```sh
make verify
make install BINDIR="$HOME/.local/bin"
```

`make verify` runs the race tests, `go vet`, staticcheck, a native build and
`git diff --check`. The staticcheck version is pinned in `scripts/verify.sh`
(override with `STATICCHECK_VERSION`); the first run downloads it into the Go
module cache.

Architecture: CLI/TUI call store, presence, wake and remote services. The fleet
package coordinates independent machine jobs with global and per-site bounds.
Workers emit immutable results; TUI updates own all view state. Request IDs
discard obsolete scan results. Tests use temporary databases and fake network
operations rather than the maintainer's inventory.

Schema changes are additive and old imports are accepted. Export version 6 adds
site subnet, default relay and probe settings. Test migration, import/export
round trips, explicit remote overrides, cancellation, terminal widths and 100
machines across multiple sites.

## Website demo

`go run ./cmd/wol-sitedemo --out demo.json` records the terminal UI for the
website. It drives the real Wake Desk with key presses on a sample inventory
(documentation addresses 198.51.100.0/24 and 203.0.113.0/24, locally
administered MACs) and writes every distinct screen with the keys that lead
from it, for 100x28 and 40x36 by default (`--sizes`). Wakes, checks, connects
and batches are played back by stubs: nothing is sent on the network, nothing
is saved, and forms move between fields but never submit. The clock is fixed,
so two runs write the same file. Regenerate it after a change to the TUI.

## Local packaging

`make package` requires Go, make, Python 3 and (on macOS) Xcode command-line
tools. It creates archives and SHA256SUMS under dist/packages/VERSION for Linux
amd64/arm64, Windows amd64 and macOS amd64/arm64. macOS packages must be built on
macOS to embed the network usage description and sign the executable. Other
hosts skip macOS and must not claim a complete release.

Cross-compilation is not runtime validation. Before publishing, extract and
test the artifacts on each OS you claim to support, including native remote
launch, network permissions, Unicode/ASCII terminal operation and clean exit.
macOS builds are ad-hoc signed, not notarized.

## Release checklist

1. Review changes, document known limitations, run make verify and back up data.
2. Change internal/buildinfo/version.go to the real release version without
   -dev, commit, then run sh scripts/release-check.sh. It signs SHA256SUMS
   with the release key (WOL_RELEASE_SIGNING_KEY, default
   ~/.config/wol-release/wol-release-private.pem) and verifies the signature
   against docs/wol-release-public.pem.
3. Test all artifacts on their intended systems. Record results in release
   notes, including tests that could not be performed.
4. Build and install the production binary locally with make build and
   install -m 755 dist/wol ~/.local/bin/wol.
5. Create the matching git tag, publish the GitHub release with every
   supported platform archive, SHA256SUMS and SHA256SUMS.sig, and update the
   Homebrew tap with `make publish-homebrew-tap VERSION=X.Y.Z`.
6. Subsequent edits bump to the next -dev version and reinstall for testing.

Release scripts run locally and do not publish or push automatically.
Manifest signing, the release key and trusted-key update verification are
described in [release signing](release-signing.md). Releases through v0.4.15
are unsigned; v0.4.16 is the first signed release.
No GitHub Actions or third-party hosting configuration is used.

## Public beta acceptance

Before announcing a beta, have a new user follow the installation and first
machine guide. Validate three sites including an unavailable relay, 100
machines with mixed probe outcomes, interrupted batches, and backup restore.
Keep untested platforms explicitly marked as unverified.

