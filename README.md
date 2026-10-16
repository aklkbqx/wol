# WOL Wake Desk

[![Release](https://img.shields.io/github/v/release/aklkbqx/wol)](https://github.com/aklkbqx/wol/releases/latest)
[![License](https://img.shields.io/github/license/aklkbqx/wol)](LICENSE)
[![Go](https://img.shields.io/github/go-mod/go-version/aklkbqx/wol)](go.mod)
[![Website](https://img.shields.io/badge/website-wol.aklkbqx.com-f5a524)](https://wol.aklkbqx.com)

Turn on a sleeping computer from another room, check that it answered, then
open the remote app you already use. WOL is one executable for macOS, Linux
and Windows with a terminal UI (Wake Desk) and a scriptable CLI. Your machine
list is a local SQLite file. There is no account, server or background service.

**Website:** [wol.aklkbqx.com](https://wol.aklkbqx.com) walks through one wake
with a real Wake Desk recording, in English and Thai.

```text
⚡ wol command center  v0.4.18  checked 21:51:54
◆ machines   │     routes   │     activity   │     sites

╭─ ◆ FLEET ────────────────────────────────────────────────────╮ ╭─ ◆ INSPECTOR ─────────────────────────────────────╮
│ 1 machines  0 online  1 unreachable  0 unknown               │ │ INSPECTOR                                         │
│                                                              │ │ Sample desktop                                    │
│ › Sample desktop        unreachable  setup                   │ │ unreachable  setup                                │
│                                                              │ │ direct broadcast 198.51.100.255:9                 │
│                                                              │ │ enter  wake or stream                             │
│                                                              │ │ ────────────────────────                          │
│                                                              │ │ IP   : 198.51.100.2                               │
│                                                              │ │ Seen : unreachable                                │
│                                                              │ │ MAC  : 02:00:00:00:00:01                          │
│                                                              │ │ Route: broadcast 198.51.100.255:9                 │
╰──────────────────────────────────────────────────────────────╯ ╰───────────────────────────────────────────────────╯
Checked 1/1 machines. Unreachable does not prove power is off.
──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
enter choose   space select   / search   ? help   q quit
```

<sub>Wake Desk v0.4.18 with a sample machine and documentation addresses.</sub>

## What it does

- **Wake** one machine, a whole site or a selection. Batch wakes run
  concurrently with per-site limits, cancellation and retry of failed targets.
- **Check** whether a machine is up. Status combines TCP, ping and ARP, so a
  machine that blocks ping still reports correctly.
- **Wake & Remote** wakes the machine if needed, waits for it, then opens RDP,
  SSH, VNC or Moonlight (for a Sunshine host).
- **Router relays** send the magic packet from an OpenWrt router over SSH
  (`etherwake`) when the target sits behind another router or at another site.
- **Shut down** a machine over SSH, now or on a schedule.
- **Inventory tools**: LAN discovery, portable JSON import/export, JSON Lines
  output for scripts, and a signature-checked `wol update`.

A sleeping machine shows `unreachable`. That is the normal state before a wake,
not an error: WOL reports power state and wake readiness separately.

## Install

Download the archive for your system from
[Releases](https://github.com/aklkbqx/wol/releases/latest), verify it against
`SHA256SUMS`, extract it and put `wol` on your `PATH`. Each release lists the
platforms that were actually tested.

Releases from v0.4.16 also ship `SHA256SUMS.sig`, an Ed25519 signature over the
checksums. [Release signing](docs/release-signing.md) explains how to check it.

To build from source you need Go 1.26.9 or newer:

```sh
git clone https://github.com/aklkbqx/wol.git
cd wol
make verify
make install BINDIR="$HOME/.local/bin"
```

Use `make install` rather than `go install` on macOS: it embeds the app bundle
information that macOS needs to grant Local Network access.

## Quick start

```sh
wol                          # open Wake Desk; the first run creates the inventory
wol scan --add               # add machines found on the LAN
wol wake --device desktop    # wake a saved machine
wol status desktop           # check whether it is up
wol remote desktop           # wake if needed, then open its remote client
```

In Wake Desk, press `a` to add a machine and `Enter` on a machine to choose an
action.

## Commands

| Command | What it does |
| --- | --- |
| `wol` / `wol tui` | Open Wake Desk |
| `wol wake` | Wake a saved machine (`--device NAME`) or a MAC address |
| `wol batch check\|wake` | Check or wake `--site NAME`, `--all` or listed machines |
| `wol status NAME` | Check a saved machine's power state |
| `wol scan` | Discover LAN neighbors; `--add` imports new hosts |
| `wol remote NAME` | Wake if needed, then open RDP, SSH, VNC or Moonlight |
| `wol shutdown NAME` | Shut a machine down over SSH, now or with `--delay` |
| `wol import FILE` / `wol export` | Move the inventory as portable JSON |
| `wol doctor` | Check the local wake toolchain |
| `wol update` | Install the latest release after verifying it |
| `wol version` | Print the version |

Useful flags:

- `wol wake --verify` waits for the machine's TCP verification port after
  sending. A sent packet only means it left your computer; `--verify` tells
  you the machine actually came up.
- `wol wake --router HOST` relays the packet through an SSH router.
- `wol batch ... --json` prints one JSON result per line for scripts;
  `--yes --no-input` runs without prompts.
- `wol shutdown cancel NAME` aborts a scheduled shutdown;
  `wol shutdown configure NAME` stores the SSH user, key and platform.

Run any command with `-h` for its full flag list.

## Wake Desk keys

| Key | Action |
| --- | --- |
| `j` / `k`, arrows | Move |
| `Enter` | Open the action picker (never starts an action by itself) |
| `w` | Wake only |
| `c` | Wake if needed, then open the remote client |
| `s` | Check the selected machine |
| `n` | Discover LAN neighbors |
| `f` | Force-wake a disabled machine |
| `a` / `e` / `d` | Add, edit, delete |
| `1`–`4` | Machines, routes, activity, sites |
| `[` / `]`, `v` | Cycle the site filter, cycle the status filter |
| `Space`, `S` / `W` | Select machines, then review a batch check / wake |
| `R` | Retry the failed targets of a batch |
| `i` / `E` | Import / export the inventory |
| `/`, `?`, `q` | Filter, help, quit |

`Esc` cancels a running check or batch. The layout adapts to narrow, compact
and wide terminals.

Display options:

| Variable | Effect |
| --- | --- |
| `WOL_TUI_REDACT=1` | Hide machine names, IP and MAC addresses on screen |
| `WOL_TUI_REDUCED_MOTION=1` or `WOL_TUI_MOTION=off` | Turn off animation |
| `WOL_TUI_ASCII=1` | Use ASCII glyphs |
| `NO_COLOR=1` | Turn off color |

`WOL_TUI_REDACT` only changes what is shown. Wakes, exports and logs still use
the real values.

## Remote sessions

A remote profile stores the protocol, host, ports, certificate policy and an
optional user or domain. It never stores a password.

```sh
wol remote configure --protocol rdp --host 198.51.100.2 --certificate strict desktop
wol remote desktop             # native RDP, SSH or VNC client, or Moonlight
wol remote --no-wake desktop   # require it online; never send a wake packet
wol remote doctor desktop      # check the runtime and this target
wol remote clear desktop       # delete the profile, keep the machine
```

- **Moonlight / Sunshine:** `wol remote pair NAME` pairs through Moonlight. With
  `--pin` it uses the Sunshine admin API instead (`SUNSHINE_USER`,
  `SUNSHINE_PASS` or a prompt). WOL refuses Sunshine's self-signed certificate
  until you confirm its SHA-256 with `--tls-fingerprint` or
  `SUNSHINE_TLS_FINGERPRINT`.
- **Browser session:** `wol remote --browser NAME` opens RDP, SSH or VNC in your
  browser through pinned Apache Guacamole containers bound to `127.0.0.1`. This
  is the only feature that needs Docker; run `wol remote setup` once to pull the
  images. Credentials stay in that short-lived local session and are never
  written to disk, exports or logs.

## Router relays

Add a route in the Routes view (`2`), then assign it to a machine. WOL runs
`etherwake` on the router over a non-interactive SSH connection, after
validating the SSH target, user, port, interface and MAC address.

## Your data

- The inventory is a local SQLite file. Set `WOL_DATA_DIR` to choose its
  directory or `WOL_DB` to point at a specific database.
- Exports contain sites, machines, remote profiles, groups and relay routes.
  They never contain passwords, session tokens or wake history, and they are
  created readable by you only.
- `wol update` installs only a release whose archive matches `SHA256SUMS`. Set
  `WOL_UPDATE_PUBLIC_KEY` to also require a valid signature
  ([details](docs/release-signing.md)).

## Documentation

- [Homelab quick start](docs/homelab.md)
- [Troubleshooting](docs/troubleshooting.md)
- [Release signing and trust](docs/release-signing.md)
- [Development and releases](docs/development.md)
- [How work is tracked on GitHub](docs/github-workflow.md)
- [Changelog](docs/CHANGELOG.md)

## Contributing

The most useful help right now is the
[first-user test](docs/first-user-test.md): about 30 minutes on your own
machines, reporting anything unclear. Bugs and ideas go to
[Issues](https://github.com/aklkbqx/wol/issues); see
[CONTRIBUTING.md](CONTRIBUTING.md) and [SECURITY.md](SECURITY.md).

```sh
make verify    # race tests, vet, native build, whitespace check
```

WOL is deliberately CLI-first. The optional browser remote is a temporary
localhost runtime; hosted services and infrastructure code stay out of this
repository.

Licensed under [Apache-2.0](LICENSE). Made by aklkbqx.
