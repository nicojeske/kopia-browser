# Project Plan — kopia-browser

> **Living document.** Claude keeps this current. At the start of a milestone set its status to
> `IN PROGRESS`; at the end tick its checkboxes and set `DONE`. Record surprises in
> [JOURNAL.md](JOURNAL.md) and decisions in [DECISIONS.md](DECISIONS.md).

## Goal

Web app to browse Velero-created kopia backups on Garage S3: list namespaces (one repo each),
list snapshots per namespace, browse snapshot file trees, download single files and whole folders.
Single Go binary + htmx UI, deployed in k8s behind an SSO reverse proxy (no in-app auth).

## Status overview

| Milestone | Status |
|-----------|--------|
| M0 Scaffold | DONE |
| M1 List namespaces + snapshots | DONE |
| M2 Browse dir tree | DONE |
| M3 Download single file | DONE |
| M4 Download folder (tar) | DONE |
| M5 UI refinement & E2E hardening | DONE |
| M6 Docker + k8s | DONE |
| M7 Dashboard stats & enriched sidebar | DONE |
| M8 CI/CD (GitHub Actions) | DONE |

Statuses: `TODO` → `IN PROGRESS` → `DONE`.

## Layering & testing model

Every feature is built as three thin layers, each with its own test type. This keeps the UI a
separately-testable layer rather than an afterthought.

| Layer | Package | Responsibility | Test type |
|-------|---------|----------------|-----------|
| Data | `internal/kopia` | pure kopia ops, no HTTP/HTML | Go unit + integration (real garage, `-tags=integration`) |
| Handler | `internal/web` | call data layer, render templates | Go `httptest` (assert status + HTML) |
| UI | `web/templates` | htmx pages, interactions, downloads | browser E2E via `chromedp` (headless, `make e2e`) |

A feature isn't `DONE` until all three layers exist and their tests pass.

## Milestones

### M0 — Scaffold — `DONE`
- [x] `git init`, `.gitignore` (`.env`, binaries, kopia cache, tmp)
- [x] `go.mod` (`github.com/nicojeske/kopia-browser`), Go 1.26
- [x] Directory layout (see [ARCHITECTURE.md](ARCHITECTURE.md))
- [x] `internal/config` — load + validate env (and `.env` in dev); aggregates all missing required vars
- [x] Minimal HTTP server: `/healthz` + a hello template render (embedded via root `assets` pkg)
- [x] `Makefile` (run/test/test-integration/build/docker)
- [x] `.env.example` (empty values, committed)
- [x] CLAUDE.md + docs committed
- **Verify:** ✅ build/vet/test green; server serves `/healthz`→`ok` and `/`→hello (200), `/{$}` 404s non-root; missing-required vars fail fast naming each.

### M1 — List namespaces + snapshots — `DONE`
- **Data:** `internal/kopia` Manager (RepoManager)
  - [x] `ListNamespaces()` — minio-go delimiter `ListObjects` under `KOPIA_PREFIX`, derive first path segment (see DECISIONS.md; cheaper than scanning all blobs)
  - [x] `open(ns)` — connect + open repo once per namespace, cached, read-only
  - [x] `ListSnapshots(ns)` via `snapshot.ListSnapshotManifests`+`LoadSnapshots`, newest first
  - [x] Integration test vs real garage: `paperless` present; snapshots non-empty (verified live: 30 namespaces)
- **Handler:**
  - [x] `GET /` → namespace list page
  - [x] `GET /repo/{ns}` → snapshot table (shows `tags.backup`, `startTime`, human size, file count; raw source path hidden)
  - [x] `httptest` tests assert status + expected HTML (table-driven, fake data layer; asserts no `host_pods` leak)
- **UI + test harness:**
  - [x] htmx page templates for both routes (`namespaces.html`, `snapshots.html`, shared `partials.html`); vendored htmx + css served at `/static/`
  - [x] E2E harness: `httptest.NewServer` on a random port + fake data layer; `make e2e` runs `chromedp` (build tag `e2e`)
  - [x] First `chromedp` E2E: load `/`, click a namespace, see snapshot table
- **Verify:** ✅ unit + integration (live garage) + httptest + `make e2e` all green; live server renders real `paperless` snapshots, no source-path leak.

### M2 — Browse dir tree — `DONE`
- **Data:** [x] `Dir(ns, snapID, path)` — walk `fs.Directory` from `rootEntry.obj`; unit/integration test on `paperless`
- **Handler:** [x] `GET /repo/{ns}/snap/{id}/browse/{path...}`; httptest for listing + breadcrumb + path-escaping; htmx fragment vs full-page branch
- **UI:** [x] htmx dir listing partial + breadcrumb + SPA swap (`hx-push-url`); chromedp E2E navigates into a directory and verifies URL + content
- **Verify:** unit + httptest ✅ (incl. `cleanBrowsePath`, htmx branch, leak guard); `make e2e` ✅; integration pending live run.

### M3 — Download single file — `DONE`
- **Data:** [x] `OpenFile(...)` → `io.ReadSeekCloser`; integration test reads a known file
- **Handler:** [x] `GET /repo/{ns}/snap/{id}/download/{path...}` for files; httptest checks headers (`Content-Disposition`, type) + body
- **UI:** [x] download links on listing; chromedp E2E asserts /download/ href
- **Verify:** unit + httptest ✅; `make e2e` ✅; integration + live checksum pending live run.

