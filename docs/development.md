# Development and release

Go 1.26.6 or newer is required. Versions are pinned in go.mod/go.sum.
The current TUI uses Bubble Tea v1; do not copy v2 APIs into this codebase.

```sh
make verify
make install BINDIR="$HOME/.local/bin"
```

Architecture: CLI/TUI call store, presence, wake and remote services. The fleet
package coordinates independent machine jobs with global and per-site bounds.
Workers emit immutable results; TUI updates own all view state. Request IDs
discard obsolete scan results. Tests use temporary databases and fake network
operations rather than the maintainer's inventory.

Schema changes are additive and old imports are accepted. Export version 6 adds
site subnet, default relay and probe settings. Test migration, import/export
round trips, explicit remote overrides, cancellation, terminal widths and 100
machines across multiple sites.

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
   -dev, commit, then run sh scripts/release-check.sh.
3. Test all artifacts on their intended systems. Record results in release
   notes, including tests that could not be performed.
4. Build and install the production binary locally with make build and
   install -m 755 dist/wol ~/.local/bin/wol.
5. Create the matching git tag and publish the GitHub release with every
   supported platform archive and SHA256SUMS.
6. Subsequent edits bump to the next -dev version and reinstall for testing.

Release scripts run locally and do not publish or push automatically.
No GitHub Actions or third-party hosting configuration is used.

## Public beta acceptance

Before announcing a beta, have a new user follow the installation and first
machine guide. Validate three sites including an unavailable relay, 100
machines with mixed probe outcomes, interrupted batches, and backup restore.
Keep untested platforms explicitly marked as unverified.

