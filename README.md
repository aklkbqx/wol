# WOL Wake Desk: Wake-on-LAN CLI and terminal UI

[![Release](https://img.shields.io/github/v/release/aklkbqx/wol)](https://github.com/aklkbqx/wol/releases/latest)
[![License](https://img.shields.io/github/license/aklkbqx/wol)](LICENSE)
[![Go](https://img.shields.io/github/go-mod/go-version/aklkbqx/wol)](go.mod)

WOL wakes computers on your network with Wake-on-LAN magic packets, checks
whether they came online, and opens a remote session when they do. It is a
single executable for macOS, Linux and Windows with an interactive terminal UI
(Wake Desk) and a scriptable CLI. The machine inventory is a local SQLite file;
wake and inventory features need no web application, account or hosted service.

- Wake one machine, a site or a selection. Batch wakes run concurrently with
  per-site limits, cancellation and retry of unsuccessful targets.
- Wake through a router relay over SSH (`etherwake` on OpenWrt) when the target
  sits behind another router or at another site.
- Status checks combine TCP, ping and ARP, so machines that block ping still
  report the right state.
- Wake & Remote: wake if needed, then open RDP, SSH, VNC or Moonlight for a
  Sunshine host.
- LAN discovery, inventory import/export, JSON Lines output for scripts and a
  checksum-verified `wol update`.

```text
 wol  checked 05:27:04

 3 machines  1 online  1 unreachable  1 unknown

 › windows     online    stream
   private     unknown   stream
   private2    unreachable    setup
 192.168.50.200  00:11:22:33:44:55  LAN 192.168.50.255:9


 enter choose   w wake   c stream   s check   x stop   ? help   q quit
```

Power and wake readiness are intentionally separate: `UNREACHABLE + CONFIGURED` is the
normal state for a machine that can be woken.

## Install

Download the archive for your operating system and architecture from
[Releases](https://github.com/aklkbqx/wol/releases), verify its SHA256SUMS when
provided, extract it, and place the executable on PATH. See the release notes
for the platforms actually verified for that version.

To build the development version, install Go 1.26.9 or newer, clone this repo,
and run:

```sh
make verify
make install BINDIR="$HOME/.local/bin"
```

Add that directory to PATH if needed. `make install` uses the same native build
and macOS packaging as `make build`. A plain `go install` does not perform those
macOS packaging steps. Wake and inventory need no Docker or background service;
optional browser remote sessions use Docker.

See [Homelab quick start](docs/homelab.md),
[Troubleshooting](docs/troubleshooting.md), and
[Development and release](docs/development.md).

## Use

Run `wol` in a terminal to open Wake Desk. The first run creates a private
local inventory automatically. Press `a` to add a machine or route.

```bash
wol                         # interactive Wake Desk
wol wake --device windows   # wake a stored machine
wol remote windows          # wake if needed, then open the native client (RDP/SSH/VNC) or Moonlight
wol remote --browser windows  # use the localhost Guacamole session instead
wol remote --no-wake windows  # require it online; never send a wake packet
wol scan --add              # import new LAN neighbors into inventory
wol remote configure --protocol rdp --host 192.168.50.200 --certificate strict windows
wol remote clear windows    # remove the machine's remote profile
wol remote doctor windows   # check the runtime and this target
wol remote setup            # pull the pinned local remote containers
wol status windows          # check its current power state
wol wake AA:BB:CC:DD:EE:FF  # send directly to a MAC address
wol export --output inventory.json
wol import inventory.json
wol update
wol update --check
wol version
```

`wol update` verifies the downloaded archive against the release's `SHA256SUMS`
(or its exact versioned legacy checksums file) before installing. Missing,
malformed or mismatched checksums leave the existing executable unchanged.
Older releases without a checksums file require manual installation after
verification. These checksums detect corruption. Releases from v0.4.16 also
ship `SHA256SUMS.sig`, an Ed25519 signature over the manifest. To require it,
save [the release public key](docs/wol-release-public.pem), check that its
fingerprint is `ac3979581b0ada7f53a4309a2cfefa983283f62cf914f87b5aa0e8619607f1be`
and run updates with `WOL_UPDATE_PUBLIC_KEY=/path/to/wol-release-public.pem`;
see [release signing and trust setup](docs/release-signing.md). Releases through
v0.4.15 are unsigned, so this option cannot install them.
On macOS, a missing archive stops the update with instructions to use a release
archive or `make install`, preserving the network-permission packaging.

`wol wake --device NAME --verify` waits for the configured TCP service after
sending. A successful packet send means the packet was delivered; it does not
claim the machine has finished booting.

## Local remote sessions

Remote access is an optional local companion to Wake-on-LAN, not an external
website. A remote profile stores only the connection protocol, host, ports,
mode, certificate policy, and optional username/domain hints. It never stores a
hosted remote URL or a password.

Configure a machine for RDP, VNC, or SSH:

```bash
wol remote configure --protocol rdp --host 192.168.50.200 --port 3389 --certificate trust-local windows
wol remote configure --protocol ssh --host 192.168.50.5 --port 22 private
wol remote doctor windows
wol remote setup
```

The full profile command is:

```text
wol remote configure [--db path] [--protocol rdp|vnc|ssh] [--host HOST] [--port N] [--verify-port N] [--username USER] [--domain DOMAIN] [--certificate strict|trust-local] <machine>
```

`wol remote NAME` checks the machine, wakes it when necessary, and opens the
native RDP, SSH, or VNC client registered on this computer. Sunshine profiles
launch Moonlight. Install a client that handles the selected protocol first;
native clients manage their own credentials and certificate settings.

`wol remote pair NAME` pairs with Moonlight when it is installed. Without
Moonlight, `--pin` pairs through the Sunshine admin API using `SUNSHINE_USER`
and `SUNSHINE_PASS`, or a password prompt. Sunshine normally serves a
self-signed certificate, so WOL refuses it and prints its SHA-256 without
sending credentials. Compare that value on the Sunshine machine (for example
`openssl x509 -in credentials/cacert.pem -noout -fingerprint -sha256` in
Sunshine's config directory), then rerun with `--tls-fingerprint VALUE` or
`SUNSHINE_TLS_FINGERPRINT`. Avoid `--pass`: arguments appear in process lists.

`wol remote --browser NAME` opens RDP, SSH, or VNC through an ephemeral browser
session on `127.0.0.1` using pinned Apache Guacamole
containers, and opens a one-time localhost sign-in. Credentials are accepted
by that loopback session only, encrypted into a short-lived launch token, and
never written to SQLite, exports, command arguments, or logs. The listener is
not exposed to the LAN or internet and stops with the CLI session. Docker is required only
for this browser-based remote flow. Run `wol remote doctor` for a read-only
check; `wol remote doctor NAME` also checks the selected profile and service.
`wol remote setup` downloads the required pinned images. For browser sessions, the default RDP
certificate policy is `strict`; use `trust-local` only for a private machine
whose self-signed RDP certificate you explicitly trust.

Use `wol remote --no-wake NAME` when the target must already be online and the
command must not send a wake packet.

Use `wol remote clear NAME` to delete a profile without changing the machine or
wake configuration. WOL never sends this remote flow to a hosted endpoint.

## Wake Desk controls

- `j/k` or arrows: move
- `Enter`: open the action picker; it never starts an action immediately
- `w`: wake only
- `c`: wake if needed, then open the native remote client or Moonlight
- `s`: check selected machine
- `n`: discover LAN neighbors and add new hosts
- `f`: force wake a disabled machine
- `a/e/d`: add, edit, delete
- `1/2/3/4`: machines, routes, activity, sites
- `[` / `]`: cycle site filter; `v`: cycle status filter
- `Space`: select machines; `S` / `W`: review batch check / wake
- `R`: retry unsuccessful batch targets
- `i` / `E`: import / export inventory; `Ctrl+S`: save a form
- `/`: filter
- `?`: help
- `q`: quit

The action picker always presents **Wake only** and **Wake & Remote** as
separate choices. The latter is disabled until the selected machine has a
valid remote profile. The interface adapts to narrow, compact, and wide
terminals. Inventory appears as soon as it is read. Startup and refresh checks update
machines incrementally while navigation remains available. A failed inventory
refresh preserves the previous data. `Esc` cancels a check or batch; obsolete
results are ignored. Motion is
limited to active work such as verification, wake, or local remote startup.
Use `WOL_TUI_REDUCED_MOTION=1` or `WOL_TUI_MOTION=off` to disable it,
`WOL_TUI_ASCII=1` for ASCII glyphs, and `NO_COLOR=1` to disable color.

## Inventory and privacy

Inventory is stored locally in SQLite. Wake Desk never displays its filesystem
path. Set `WOL_DATA_DIR` to choose a data directory or `WOL_DB` to select an
explicit database for automation.

Portable exports contain sites, machines, remote connection profiles, groups,
and wake relay routes. They exclude passwords, session tokens, wake history,
and any unrelated tables that may exist in an older database. Export files are
created with owner-only permissions.

## Router relays

Wake Desk can call `etherwake` through a non-interactive SSH connection. Add a
route from the Routes view, then assign that route to a machine. SSH targets,
users, ports, interfaces, and MAC addresses are validated before execution.

## Development

```bash
make test
make check
make build
```

New to WOL? The [first-user test](docs/first-user-test.md) takes about 30
minutes and is the most useful help right now.

The public project is deliberately CLI-first. Its optional browser remote is a
temporary localhost runtime; private hosted web, authentication, deployment,
and infrastructure code do not belong in this repository.

Licensed under Apache-2.0. Credit: aklkbqx.
