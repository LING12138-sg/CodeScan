# CodeScan Frontend

This frontend is the operator workbench for CodeScan. It covers project upload, route inventory review, stage-by-stage security audits, per-stage gap checking, finding revalidation, and report export.

## Prerequisites

- Node.js 20+
- The backend API running on the same host or behind the same reverse proxy

## Development

Install dependencies:

```bash
npm install
```

Start the dev server:

```bash
npm run dev
```

Build the production bundle:

```bash
npm run build
```

## Audit Workflow

1. Upload or select a project from the dashboard.
2. Run the initial route inventory scan.
3. Open any audit stage and run the stage scan manually.
4. Use `Gap Check` to rescan the current stage for missed findings.
5. Use `Revalidate Findings` to confirm, downgrade, or reject current findings.
6. Export the consolidated HTML report after the needed stages are complete.

## Notes

- Gap check and revalidation overwrite the current stage result instead of storing a full run history.
- Revalidation is static-only and updates verification status and reviewed severity per finding.
- The backend initializer no longer injects a hardcoded database password. Set `CODESCAN_DB_PASSWORD` before running `go run ./cmd/init`, or populate your local-only `data/config.json` directly.
- Do not publish `data/config.json`. The open-source-safe template is `data/config.example.json`.
