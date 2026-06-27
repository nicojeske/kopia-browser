# CLAUDE.md

Project memory for Claude Code. Read this first every session.

## What this is

`kopia-browser` — a web app to browse [Velero](https://velero.io/)-created [kopia](https://kopia.io/)
backups stored on S3, view individual snapshots, and download files/folders from them.

- **Module:** `github.com/nicojeske/kopia-browser`
- **Stack:** Go backend + kopia Go library, frontend `html/template` + htmx, single binary + Docker image, deploy in k8s.
- **Backups:** Garage S3, bucket `velero-backup`, one kopia repo per namespace at `kopia/<namespace>/`. All repos share one password. See [docs/KOPIA.md](docs/KOPIA.md).

## Documentation map — keep these current

| File | Purpose | When to update |
|------|---------|----------------|
| [docs/PLAN.md](docs/PLAN.md) | Living project plan + milestone status | At start/end of every milestone; tick boxes, set status |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Components, data flow, structure | When structure or a component changes |
| [docs/KOPIA.md](docs/KOPIA.md) | Verified kopia/velero/garage facts & commands | When you learn a new domain fact |
| [docs/DECISIONS.md](docs/DECISIONS.md) | Append-only decision log (ADR-lite) | When a non-trivial decision is made |
| [docs/JOURNAL.md](docs/JOURNAL.md) | Per-session notes: done / next / blockers | End of every working session |

**This is multi-session work.** Each milestone may be a separate session. Before coding: read PLAN.md (current milestone) + JOURNAL.md (last session). After working: update PLAN.md status and append a JOURNAL.md entry so the next session can resume cleanly.

## Build / run / test

Makefile targets (Go 1.26, module `github.com/nicojeske/kopia-browser`):

- `make run` — run locally; `internal/config` loads `.env` via godotenv
- `make test` — unit tests (`go test ./...`)
- `make test-integration` — `go test -tags=integration ./...`; no integration tests yet (added M1)
- `make build` — build binary to `bin/kopia-browser`
- `make docker` — build image (Dockerfile arrives M6)

Equivalent raw commands work too (e.g. `go run ./cmd/kopia-browser`) if `make` is unavailable.

## Hard rules

- **Read-only.** Never write, modify, or delete anything in the kopia repos / S3 bucket. This app only reads.
- **No secrets in git.** S3 creds + repo password come from env vars only. `.env` is gitignored; commit `.env.example` with empty values. Never paste real secret values into any committed file, doc, or commit message.
- **Verify against reality.** kopia internals are subtle — run the binary / integration tests against real garage to confirm behavior, don't just trust compilation.
- **Small commits.** One working increment per commit. Claude may create atomic commits autonomously (no need to ask first) once a change builds + tests pass. Never push unless asked.

## Environment variables

`S3_ENDPOINT`, `S3_REGION` (=`garage`), `S3_BUCKET` (=`velero-backup`), `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `KOPIA_REPO_PASSWORD`, `KOPIA_PREFIX` (=`kopia/`), `LISTEN_ADDR` (=`:8080`), `LOG_LEVEL` (=`info`; one of `debug|info|warn|error`), `STATS_REFRESH_INTERVAL` (=`60m`), `KOPIA_CACHE_DIR` (=`.kopia-cache`).
