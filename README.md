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

## Configuration

Keep non-secret defaults in `config.yaml`. Put credentials in environment variables or a local `.env` file that is not committed:

```bash
cp .env.example .env
```

`make dev` and `make test` load `.env` automatically when it exists.

AI settings:

```bash
REELWARDEN_AI_ENABLED=false
REELWARDEN_AI_BASE_URL=http://localhost:11434/v1
REELWARDEN_AI_API_KEY=
REELWARDEN_AI_MODEL=qwen3
```

TMDB settings:

```bash
REELWARDEN_TMDB_ENABLED=false
REELWARDEN_TMDB_TOKEN=
REELWARDEN_TMDB_API_KEY=
REELWARDEN_TMDB_LANGUAGE=zh-CN
REELWARDEN_TMDB_REGION=CN
```

The runtime config endpoint returns only redacted configuration status. It never returns API keys or tokens.

TMDB and AI remain mutually blocked unless `COMPLIANCE-TMDB-AI` is accepted by the project authority process.

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
