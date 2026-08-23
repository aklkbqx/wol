# Security policy

WOL sends network packets that can power on machines. Treat access to the CLI,
its SQLite inventory, and configured SSH relay keys as administrative access.

## Safe operation

- The program opens no listening socket and starts no background service.
- Keep the inventory database and exports outside source control with
  user-only filesystem permissions.
- Review broadcast destinations and SSH relay hosts before saving them.
- Relay commands validate MAC addresses and SSH arguments and use batch mode.
- A sent packet is not proof that a machine booted; use `wol status` or
  `wol wake --verify` when reachability matters.
- Do not run WOL with elevated privileges unless your operating system requires
  them for the selected network operation.

## Reporting a vulnerability

Do not open a public issue for an undisclosed vulnerability. Contact the
maintainer privately with a reproduction, affected version, and impact.
