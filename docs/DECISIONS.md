# Decision log

> Append-only. Newest at top. One short entry per non-trivial decision: what + why.

## 2026-06-26 — M4 folder tar download
- **Same `/download/{path...}` route for both files and directories.** No extra route, no query param. The handler calls `OpenFile` first; `ErrNotAFile` returned for any directory (incl. empty path = snapshot root) triggers the tar branch. Clean branching on sentinel errors; no extra round trip.
- **`io.Writer` signature for `TarDir` (not `io.Pipe`).** Handler sets headers, then calls `TarDir(ctx, ns, snapID, path, w)`. Backpressure is trivial (handler blocks on write), no goroutine needed. Content-Length is unknown so chunked transfer encoding is used automatically by Go's `http.ResponseWriter`.
- **Plain (uncompressed) tar.** kopia already compresses blocks internally; re-gzip would gain little on many file types and costs CPU per request. Extension `.tar`, `Content-Type: application/x-tar`.
- **`descendToDir` helper extracted.** `Dir` and `TarDir` both descend to a directory; the shared logic is now in a single private method, mapping `kopiafs.ErrEntryNotFound → ErrNotFound` and non-dir segment → `ErrNotADirectory`. `OpenFile` is intentionally not changed (it descends to the parent and resolves the last segment separately — different shape).
- **`kopiafs.IterateEntries` for tree walk (not `GetAllEntries`).** Streaming callback avoids materializing a slice for each directory level; better for deep trees. Symlinks are written to the tar (TypeSymlink + Readlink target); unknown types are logged and skipped.

## 2026-06-26 — M2 browse + SPA navigation
- **htmx partial swap with `hx-push-url`** for dir navigation. Handler branches on `HX-Request: true` header: htmx gets only the `browse-content` fragment (inner HTML of `#listing`); plain browser gets the full `browse.html` page. `hx-push-url="true"` keeps the URL bar in sync so the back button works and links are shareable. `href` fallback on every dir link means navigation works without JS.
- **Path sanitization in handler layer** (`cleanBrowsePath`). `{path...}` URL wildcard value is attacker-controllable. Strategy: prefix `"/"` + `path.Clean` (resolves `..` safely by treating as absolute) → strip leading `/`. After clean, `..` cannot escape root. Defensive segment check added as belt-and-suspenders. No existing sanitizer existed; new unit test covers edge cases.
- **kopia fs tree walk API used** (`snapshotfs.SnapshotRoot` + `fs.Directory.Child` + `fs.GetAllEntries`). `snapshot.LoadSnapshot` (singular) fetches one manifest by ID. `snapshotfs.SnapshotRoot(rep, man)` returns `fs.Entry`; asserted to `fs.Directory`. Path descends via `dir.Child(ctx, seg)` per segment. Sort: dirs first, then alphabetically.

## 2026-06-26 — M1 data-layer choices
- **Namespace enumeration via minio-go delimiter listing**, not kopia's `blob.Storage`. kopia's `Storage` exposes no delimiter/common-prefix listing, so deriving namespaces from it means scanning *every* blob in *every* repo. minio-go (already a transitive kopia dep) does a delimiter `ListObjects` returning common prefixes (`kopia/<ns>/`) in one round trip — cheap and always fresh, no cache/staleness logic. Cost: a second S3 client path alongside kopia's, encapsulated in `internal/kopia`. This supersedes ARCHITECTURE.md's earlier "no separate AWS SDK needed" note (minio ≠ AWS SDK, but the intent was kopia-only S3 access). Verified live: returns 30 real namespaces incl. `paperless`.
- **Absolute kopia cache directory (required).** kopia's content cache nil-derefs (SIGSEGV in `contentCacheImpl.fetchBlobInternal`, storage `c.st` nil) when `CachingOptions.CacheDirectory` is a *relative* path. `kopia.New` resolves `KOPIA_CACHE_DIR` via `filepath.Abs` before use. Caught by live server run; unit/integration tests passed because they used absolute temp dirs.
- **Connect-then-Open, cached per namespace.** `repo.Connect` (writes config file, `ReadOnly:true`) only when the per-ns config file is absent, then `repo.Open`; the open `repo.Repository` is cached in the Manager and reused across requests. Read-only enforced three ways: `s3.New(...,false)`, `ClientOptions.ReadOnly=true`, and only ever calling `repo.Repository` (never the writer).

## 2026-06-26 — UI layering & testing strategy
- **Strict 3-layer split:** pure `internal/kopia` data layer (no HTTP/HTML) → `internal/web` handlers → `web/templates`. Makes the UI a separately-testable layer; handlers depend on a data-layer interface so they can be tested against a fake. Kept server-rendered htmx (rejected SPA + JSON API as unneeded build complexity).
- **Test pyramid:** Go unit (data logic) + Go integration vs real garage (`-tags=integration`) + `httptest` (handlers/HTML) + `chromedp` headless browser E2E (`make e2e`, build-tagged, committed/CI-runnable). **kapture MCP** kept for ad-hoc visual/screenshot checks of the live app during dev (already configured; needs Chrome extension + open tab). Rejected Playwright MCP — chromedp keeps E2E in-repo and deterministic with no external MCP dependency.

## 2026-06-26 — Initial stack
- **Backend in Go using the kopia Go library.** Kopia's only first-class programmatic API is Go; native embedding gives streaming downloads, multi-repo handling, and a single binary. Rejected: CLI subprocess (state/concurrency/temp-file pain), kopia server REST (one-repo-per-connection, fights the per-namespace layout).
- **Frontend: `html/template` + htmx.** App is mostly file-tree navigation + download links; server rendering keeps it a single binary with no JS build. Rejected SPA as unnecessary complexity for a first from-scratch build.
- **stdlib `http.ServeMux`** (Go 1.22 pattern routing) — no router dependency needed.
- **No in-app auth.** Deployed behind an SSO reverse proxy.
- **Strictly read-only** against kopia repos/S3.
- **Secrets via env only**, `.env` gitignored.
