# Wake-on-LAN

Go daemon and CLI for magic-packet wake, broadcast, and device state.

This file is the always-on contract. `CLAUDE.md`, `GROK.md`, `GEMINI.md` symlink here. Do not fork copies.

## Surfaces

| Path | Role |
|------|------|
| `cmd/` | Entrypoints |
| `internal/` | Broadcast, SQLite, discovery |
| `deploy/` | Docker / Kubernetes |
| `Makefile` | Build and test |

Current source beats README and comments.

## Hard rules

- Inspect `git status --short`. Do not revert unrelated files. Never use or scaffold GitHub Workflows, `.github/workflows/`, or Vercel/Netlify configs (GitHub Workflows are strictly forbidden; developer does not use them).
- Do not print or commit `.env`, keys, or tokens. Do not start the daemon unless asked.
- Locate with graft, then Serena symbols — do not grep or read whole files first.
- Magic-packet send must tolerate missing/unreachable interfaces. Never crash the daemon on a bad subnet.
- SQLite device-state updates are concurrency-safe.
- SIGINT/SIGTERM stops listeners cleanly.

## Verify

```bash
make build
go test -v -race ./...
go vet ./...
```

## Read next

- `README.md` — protocol and schema
- `go.mod` / `Makefile`

<!-- graft:start -->
## Graft

This repo is indexed in `graft/`. Use the graft skill (or `graft ask` / `graft grep` / `graft skeleton` / `graft callers` / `graft map`) before grepping or reading whole files. `graft ask "<q>" --source` returns the `file:line` spans to edit. New here: `graft map`. After large edits: `graft build`.
<!-- graft:end -->
