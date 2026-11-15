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
   ● wol  all sites                 Machines   Routes   Activity   Sites           not checked yet


   MACHINES                  2
                                   ▄████▄   █▀▄ ▄▀▄ ▄▀▀
   ● studio                        ██████   █ █ █▀█  ▀▄
                                   ▀████▀   ▀ ▀ ▀ ▀ ▀▀
   ○ nas
                                   asleep   no answer on the last check


                                   address                        hardware
                                   198.51.100.20                  02:00:00:00:00:02
                                   wakes by                       checks
                                   broadcast 198.51.100.255:9     tcp:445


                                     w   Wake nas      c  wake and connect    s  check


   1 awake  1 asleep                ↑↓  move    space  mark    /  filter    ?  all keys    q  quit
```

<sub>Wake Desk Spotlight without color, with sample machines and documentation addresses. In a
truecolor terminal the stage shows a picture of the machine instead of the lamp.</sub>

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

## Step by step

Setup takes three steps and you do it once. After that, waking a machine is one key.

**1. Enable Wake on LAN on the machine you want to wake.** Turn it on in the
BIOS/UEFI power settings (some boards call it "Power On by PCI-E") and connect
the machine by Ethernet. On Windows, also turn on "Wake on Magic Packet" in the
network adapter's settings.

![Enable Wake on LAN in the BIOS and plug in Ethernet](docs/media/step1-enable-wol.gif)

**2. Install `wol` on the computer you wake from**, then run `wol doctor`.

| System | Install |
|--------|---------|
| macOS, Linux | `brew install aklkbqx/wol/wol` |
| macOS, Linux without Homebrew | [curl script](#macos-and-linux-without-homebrew) |
| Windows | [PowerShell script](#windows) |

**3. Add your machines.** Open Wake Desk with `wol`, press `a`, fill in the
name, MAC address and IP address, then press `ctrl+s`. Press `n` instead to
pick machines from your LAN neighbors, or run `wol scan --add`.

![Press a in Wake Desk, type the machine and save it](docs/media/step3-add-machines.gif)

**4. Wake.** Pick a machine and press `w`. WOL sends the magic packet, then
counts down until the machine answers.

![Press w; the countdown runs until the machine answers](docs/media/step4-wake.gif)

**5. Wake & Remote.** Press `c` instead to wake the machine if needed, wait for
it, then open RDP, SSH, VNC or Moonlight. Set the protocol once with `p`.

![Press c to wake the machine and open its stream](docs/media/step5-wake-and-remote.gif)

<sub>Recorded from Wake Desk with sample machines and documentation addresses;
no packets were sent. The key pressed and what it did are shown under the
screen. Wake Desk is keyboard-only.</sub>

## Install

### macOS and Linux with Homebrew

```sh
brew install aklkbqx/wol/wol
wol doctor
```

### macOS and Linux without Homebrew

This downloads the latest release for your system, checks it against
`SHA256SUMS` and installs it to `~/.local/bin`:

```sh
v=$(curl -fsSLI -o /dev/null -w '%{url_effective}' https://github.com/aklkbqx/wol/releases/latest); v=${v##*/v}
os=$(uname -s | tr '[:upper:]' '[:lower:]')
case $(uname -m) in x86_64|amd64) arch=amd64 ;; arm64|aarch64) arch=arm64 ;; esac
f="wol_${v}_${os}_${arch}.tar.gz"
curl -fLO "https://github.com/aklkbqx/wol/releases/download/v$v/$f"
curl -fLO "https://github.com/aklkbqx/wol/releases/download/v$v/SHA256SUMS"
if command -v sha256sum >/dev/null; then sum=sha256sum; else sum='shasum -a 256'; fi
grep " $f\$" SHA256SUMS | $sum -c
tar -xzf "$f" wol && mkdir -p ~/.local/bin && install -m 755 wol ~/.local/bin/wol
~/.local/bin/wol doctor
```

Add `~/.local/bin` to your `PATH` if your shell does not already include it.

### Windows

In PowerShell (64-bit Windows):

```powershell
$v = (Invoke-RestMethod https://api.github.com/repos/aklkbqx/wol/releases/latest).tag_name.TrimStart('v')
$f = "wol_${v}_windows_amd64.zip"
$base = "https://github.com/aklkbqx/wol/releases/download/v$v"
Invoke-WebRequest "$base/$f" -OutFile $f
Invoke-WebRequest "$base/SHA256SUMS" -OutFile SHA256SUMS
$want = ((Select-String -Path SHA256SUMS -Pattern " $f$").Line -split '\s+')[0]
if ((Get-FileHash $f -Algorithm SHA256).Hash -ne $want) { throw 'Checksum mismatch' }
$dir = "$env:LOCALAPPDATA\Programs\wol"
Expand-Archive $f -DestinationPath $dir -Force
[Environment]::SetEnvironmentVariable('Path', [Environment]::GetEnvironmentVariable('Path', 'User') + ";$dir", 'User')
& "$dir\wol.exe" doctor
```

Open a new terminal afterwards so `wol` is on your `PATH`.

### Other options

You can also download the archive for your system from
[Releases](https://github.com/aklkbqx/wol/releases/latest) by hand. Each
release lists the platforms that were actually tested.

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
| `t` | Cycle the color theme (`Night Room`, `Brass Night`, `Catppuccin Mocha`, `Tokyo Night`, `Gruvbox Dark`, `Daylight Paper`) |
| `/`, `?`, `q` | Filter, help, quit |

`Esc` cancels a running check or batch. The Machines tab opens in Spotlight: a
machine list on the left and one large stage for the focused machine, with a
live countdown while a wake waits for the machine to answer. It follows the
window size:

| Window | Layout |
| --- | --- |
| 90 columns or wider | Full list beside the stage; a picture of the machine from 24 rows |
| 72 to 89 columns | A narrower list; the facts stack in one column |
| 50 to 71 columns | The stage alone, naming the machines above and below; `l` shows the list |
| 36 to 71 columns, 30 rows or more, as tall as wide or taller | Portrait: the picture on top, the name in plain bold, the facts, buttons three rows tall, the machines above and below |
| Under 50 columns or 20 rows | The list alone, each machine's state in words; from 90 columns and 16 rows the focused machine shows beside it |

Under 30×10, with `WOL_TUI_ASCII=1` and on the other tabs, the classic layout is
used. On a short window `?` shows the keys in pages; `space` turns the page.

On a phone SSH app, set the line spacing to 1.0 if the picture shows gaps
between rows, or turn pictures off with `WOL_TUI_IMAGES=off`. The portrait
layout avoids block letters, which the same spacing splits apart.

Display options:

| Variable | Effect |
| --- | --- |
| `WOL_TUI_THEME=<id>` | Set startup palette (`night` default, `brass`, `catppuccin`, `tokyonight`, `gruvbox`, `daylight`) |
| `WOL_TUI_REDACT=1` | Hide machine names, IP and MAC addresses on screen |
| `WOL_TUI_REDUCED_MOTION=1` or `WOL_TUI_MOTION=off` | Turn off animation |
| `WOL_TUI_ASCII=1` | Use ASCII glyphs |
| `WOL_TUI_IMAGES=<mode>` | Pictures: `auto` (default), `kitty`, `blocks` or `off` |
| `NO_COLOR=1` | Turn off color |

`WOL_TUI_REDACT` only changes what is shown. Wakes, exports and logs still use
the real values.

## Pictures

In a truecolor terminal the stage shows a picture of the machine in a night
room: the screen is lit and the power light is green when it answers, dark
when it sleeps, and pulses amber while a wake is on its way. A window too
small for the picture shows a status lamp instead.

- **Type** (in the add/edit form, `Ctrl+N` cycles): desktop, laptop, server,
  NAS, mini PC or console. Leave it blank to guess from the name
  (`nas-vault` is drawn as a NAS, `render-node` as a server).
- **Picture file**: a PNG, JPEG or GIF of your own machine, up to 20 MB and
  40 megapixels. `~/` works. It is dimmed while the machine sleeps. When the
  file moves or cannot be read, the stage says so and falls back to the room.

Kitty and Ghostty draw sharp pictures with the kitty graphics protocol, and
add a small icon of each machine type to the list. Other truecolor terminals
(iTerm2, WezTerm, Windows Terminal, VS Code, and tmux in a truecolor
terminal) get half-block pictures. Terminals without truecolor, `NO_COLOR`
and `WOL_TUI_ASCII` get none. `WOL_TUI_REDACT=1` never shows your picture
and never guesses a type from the name. A terminal cannot change its window
icon, so the power logo sits at the top of the machine list instead.

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
