# ReelWarden（影卫）

ReelWarden is a deterministic local media organizer with an optional AI assistant.

This repository now contains a minimal v0.1.1 MVP slice for the read-only movie workflow:

- local-first Go server
- safe defaults: `127.0.0.1`, AI off, TMDB off, no real file writes
- configuration loading from `config.example.yaml` plus environment overrides
- `COMPLIANCE-TMDB-AI` runtime block
- read-only library root registration
- video file scanning for common movie containers
- deterministic filename parsing with CJK-friendly titles and year extraction
- mock metadata candidates with grouped evidence
- manual candidate confirmation
- Jellyfin-style naming preview
- immutable Dry Run Action Plan generation
- React/Vite web console for the full read-only loop

## Quick start

```bash
cp config.example.yaml config.yaml
npm install
make dev
```

Open the web console at <http://127.0.0.1:5173>.

The local API listens on <http://127.0.0.1:8787>.

## Verification

```bash
make test
```

## MVP API walkthrough

```bash
curl -X POST http://127.0.0.1:8787/api/library_roots \
  -H 'content-type: application/json' \
  -d '{"path":"/media/movies"}'

curl -X POST http://127.0.0.1:8787/api/scans \
  -H 'content-type: application/json' \
  -d '{"library_root_id":"root_xxx"}'

curl http://127.0.0.1:8787/api/assets
curl http://127.0.0.1:8787/api/assets/asset_xxx/candidates

curl -X POST http://127.0.0.1:8787/api/assets/asset_xxx/confirm \
  -H 'content-type: application/json' \
  -d '{"candidate_id":"cand_xxx"}'

curl -X POST http://127.0.0.1:8787/api/plans \
  -H 'content-type: application/json' \
  -d '{"asset_id":"asset_xxx"}'
```

The generated plan is dry-run only. It never moves, deletes, overwrites, or writes media files.

## Current limitations

- The persistence layer is still a compile-safe MVP placeholder; the authority baseline still targets SQLite with `modernc.org/sqlite` and WAL for the production path.
- Mock metadata is implemented; Local NFO and TMDB adapters remain next steps before Alpha Core.
- `ffprobe` media probing remains next step.
