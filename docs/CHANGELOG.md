# 0.4.18 — 2026-10-15

- Improve normal-text contrast on dark TUI panels and add `WOL_TUI_REDACT=1`
  to hide identifiers and diagnostic details without changing action inputs (#19).

# 0.4.17 — 2026-10-13

- `wol shutdown` keeps flags that follow the machine name when other flags come
  before it. Before, `wol shutdown --db FILE pc --cancel` dropped `--cancel`
  and shut the machine down. Two machine names are now a usage error (#16).
- Tests cover `wol export`, `wol shutdown status` and `wol shutdown cancel` (#17).

# 0.4.16 — 2026-10-13

- `wol batch`, `wol status`, `wol import` and `wol remote` accept flags after
  machine names and files, like `wol wake` (#15).
- Releases are signed: `SHA256SUMS.sig` is an Ed25519 signature over the
  manifest, checkable with `WOL_UPDATE_PUBLIC_KEY` and the public key in
  `docs/wol-release-public.pem` (#14).
- `wol-release-sign fingerprint` prints a public key's SHA-256 fingerprint.
- The browser-remote proxy sends Guacamole only its own `X-Forwarded-*`
  headers; values supplied by the browser are dropped (#9).
- `make verify` runs staticcheck again; unused code and error-string issues it
  reported are fixed (#9).

# 0.4.15 — 2026-10-12

- `wol wake` accepts flags after the MAC address (`wol wake MAC --destination X`);
  arguments after `--` stay positional (#11).
- `wol wake --verify` returns within its timeout instead of up to a second
  later (#12).
- On Windows, the `wol.exe.old` left by a self-update is removed the next time
  WOL starts (#10).

# 0.4.14 — 2026-10-12

- Batch wakes no longer report sent machines as failed when workers record
  attempts at the same instant; stored IDs gain a random suffix.
- Imports reject invalid MAC, IPv4 and port values and references to machines
  or relays missing from the file, instead of storing dangling entries.
- Sunshine pairing verifies the certificate or a pinned `--tls-fingerprint`
  before sending credentials, and usage output no longer prints `SUNSHINE_PASS`.
- Updates can require an Ed25519-signed checksum manifest via
  `WOL_UPDATE_PUBLIC_KEY`; archives have size and entry limits before extraction.
- Prerelease versions compare numerically (`beta.10` is newer than `beta.2`);
  the latest-release redirect fallback uses the configured HTTP client.
- Replaced executables are flushed to disk before the atomic rename.

# 0.4.13 — 2026-10-11

- `wol doctor` warns on macOS when the executable lacks the embedded Local
  Network usage description (for example after `go install` or a
  source-build update), which otherwise makes status checks report unknown.
- Updates verify release archive SHA256 checksums before extracting/installing;
  missing or invalid checksums preserve the installed executable.
- Automatic source fallback stops on macOS and explains packaging-aware install.
- Restore the omitted Apache-2.0 section 5 text and enable private security reports.
- Require Go 1.26.9 and prefer Go 1.27.2 for patched release builds.

# 0.4.12 — 2026-09-21

- Sites with IPv4 subnet, direct/relay wake defaults, probe timeout and concurrency.
- Inventory remains visible while checks stream in; search and navigation stay available.
- Multi-selection, reviewed check/wake batches, cancellation and failed-target retry.
- Discovery interface filtering and reviewed forms for multiple selected hosts.
- TUI import/export, site/relay selectors and Ctrl+S form save.
- Batch CLI with JSON Lines, explicit wake confirmation and documented exit codes.
- Real subnet broadcast calculation, conservative ARP status and atomic IP/profile sync.
- Consistent native make install and local verification/package scripts.

Runtime validation on every packaged operating system and independent first-user
acceptance are required before a public beta announcement.

