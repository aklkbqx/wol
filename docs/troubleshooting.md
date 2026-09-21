# Troubleshooting

| Symptom | Check |
| --- | --- |
| Packet sent, machine stays off | Firmware/NIC WOL, wired connection, sleep/shutdown support, correct MAC and broadcast |
| Unknown with cached ARP | Cache is not live proof. Check firewall, macOS Local Network permission and routing |
| No neighbors discovered | Connect to the target LAN; discovery reads complete ARP entries rather than sweeping every address |
| Other LAN unreachable | Establish your VPN/routing for probes and remote; configure a relay for wake broadcasts |
| Relay port reachable but wake fails | Verify SSH batch-mode authentication, etherwake availability and router interface |
| Native remote fails | Install a handler for the protocol and test the target service port; client errors are surfaced |
| Browser remote fails | Run remote doctor; only this optional flow needs Docker |
| Narrow or monochrome terminal | Try NO_COLOR=1, WOL_TUI_ASCII=1 and WOL_TUI_REDUCED_MOTION=1 |
| Permission denied during install | Choose a writable BINDIR, for example make install BINDIR="$HOME/.local/bin" |

On macOS, a locally built binary includes its Local Network usage description
and an ad-hoc signature. This does not grant network access or provide Apple
notarization. Review Local Network settings for the terminal/application that
launched WOL. A denial can appear as "no route to host"; the message alone
cannot distinguish permissions from missing routing.

For bug reports include version, OS/architecture, installation method,
terminal size and a minimal reproduction using invented machine data.
Review and redact hostnames, IPs, MACs, usernames and paths from any output.
Never attach your database, private keys, tokens or an unredacted export.

