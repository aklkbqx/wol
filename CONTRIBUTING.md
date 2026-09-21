# Contributing

WOL is intentionally a standalone CLI/TUI. Keep changes focused on local
inventory, presence checks, Wake-on-LAN delivery, terminal usability, and
portable packaging.

Before opening a pull request:

```bash
make verify
```

- Add focused tests for behavior changes.
- Preserve narrow-terminal, `NO_COLOR`, ASCII, and reduced-motion behavior.
- Keep optional browser-remote changes inside its localhost companion. Do not add hosted services, deployment automation, credentials,
  real machine inventories, or private network details.
- Keep external commands non-interactive and validate every dynamic argument.

Read [Development](docs/development.md) for architecture, migrations and local release checks, and [Community conduct](CODE_OF_CONDUCT.md). Describe unverified platforms in your PR. No GitHub Actions workflows are used.
