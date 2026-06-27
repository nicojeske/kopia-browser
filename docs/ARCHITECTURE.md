# Architecture — kopia-browser

> Update when structure or a component changes.

## Overview

Single Go binary. Serves a server-rendered htmx UI and streams downloads. Talks to Garage S3
through the kopia Go library, opening one kopia repository per namespace (read-only).

```
browser ──HTTP/htmx──> kopia-browser (Go) ──kopia lib──> Garage S3 (velero-backup/kopia/<ns>/)
```

## Directory layout

```
cmd/kopia-browser/main.go   entrypoint: load config, build RepoManager, start server
internal/config/            env + .env loader and validation
internal/kopia/             RepoManager — all kopia interaction (read-only)
internal/web/               http handlers, routing, template rendering
web/templates/              html/template files + htmx partials (incl. error.html)
web/static/                 htmx.min.js, app.css (dark theme, CSS vars)
web/static/fonts/           self-hosted Geist + Geist Mono woff2 (OFL; embedded in binary)
docs/                       this documentation
```

## Components

### internal/config
Loads env vars (see CLAUDE.md), validates required ones, loads `.env` in dev. Returns a typed `Config`.

### internal/kopia — StatsCache

`StatsCache` runs in the background and periodically (every `STATS_REFRESH_INTERVAL`, default 15m) calls `ListNamespaces` + `ListSnapshots` per namespace to compute `StatsSnapshot`:
- `NamespaceStats` per ns: volume count, snapshot count, total size (latest-per-volume), last backup time.
- Aggregate totals: `TotalSize`, `TotalSnapshots`, `NamespaceCount`, `MaxSize` (for bar scaling).
- `Ready bool` — false until the first refresh completes; handlers render "calculating" placeholders meanwhile.

Handlers call `cache.Get()` (sync, RWMutex RLock) to get a copy of the last snapshot. The `Manager` is concurrency-safe so the background goroutine and live request handlers share it without issue.

### internal/kopia — RepoManager
The heart. Holds `map[namespace]*openRepo` with a mutex; repos opened lazily and reused.
**Strictly read-only.** Responsibilities:
- `ListNamespaces()` — uses **minio-go** (already a transitive kopia dep) with a delimiter `ListObjects` under `KOPIA_PREFIX`, returning common prefixes (`kopia/<ns>/`) → first path segment is the namespace. kopia's own `blob.Storage` has no delimiter listing, so deriving namespaces from it would mean scanning every blob; the minio path is one round trip. (minio ≠ AWS SDK; see DECISIONS.md 2026-06-26.)
- `Open(ns)` — `repo.Connect` (write config to a temp/cache path) + `repo.Open`, cached per namespace.
- `ListSnapshots(ns)` — `snapshot.ListSnapshots`.
- `Dir(ns, snapID, path)` — resolve snapshot, walk `fs.Directory` from `rootEntry.obj` to the requested path, return entries.
- `OpenFile(ns, snapID, path)` — `io.ReadSeekCloser` for a file object.
- `TarDir(ns, snapID, path, w)` — stream a tar of a directory subtree.

kopia needs a cache directory — configured via `KOPIA_CACHE_DIR` to a writable path (local `.kopia-cache` in dev, volume in k8s). **Must be absolute**: `kopia.New` resolves it with `filepath.Abs` because kopia's content cache nil-derefs on a relative path (see DECISIONS.md).

### internal/web
stdlib `http.ServeMux` (Go 1.22 method + `{wildcard}` patterns, no router dependency).

Routes:
| Method+Path | Purpose |
|-------------|---------|
| `GET /` | namespace list |
| `GET /repo/{ns}` | volume list for namespace (grouped by Velero `volume` tag) |
| `GET /repo/{ns}/vol/{volume...}` | snapshot table for one volume (`volume` = raw tag value; empty = untagged) |
| `GET /repo/{ns}/snap/{id}/browse/{path...}` | directory listing (htmx partial + breadcrumb) |
| `GET /repo/{ns}/snap/{id}/download/{path...}` | stream file, or tar if path is a directory |
| `GET /healthz` | liveness |

UI: server-rendered `html/template`; htmx swaps directory listings without full page reloads. Dark-theme shell: 248px sidebar + main column; CSS custom properties; self-hosted Geist fonts.

**Dashboard (`/`):** four summary stat cards (namespace count, total snapshots, total stored, last backup), client-side search + sort pills (Size/Count/Name/Recent), per-namespace cards with volume count / snapshot count / stored size / size bar / last-backup line. Stats sourced from `StatsCache.Get()`. When not ready: "—" placeholders + "Statistics are being calculated…" banner.

**Sidebar:** search input (client-side filter of nav rows), namespace nav with per-namespace snapshot count from cache, repository footer (drive icon + total size + up-to-5-segment composition bar + shield line). Sidebar search JS in `foot` partial. All data injected by `injectSidebarData` helper in handlers.

## Data flow: browse
1. Handler parses `ns`, `snapID`, `path`.
2. RepoManager opens/reuses repo for `ns`, finds snapshot by id, walks to `path`.
3. Returns entries (name, type, size, mtime) → template renders rows with links.

## Data flow: download
- File: open object reader, set `Content-Disposition`/`Content-Type`, `io.Copy` to response.
- Directory: set tar headers, walk subtree, write tar entries streaming (no temp disk).

## Layering (strict)

UI is a separately-testable layer on top of a pure data layer:

```
web/templates (htmx)  ── rendered by ──>  internal/web (handlers)  ── calls ──>  internal/kopia (data)
   browser E2E                               httptest                              unit + integration
```

- `internal/kopia` has no knowledge of HTTP or HTML — it returns Go values. This lets handlers be
  tested against a fake data layer (define an interface the handlers depend on; real RepoManager +
  a test fake both implement it).
- `internal/web` handlers translate data ⇄ templates only.
- `web/templates` hold all markup; verified by real-browser E2E.

## Testing

| Layer | Tool | Command | Notes |
|-------|------|---------|-------|
| Data ops | Go unit | `make test` | pure logic, no I/O |
| Data ops | Go integration | `make test-integration` (`-tags=integration`) | real garage; skips without creds |
| Handlers | Go `httptest` | `make test` | assert status + HTML against a fake data layer |
| UI end-to-end | `chromedp` (headless Chrome) | `make e2e` (build tag `e2e`) | boots server on random port, drives real browser; committed + CI-runnable |

E2E harness lives in `internal/web` (or `internal/e2e`): a helper boots the full server on a random
port against a chosen data layer (real garage or fake), returns the base URL, then `chromedp`
navigates it. Keep E2E behind a build tag so `make test` stays fast and offline.

## Notes / constraints
- Velero `source.path` is an ugly host-pod path; UI shows `tags` + `startTime` instead. See [KOPIA.md](KOPIA.md).
- Concurrency: RepoManager methods must be safe for concurrent requests.
