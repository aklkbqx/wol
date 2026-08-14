# Contributing

## Development

Run the backend tests and frontend checks before opening a pull request:

```bash
make test
make check
```

For web work, run the Go server and SvelteKit dev server separately. Keep API contracts in `api/openapi.yaml` and keep user-facing copy concise and action-oriented.

## Pull requests

- Include focused tests for behavior changes.
- Keep the CLI, API and web UI contracts aligned.
- Do not add credentials, real MAC inventories or private network details.
- Explain network assumptions in the documentation when changing transport behavior.
