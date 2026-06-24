# ReelWarden（影卫）

ReelWarden is a deterministic local media organizer with an optional AI assistant.

The current implementation is an early v0.1.1 scaffold focused on the authority baseline:

- local-first Go server
- SQLite using `modernc.org/sqlite` with WAL
- REST `/health` endpoint
- configuration loading from YAML and environment variables
- compliance gate framework for `COMPLIANCE-TMDB-AI`
- React/Vite web shell

## Quick start

```bash
cp config.example.yaml config.yaml
make dev
```

Open <http://127.0.0.1:8787>.

## Safety defaults

- AI is off by default.
- TMDB is off by default.
- The server binds to `127.0.0.1` by default.
- v0.1.1 exposes no real file-write execution API.
- TMDB and AI cannot be enabled together unless the compliance gate is accepted by a non-user-controlled authority path.
