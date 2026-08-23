# Contributing

WOL is intentionally a standalone CLI/TUI. Keep changes focused on local
inventory, presence checks, Wake-on-LAN delivery, terminal usability, and
portable packaging.

Before opening a pull request:

```bash
make test
make check
```

- Add focused tests for behavior changes.
- Preserve narrow-terminal, `NO_COLOR`, ASCII, and reduced-motion behavior.
- Do not add web servers, browser clients, deployment automation, credentials,
  real machine inventories, or private network details.
- Keep external commands non-interactive and validate every dynamic argument.
