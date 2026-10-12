# Release manifest signatures

Releases from v0.4.16 ship `SHA256SUMS.sig`, a detached Ed25519 signature over
the exact bytes of `SHA256SUMS`. Releases through v0.4.15 are unsigned, so
setting the public-key option refuses to install them. Ordinary updates still
verify checksums when this option is unset.

## Release key

| | |
| --- | --- |
| Algorithm | Ed25519 (PKIX PEM public key, PKCS8 PEM private key) |
| Public key | [`docs/wol-release-public.pem`](wol-release-public.pem) |
| Fingerprint | `ac3979581b0ada7f53a4309a2cfefa983283f62cf914f87b5aa0e8619607f1be` |
| Valid from | v0.4.16 |

The fingerprint is the SHA-256 of the public key's DER encoding. Check it with
either command:

```sh
go run ./cmd/wol-release-sign fingerprint --key docs/wol-release-public.pem
openssl pkey -pubin -in docs/wol-release-public.pem -outform DER | shasum -a 256
```

The fingerprint is also printed in this file, the README and every signed
release's notes. Compare at least two of these, for example a tagged copy of
this file and the release notes, before you trust the key. The private key is
dedicated to WOL, lives only on the maintainer's machine outside the checkout,
with an offline backup, and never in this repository, release archives or
issues.

### Rotation and compromise

A planned rotation lands in a release signed by the current key: that release
adds the new public key, changes the fingerprint here and in the README, and
says so in its notes. The next release is signed by the new key only, so
`WOL_UPDATE_PUBLIC_KEY` users must review and replace their trusted file once.
If the key is lost or exposed, the maintainer stops signing with it, opens an
issue naming the affected fingerprint and publishes the replacement through the
same channels. Never accept a replacement key that only appears in update
metadata or beside the archives.

## Maintainer workflow

`sh scripts/release-check.sh` signs and verifies the manifest automatically. It
reads the private key from `WOL_RELEASE_SIGNING_KEY` (default
`~/.config/wol-release/wol-release-private.pem`) and checks the signature
against `docs/wol-release-public.pem`. `WOL_RELEASE_UNSIGNED=1` skips signing for
a dry run; never publish its output.

The manual commands below take an Ed25519 private key in unencrypted PKCS8 PEM
(`PRIVATE KEY`) and its PKIX PEM public key (`PUBLIC KEY`), for example from
`openssl genpkey -algorithm ed25519`. Keep the private key outside this
checkout and outside release archives.

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

Obtain the public key from this repository, not from the release assets, and
check its fingerprint against a second source as described above. A key
downloaded beside the archives is insufficient to establish trust. WOL does not
distribute a trust root or rotate keys automatically.

```sh
WOL_UPDATE_PUBLIC_KEY=/trusted/location/wol-release-public.pem wol update
```

When configured, the updater requires `<checksum-manifest-name>.sig`, verifies
the exact manifest bytes before parsing checksums or downloading an archive,
and refuses missing, malformed or invalid signatures. It accepts the canonical
`SHA256SUMS` manifest or the exact legacy `wol_VERSION_checksums.txt` filename.
Archive names must begin with `wol_VERSION_` for the release's version, which
prevents an older signed manifest from being relabelled as a newer release.
An explicitly requested version must also match the release metadata returned
by the server; a differently tagged signed release is refused.
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
incorrect. ZIP central-directory records are counted before the standard
parser allocates entries; declared record counts are not trusted. These limits
apply before replacing the executable.
