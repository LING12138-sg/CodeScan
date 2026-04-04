# CodeScan

CodeScan is a Go + Vue application for source code security review, staged audit workflows, and HTML report export.

## Local Runtime Config

- Keep your real runtime config in `data/config.json`.
- `data/config.json` is local-only and must never be published.
- Safe example values live in `data/config.example.json`.
- The backend also supports environment variable overrides such as `CODESCAN_AUTH_KEY`, `CODESCAN_DB_PASSWORD`, and `CODESCAN_AI_API_KEY`.

## Run Locally

Initialize local directories and config:

```bash
go run ./cmd/init
```

Start the backend:

```bash
go run .
```

Start the frontend from `frontend/README.md`.

## Open Source Release Safety

Before pushing code or creating a release archive, run the built-in release guard:

```bash
go run ./cmd/release check
```

This scans only publishable files. It intentionally excludes local-only content such as:

- `data/config.json`
- `data/tasks.json`
- `projects/`
- `frontend/node_modules/`
- `frontend/dist/`
- `frontend/.cache/`
- `bin/`
- existing `*.zip` and `*.exe` artifacts

To create a clean release archive:

```bash
go run ./cmd/release export -out release/CodeScan-open-source.zip
```

The export command validates the generated ZIP and refuses to ship secrets or excluded files.
