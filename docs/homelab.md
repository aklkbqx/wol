# Homelab quick start

WOL is a local CLI/TUI for 5–100 machines. Inventory is local SQLite; no account
or background service is required. Network access is still subject to your OS,
router and firewall. Wake capability, live reachability, and remote access are
separate properties.

## First machine

1. Install the binary for your OS and architecture from a release, verify its
   SHA256SUMS, then put it on PATH. Development builds can use `make install`.
2. Run `wol`. Press `n` to discover neighbors, `a` to add manually, or `i`
   to import an inventory file.
3. In discovery, `f` cycles interfaces. Space selects new hosts; Enter reviews
   each selected host in a form. Check MAC/IP and the suggested service port.
4. Tab moves between fields, Enter advances, Ctrl+S saves, Esc cancels.
   Ctrl+N cycles existing site/relay names in those fields.
5. Use `s` to check a machine, Enter for actions, and `p` to configure remote.
   A discovered port is a hint, not proof of the machine's operating system.

Enable Wake-on-LAN in the target's firmware and network adapter settings.
Wake from shutdown depends on the hardware, power settings and network path.
A successful packet send does not establish that a machine booted.

## Multiple LANs

Open Sites with `4`. Add a name and IPv4 subnet. Leave broadcast blank to
calculate it from that subnet. Set a local interface for direct wake, or select
an existing SSH relay by name. The relay's interface belongs to the router,
not the computer running WOL.

Open Routes with `2` to create a relay first. WOL uses non-interactive SSH and
etherwake on the relay. Set up and test SSH access separately; WOL does not
copy keys to routers. Sites `s` checks route configuration and the relay TCP
port, not SSH authentication or successful packet delivery.

Assign a machine to a site in its edit form. Empty machine broadcast/interface
and zero port inherit site defaults. An explicit machine relay overrides the
site relay. Site deletion requires reassigning its machines first.

The site setting does not establish VPN connections or IP routes. Live status
and remote clients need direct routed access (for example a VPN configured by
you). Discovery reads neighbors on connected LANs; use manual entry or import
for another LAN. Ambiguous MAC/IP mappings and relay sites are not auto-synced.

## Daily controls

- `1/2/3/4`: machines / routes / activity / sites.
- `/`: search machine name, IP, MAC or site; `[` or `]`: cycle site filter.
- `v`: cycle reachability filter; Space: select a machine across filters.
- `S`: review/check selected machines; `W`: review/wake selected machines.
- Enter confirms a batch; Esc cancels it. Completed results are retained.
- `R`: review retries for unsuccessful targets from the last batch.
- `E`: export to a new private JSON file; `i`: import/merge an inventory.
- `?`: help; `q`: quit. Esc also clears filters and machine selection.

Inventory remains usable during background checks. Per-site worker limits and
timeouts bound work; at most 16 network operations run at once in the TUI.
Unknown is not offline. An ARP cache entry alone does not prove a machine is
currently reachable. Each result includes its method and check time.

## Automation

```sh
wol status --json --no-input my-pc
wol batch check --site Lab --json --no-input
wol batch check --all --concurrency 8
wol batch wake --site Lab
wol batch wake --site Lab --yes --json --no-input
```

Flags precede positional machine names. Batch selection is exactly one of
`--all`, `--site NAME`, or a list of machine names/IDs. Wake without `--yes`
prints a preview and exits without sending packets. JSON batch output is one
object per line; diagnostics go to stderr. No interactive prompt is used.

Batch exit codes: 0 all successful; 1 none successful/runtime error; 2 invalid
input or confirmation required; 3 partial success; 130 interrupted. For checks,
only a live online result is successful. For wake, success means packet sent.

## Backup, upgrade and uninstall

Use `wol export --output inventory.json` before upgrading. Imports merge by
identity; use a fresh `--db` path to inspect a backup before merging. Exports
include connection settings but exclude passwords and transient session tokens.
Keep them private. Portable exports do not preserve activity history.

Stop WOL before making a filesystem backup of its data directory. Keep the
SQLite database and any WAL/SHM files together. Newer inventory schema versions
may not open in an older binary: retain the old data backup when rolling back.

Replace the binary to upgrade, then run `wol version` and `wol doctor`.
To uninstall, remove the installed executable. Keep or deliberately remove the
data directory separately; uninstalling the binary should not erase inventory.

