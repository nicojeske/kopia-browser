# Session journal

> Append a short entry at the end of every working session so the next session resumes cleanly.
> Newest at top. Format: date — what was done / what's next / blockers.

## 2026-06-26 — M1 List namespaces + snapshots (DONE)
- **Done:** Full 3-layer M1. Data: `internal/kopia` Manager — `ListNamespaces` (minio-go delimiter listing, one round trip), lazy per-ns `repo.Connect`(ReadOnly)+`repo.Open` cached, `ListSnapshots` (`ListSnapshotManifests`+`LoadSnapshots`+`SortByTime`), `SnapshotInfo` DTO (no kopia types leak to UI). Added `KOPIA_CACHE_DIR` config (default `.kopia-cache`, resolved absolute). Handlers: `Backups` interface, `GET /` namespaces + `GET /repo/{ns}` snapshot table, `/static/` serving, humanBytes/humanTime funcs. UI: `partials.html`+`namespaces.html`+`snapshots.html`, vendored htmx 2.0.4 + app.css. Tests: httptest (table-driven vs fake, asserts no `host_pods` leak), live integration (`-tags=integration`), chromedp E2E (`make e2e`, `-tags=e2e`, fake backend). Deps: kopia v0.22.3, minio-go/v7, chromedp.
- **Verified:** unit+httptest ✓, integration vs garage ✓ (30 namespaces incl `paperless`, real snapshot w/ backup name+tags+size), `make e2e` ✓, live server renders real paperless snapshots, no source-path leak, read-only.
- **Gotcha found & fixed:** relative kopia `CacheDirectory` → SIGSEGV nil-deref in content cache. `kopia.New` now `filepath.Abs`-resolves it. Tests passed despite this (used absolute temp dirs); only the live server run caught it. See DECISIONS.md / KOPIA.md.
- **Next:** **M2** — browse dir tree: `Dir(ns, snapID, path)` walking `fs.Directory` from `rootEntry.obj`; confirm `fs` iterate/Open API at v0.22.3; `GET /repo/{ns}/snap/{id}/browse/{path...}` + breadcrumb + htmx partial; chromedp navigate-into-dir E2E.
- **Blockers:** none. (`make` still not on PATH in shell; raw `go`/`go test -tags=...` used.)

## 2026-06-26 — M0 Scaffold (DONE)
- **Done:** Go 1.26 module scaffold. `internal/config` (godotenv `.env` load + required-var validation, aggregates all missing). `internal/web` stdlib `ServeMux` server: `GET /healthz`→`ok`, `GET /{$}`→embedded `index.html`. Root `assets` package embeds `web/templates` + `web/static` (`all:` prefix needed for `.gitkeep`). Makefile (run/test/test-integration/build/docker), `.env.example`. Verified: `go build`/`vet`/`test` green; server boots + serves both routes (200), non-root 404s, missing creds fail fast. Updated CLAUDE.md build section, PLAN.md (M0 DONE).
- **Next:** **M1** — `internal/kopia` RepoManager: `ListNamespaces` (S3 blob list under `KOPIA_PREFIX`), `Open(ns)`, `ListSnapshots`; `GET /` + `GET /repo/{ns}` pages; integration test vs garage.
- **Blockers:** `make` installed via winget but not yet on PATH in active shell (needs refresh); raw `go` commands work meanwhile.

## 2026-06-26 — Planning & docs setup
- **Done:** Decided stack (Go + kopia lib + htmx). Verified live connection to Garage S3 (`paperless` repo): endpoint/region/prefix/password all confirmed working, snapshot JSON shape captured. Wrote CLAUDE.md and `docs/` (PLAN, ARCHITECTURE, KOPIA, DECISIONS, JOURNAL). Go installed via winget (needs new shell to appear on PATH).
- **Next:** Start **M0** (scaffold). Confirm `go` is on PATH first.
- **Blockers:** none.
