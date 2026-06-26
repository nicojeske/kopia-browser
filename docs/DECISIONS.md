# Decision log

> Append-only. Newest at top. One short entry per non-trivial decision: what + why.

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
