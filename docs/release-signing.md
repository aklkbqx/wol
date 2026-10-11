# Release manifest signatures

Development builds support optional Ed25519 verification. No production trust
key has been chosen and v0.4.13 is unsigned. Setting the public-key option will
therefore refuse installation of that release. Ordinary updates still verify
checksums when this option is unset.

## Maintainer workflow

Use an existing Ed25519 private key in unencrypted PKCS8 PEM (`PRIVATE KEY`) and
its corresponding PKIX PEM public key (`PUBLIC KEY`). Keep the private key
outside this checkout and outside release archives. This tool does not create
keys or establish a trusted publisher identity.

Build the normal versioned archives and `SHA256SUMS` first, then sign the final
manifest bytes locally:

```sh
go run ./cmd/wol-release-sign sign \
  --key /secure/location/wol-release-private.pem \
  --manifest dist/release/SHA256SUMS \
  --output dist/release/SHA256SUMS.sig

go run ./cmd/wol-release-sign verify \
  --key /trusted/location/wol-release-public.pem \
  --manifest dist/release/SHA256SUMS \
  --signature dist/release/SHA256SUMS.sig
```

Adjust the manifest path to the prepared release directory. The signature is
64 raw bytes, without base64 encoding. Signing creates a new file with mode
0600 on Unix and refuses to overwrite any existing path. On Windows, protect
key files with filesystem ACLs. Input files are bounded and must be regular
files. No key contents are printed. Verify archive checksums as well as the
signature before publishing the archives, manifest and signature together.
Changing whitespace, line endings or any other manifest byte requires signing
again. A detached signature authenticates the checksum manifest; each archive
must still match its checksum.

## Consumer trust

Obtain the public key through a separately authenticated channel and confirm
its identity with the maintainer before trusting it. A key downloaded beside
the archives is insufficient to establish trust. Use a key dedicated to WOL.
The current tooling does not choose a key, distribute a trust root or automate
key rotation.

```sh
WOL_UPDATE_PUBLIC_KEY=/trusted/location/wol-release-public.pem wol update
```

When configured, the updater requires `<checksum-manifest-name>.sig`, verifies
the exact manifest bytes before parsing checksums or downloading an archive,
and refuses missing, malformed or invalid signatures. It accepts the canonical
`SHA256SUMS` manifest or the exact legacy `wol_VERSION_checksums.txt` filename.
Archive names must begin with `wol_VERSION_` for the release's version, which
prevents an older signed manifest from being relabelled as a newer release.
Source-build fallback is disabled in this mode. Failures leave the installed
executable unchanged. `wol update --check` only checks release metadata; it
does not verify signatures or install anything.

Without this option, checksums protect against corruption but do not provide
publisher authentication. Even with signatures, GitHub's release metadata is
not signed: this is not an independent freshness service and cannot prevent a
server from hiding a newer release. Explicitly requesting an older signed tag
still permits a downgrade. The signature does not attest to the build process.

For rotation, authenticate the new key through the same independent channel,
review its fingerprint, and explicitly replace the locally trusted file.
Do not silently accept a replacement key provided by update metadata. If a key
is compromised, stop trusting it and authenticate a replacement before further
updates. Releases signed with another key will fail until trust is changed.

## Resource limits

Release archives are limited to 128 MiB downloaded bytes; checksum manifests
to 1 MiB; signatures to 64 bytes; PEM keys to 16 KiB. Extraction limits each
binary to 128 MiB, tar expansion (including skipped data and metadata) to
256 MiB, and archive entries to 4096. ZIP entries' declared uncompressed sizes
must total at most 256 MiB and binary reads remain bounded even if metadata is
incorrect. These limits apply before replacing the executable.
