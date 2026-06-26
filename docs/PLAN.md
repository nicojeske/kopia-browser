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
| M1 List namespaces + snapshots | TODO |
| M2 Browse dir tree | TODO |
| M3 Download single file | TODO |
| M4 Download folder (tar) | TODO |
| M5 UI polish | TODO |
| M6 Docker + k8s | TODO |

Statuses: `TODO` → `IN PROGRESS` → `DONE`.

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

### M1 — List namespaces + snapshots — `TODO`
- [ ] `internal/kopia` RepoManager: `ListNamespaces()` (list S3 blobs under `KOPIA_PREFIX`, derive first path segment)
- [ ] RepoManager `Open(ns)` — connect + open repo once per namespace, cached, read-only
- [ ] `ListSnapshots(ns)` via `snapshot.ListSnapshots`
- [ ] `GET /` → namespace list page
- [ ] `GET /repo/{ns}` → snapshot table (show `tags.backup`, `startTime`, size, file count; hide raw source path)
- [ ] Integration test vs real garage: `paperless` present; snapshots non-empty
- **Verify:** integration test green; pages render in browser.

### M2 — Browse dir tree — `TODO`
- [ ] `Dir(ns, snapID, path)` — walk `fs.Directory` from `rootEntry.obj`
- [ ] `GET /repo/{ns}/snap/{id}/browse/{path...}` — htmx dir listing + breadcrumb
- **Verify:** browse paperless snapshot in browser.

### M3 — Download single file — `TODO`
- [ ] `OpenFile(...)` → `io.ReadSeekCloser`; stream with correct headers
- [ ] `GET /repo/{ns}/snap/{id}/download/{path...}` for files
- **Verify:** downloaded file checksum matches `kopia restore` output.

### M4 — Download folder (tar) — `TODO`
- [ ] `TarDir(...)` — stream tar of a directory subtree on the fly
- [ ] Same download route serves tar when target is a directory
- **Verify:** tar extracts; contents match.

### M5 — UI polish — `TODO`
- [ ] Snapshot metadata, human sizes, sorting, breadcrumbs, error pages
- [ ] Static assets (htmx, minimal css)
- **Verify:** manual pass over all flows.

### M6 — Docker + k8s — `TODO`
- [ ] Multi-stage `Dockerfile` (distroless/scratch)
- [ ] k8s manifests: Deployment + Service + Ingress notes; env via Secret
- [ ] Persistent kopia cache volume (or sized emptyDir)
- **Verify:** image runs locally with env; manifests reviewed.

## Out of scope
- In-app auth (handled by SSO reverse proxy)
- Any write/restore-into-cluster operation (read + download only)
