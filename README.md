# WOL Wake Desk

WOL is a fast, standalone Wake-on-LAN CLI and terminal interface. It opens your
local machine inventory directly from SQLite and never starts a web server.

```text
WOL WAKE DESK  v0.3.0  ·  Credit: aklkbqx
LOCAL INVENTORY  ·  compact

1 Machines   2 Routes   3 Activity

FLEET
  POWER  1 online · 1 offline · 1 unknown
  WAKE   3 ready · 0 blocked

› windows       POWER ● ONLINE   WAKE ● READY
  private       POWER ? UNKNOWN  WAKE ● READY
  private2      POWER ○ OFFLINE  WAKE ● READY
```

Power and wake readiness are intentionally separate: `OFFLINE + READY` is the
normal state for a machine that can be woken.

## Install

Requires Go 1.26 or newer:

```bash
go install github.com/aklkbqx/wol/cmd/wol@latest
```

Or build and install from a clone:

```bash
make test
make install
```

The executable is installed to `$(go env GOPATH)/bin/wol`. No Node.js,
MongoDB, Docker, or background service is required.

## Use

Run `wol` in a terminal to open Wake Desk. The first run creates a private
local inventory automatically. Press `a` to add a machine or route.

```bash
wol                         # interactive Wake Desk
wol wake --device windows   # wake a stored machine
wol status windows          # check its current power state
wol wake AA:BB:CC:DD:EE:FF  # send directly to a MAC address
wol export --output inventory.json
wol import inventory.json
wol version
```

`wol wake --device NAME --verify` waits for the configured TCP service after
sending. A successful packet send means the packet was delivered; it does not
claim the machine has finished booting.

## Wake Desk controls

- `j/k` or arrows: move
- `Enter`: wake selected machine
- `s`: check selected machine
- `f`: force wake a disabled machine
- `a/e/d`: add, edit, delete
- `1/2/3`: machines, routes, activity
- `/`: filter
- `?`: help
- `q`: quit

The interface adapts to narrow, compact, and wide terminals. Motion appears
only while a wake signal is being sent. Use `WOL_TUI_REDUCED_MOTION=1` or
`WOL_TUI_MOTION=off` to disable it, `WOL_TUI_ASCII=1` for ASCII glyphs, and
`NO_COLOR=1` to disable color.

## Inventory and privacy

Inventory is stored locally in SQLite. Wake Desk never displays its filesystem
path. Set `WOL_DATA_DIR` to choose a data directory or `WOL_DB` to select an
explicit database for automation.

Portable exports contain sites, machines, groups, and wake relay routes. They
exclude wake history and any unrelated tables that may exist in an older
database. Export files are created with owner-only permissions.

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

The public project is deliberately CLI-only. Please do not add browser,
authentication, deployment, or private infrastructure code to this repository.

Licensed under Apache-2.0. Credit: aklkbqx.
