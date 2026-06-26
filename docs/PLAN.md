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
| M2 Browse dir tree | TODO |
| M3 Download single file | TODO |
| M4 Download folder (tar) | TODO |
| M5 UI refinement & E2E hardening | TODO |
| M6 Docker + k8s | TODO |

Statuses: `TODO` → `IN PROGRESS` → `DONE`.

## Layering & testing model

Every feature is built as three thin layers, each with its own test type. This keeps the UI a
separately-testable layer rather than an afterthought.

| Layer | Package | Responsibility | Test type |
|-------|---------|----------------|-----------|
| Data | `internal/kopia` | pure kopia ops, no HTTP/HTML | Go unit + integration (real garage, `-tags=integration`) |
| Handler | `internal/web` | call data layer, render templates | Go `httptest` (assert status + HTML) |
| UI | `web/templates` | htmx pages, interactions, downloads | browser E2E via `chromedp` (headless, `make e2e`) + kapture MCP ad-hoc |

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

### M2 — Browse dir tree — `TODO`
- **Data:** [ ] `Dir(ns, snapID, path)` — walk `fs.Directory` from `rootEntry.obj`; unit/integration test on `paperless`
- **Handler:** [ ] `GET /repo/{ns}/snap/{id}/browse/{path...}`; httptest for listing + breadcrumb + path-escaping
- **UI:** [ ] htmx dir listing partial + breadcrumb; chromedp E2E navigates into a directory and back
- **Verify:** all test layers green; browse paperless in browser.

### M3 — Download single file — `TODO`
- **Data:** [ ] `OpenFile(...)` → `io.ReadSeekCloser`; integration test reads a known file
- **Handler:** [ ] `GET /repo/{ns}/snap/{id}/download/{path...}` for files; httptest checks headers (`Content-Disposition`, type) + body
- **UI:** [ ] download links on listing; chromedp E2E triggers a download and checks bytes
- **Verify:** downloaded file checksum matches `kopia restore` output.

### M4 — Download folder (tar) — `TODO`
- **Data:** [ ] `TarDir(...)` — stream tar of a directory subtree on the fly; unit test on in-memory tree + integration on `paperless`
- **Handler:** [ ] same download route serves tar when target is a directory; httptest validates tar stream
- **UI:** [ ] "download folder" affordance; chromedp E2E downloads + extracts a folder
- **Verify:** tar extracts; contents match.

### M5 — UI refinement & E2E hardening — `TODO`
- [ ] Snapshot metadata, human sizes, sorting, error pages, empty states
- [ ] Static assets finalized (htmx, minimal css)
- [ ] Broaden chromedp E2E to cover full happy path + key error paths; kapture visual pass + screenshots
- **Verify:** full E2E suite green; manual visual pass.

### M6 — Docker + k8s — `TODO`
- [ ] Multi-stage `Dockerfile` (distroless/scratch)
- [ ] k8s manifests: Deployment + Service + Ingress notes; env via Secret
- [ ] Persistent kopia cache volume (or sized emptyDir)
- **Verify:** image runs locally with env; manifests reviewed.

## Out of scope
- In-app auth (handled by SSO reverse proxy)
- Any write/restore-into-cluster operation (read + download only)
