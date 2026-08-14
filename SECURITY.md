# Security policy

The WOL server performs a network action that can power on a device. Treat it as an administrative control plane.

## Safe defaults

- The server binds to `127.0.0.1` by default.
- The MVP has no multi-user authentication.
- Do not expose the server directly to the public Internet.
- Review and restrict relay destinations before adding remote relays.

## Reporting a vulnerability

Please do not open a public issue for an undisclosed vulnerability. Contact the maintainer privately with a reproduction, affected version and impact assessment.
