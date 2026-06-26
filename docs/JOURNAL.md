# Session journal

> Append a short entry at the end of every working session so the next session resumes cleanly.
> Newest at top. Format: date — what was done / what's next / blockers.

## 2026-06-26 — M0 Scaffold (DONE)
- **Done:** Go 1.26 module scaffold. `internal/config` (godotenv `.env` load + required-var validation, aggregates all missing). `internal/web` stdlib `ServeMux` server: `GET /healthz`→`ok`, `GET /{$}`→embedded `index.html`. Root `assets` package embeds `web/templates` + `web/static` (`all:` prefix needed for `.gitkeep`). Makefile (run/test/test-integration/build/docker), `.env.example`. Verified: `go build`/`vet`/`test` green; server boots + serves both routes (200), non-root 404s, missing creds fail fast. Updated CLAUDE.md build section, PLAN.md (M0 DONE).
- **Next:** **M1** — `internal/kopia` RepoManager: `ListNamespaces` (S3 blob list under `KOPIA_PREFIX`), `Open(ns)`, `ListSnapshots`; `GET /` + `GET /repo/{ns}` pages; integration test vs garage.
- **Blockers:** `make` installed via winget but not yet on PATH in active shell (needs refresh); raw `go` commands work meanwhile.

## 2026-06-26 — Planning & docs setup
- **Done:** Decided stack (Go + kopia lib + htmx). Verified live connection to Garage S3 (`paperless` repo): endpoint/region/prefix/password all confirmed working, snapshot JSON shape captured. Wrote CLAUDE.md and `docs/` (PLAN, ARCHITECTURE, KOPIA, DECISIONS, JOURNAL). Go installed via winget (needs new shell to appear on PATH).
- **Next:** Start **M0** (scaffold). Confirm `go` is on PATH first.
- **Blockers:** none.
