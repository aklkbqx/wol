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

