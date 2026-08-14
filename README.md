# Wake-on-LAN Toolkit

WOL is a self-hosted Wake-on-LAN (WOL) toolkit for waking and organizing computers across a local network. It combines a Go server and CLI, local SQLite storage, and a responsive SvelteKit web dashboard.

It is designed for home labs, small offices, device fleets, and network automation workflows that need a simple, private Wake-on-LAN control plane.

## Features

- Send UDP magic packets from a CLI or web dashboard
- Manage sites, devices, groups, MAC addresses, broadcast addresses, ports, and interfaces
- Store configuration and wake history locally in SQLite
- Verify optional TCP reachability after a wake attempt
- Stream new wake attempts to the dashboard with Server-Sent Events
- Import and export device inventory as JSON
- Run on macOS, Linux, and Windows with Go

WOL keeps packet delivery and reachability separate: a successful packet send does not claim that a device powered on.

## Quick start

Requirements: Go 1.26+ and Node.js 20+.

Build the SvelteKit dashboard and Go binary:

```bash
make build
```

Start the local server:

```bash
./dist/wol server --db ./wol.db --web-dir ./web/build
```

Open <http://127.0.0.1:8787>. The server binds to loopback by default.

## CLI

Send a Wake-on-LAN packet directly:

```bash
go run ./cmd/wol wake \
  --destination 192.168.1.255 \
  AA:BB:CC:DD:EE:FF
```

Use `--port`, `--repeat`, `--interval`, and `--interface` to tune delivery for your network.

## Web development

Run the Go API:

```bash
go run ./cmd/wol server --db ./wol.db
```

In another terminal, run the SvelteKit development server:

```bash
cd web
npm install
npm run dev
```

The Vite server proxies `/api` and `/healthz` to `127.0.0.1:8787`.

## Data and security

The database path is configurable with `--db`. The dashboard can export and import sites, devices, and groups as JSON; wake history remains local to the installation.

The current MVP has no multi-user authentication. Do not expose the server to an untrusted network. Read [SECURITY.md](SECURITY.md) before binding it outside loopback.

## Development

```bash
make test
make check
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution guidelines.

## License

Apache-2.0. See [LICENSE](LICENSE).