### M4 — Download folder (tar) — `DONE`
- **Data:** [x] `descendToDir` helper extracted (shared by `Dir` + `TarDir`); `TarDir(ctx, ns, snapID, dirPath, w io.Writer)` streams plain tar via `writeTarTree` + `kopiafs.IterateEntries`; handles dirs, files, symlinks; `ErrNotADirectory` sentinel added; unit test on in-memory fakes (`tar_test.go`); integration test `TestTarDirLive` on `paperless`
- **Handler:** [x] `TarDir` added to `Backups` interface; `handleDownload` branches on `ErrNotAFile` → tar (Content-Type: application/x-tar, chunked); root empty path yields `<ns>.tar`; single-file path unchanged; `ErrNotFound` → 404; httptest cases for root tar, subdir tar, file (unchanged), missing → 404
- **UI:** [x] browse.html: "Download this folder (.tar)" button near h1; `.tar-link` on each dir row; app.css styles; chromedp E2E `TestE2EFolderTarLink` asserts hrefs contain `/download/`
- **Verify:** `go test ./...` ✅; `make e2e` ✅; integration + live manual pending live run.

### M5 — UI refinement & E2E hardening — `DONE`
- [x] Volume navigation layer: `/repo/{ns}` lists volumes; `/repo/{ns}/vol/{volume...}` lists snapshots for that volume. Grouping handler-side from existing `ListSnapshots`. Untagged snapshots → "(no volume)".
- [x] Dark-theme UI redesign: CSS custom properties (`--bg`, `--ac` teal, etc.), 248px sidebar + main shell, `table.data-table` unified table component, inline SVG icon library as `{{define "icon-*"}}` template helpers, `page-eyebrow` / breadcrumb / stats-row patterns.
- [x] Self-hosted Geist fonts: 11 woff2 subsets (Geist + Geist Mono variable, OFL), `@font-face` rules in `app.css`, served from `/static/fonts/`. Zero Google CDN requests (verified via kapture network monitor).
- [x] Persistent sidebar namespace nav: `ListNamespaces` injected into `handleVolumes`, `handleSnapshots`, `handleBrowse` (full-page path only; htmx fragment path skipped). Graceful degrade on error.
- [x] Styled error page: `error.html` template + `renderError` calls `ExecuteTemplate` instead of `http.Error`. Monospace error box, "← Back to namespaces" link.
- [x] Updated E2E selectors for redesigned CSS classes (`table.data-table`, `.entry-dir-link`, `.entry-file-link`, `.btn-tar`).
- [x] kapture visual pass: index, volumes, snapshots, browse, error page — all visually confirmed.
- **Verify:** `go test ./...` ✅, `make e2e` ✅, kapture visual pass ✅, no external font requests ✅.

### M7 — Dashboard stats & enriched sidebar — `DONE`
- [x] `internal/kopia/stats.go` — `NamespaceStats`, `StatsSnapshot`, `computeNamespaceStats` (pure helper), `StatsCache` (background ticker + RWMutex).
- [x] `internal/kopia/stats_test.go` — table-driven unit tests for `computeNamespaceStats`.
- [x] `internal/config/config.go` — `StatsRefreshInterval` from `STATS_REFRESH_INTERVAL` env (default 15m).
- [x] `cmd/kopia-browser/main.go` — wire `StatsCache`, `go cache.Run(ctx)`, pass to `NewServer`; graceful shutdown via `context.WithCancel`.
- [x] `internal/web/server.go` — `Stats` interface, `injectSidebarData` helper, `handleIndex` enriched with stat cards + ns cards; `humanRel`, `humanCount` template funcs; `sidebarNSItem`/`sidebarRepo` types.
- [x] `web/templates/partials.html` — sidebar search, enriched nav rows (dot + name + snapshot count), repository footer (drive icon + size + composition bar + shield line), new icon defines (`icon-search`, `icon-drive`, `icon-caret-up`, `icon-caret-down`), sidebar search JS in `foot`.
- [x] `web/templates/namespaces.html` — 4 stat cards, search box + sort pills, enriched namespace grid (volumes/snapshots/stored mini-stats + size bar + last-backup), client-side filter+sort JS, "calculating" note when not ready.
- [x] `web/static/app.css` — stat cards, search box, sort pills, enriched ns-card stats, size bar, sidebar search, sidebar repo footer.
- [x] Tests: `fakeStats`/`sampleStats()` in `fake_test.go`; 5 new handler assertions; 2 new E2E tests (dashboard search + sort pill).
- **Verify:** `go test ./...` ✅, `go test -tags=e2e ./internal/web` ✅ (7 E2E), kapture visual pass (see JOURNAL.md).

### M6 — Docker — `DONE`
- [x] Multi-stage `Dockerfile` (distroless/scratch)
- **Verify:** ✅ `docker build -t kopia-browser .`; `docker run --env-file .env` boots + serves `/healthz` → `ok`.

### M8 — CI/CD (GitHub Actions) — `DONE`
- [x] `.github/workflows/ci.yml` — quality gate: `go vet ./...` + `go test ./...` + `go build` on every push/PR to `main`
- [x] `.github/workflows/docker-publish.yml` — build + push `ghcr.io/nicojeske/kopia-browser` on `main` push and `v*` tags; semver tagging via `docker/metadata-action`; GHA layer cache (`type=gha`)
- [x] `.github/dependabot.yml` — weekly auto-PRs for `gomod` + `github-actions`
- **Registry:** GHCR — uses built-in `GITHUB_TOKEN`, no extra secrets. Arch: `linux/amd64`.
- **CI skips:** integration tests (needs garage creds) and E2E tests (needs Chrome) — both run locally only.
- **Verify:** ✅ `go test ./...` + `go build ./cmd/kopia-browser` pass locally (mirror CI steps). After first GitHub push: `ci` workflow green; `docker-publish` pushes `:latest` + `:sha-<short>` to GHCR. Tag `v0.1.0` → `:0.1.0` / `:0.1` published.

## Out of scope
- In-app auth (handled by SSO reverse proxy)
- Any write/restore-into-cluster operation (read + download only)
