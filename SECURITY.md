# Security policy

WOL sends network packets that can power on machines. Treat access to the CLI,
its SQLite inventory, and configured SSH relay keys as administrative access.

## Safe operation

- Core inventory/wake commands start no persistent service. Optional browser remote sessions open a temporary loopback listener and Docker sidecars; they stop when the session closes.
- Keep the inventory database and exports outside source control with
  user-only filesystem permissions.
- Review broadcast destinations and SSH relay hosts before saving them.
- Relay commands validate MAC addresses and SSH arguments and use batch mode.
- A sent packet is not proof that a machine booted; use `wol status` or
  `wol wake --verify` when reachability matters.
- Do not run WOL with elevated privileges unless your operating system requires
  them for the selected network operation.

## Reporting a vulnerability

Do not open a public issue for an undisclosed vulnerability. Use GitHub private vulnerability reporting when enabled for this repository.
Otherwise request a private contact channel through the maintainer profile at
https://github.com/aklkbqx before sharing technical details. Include the affected
version, a minimal reproduction and impact; omit credentials and real inventories.
