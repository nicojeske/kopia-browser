# Decision log

> Append-only. Newest at top. One short entry per non-trivial decision: what + why.

## 2026-06-26 — Initial stack
- **Backend in Go using the kopia Go library.** Kopia's only first-class programmatic API is Go; native embedding gives streaming downloads, multi-repo handling, and a single binary. Rejected: CLI subprocess (state/concurrency/temp-file pain), kopia server REST (one-repo-per-connection, fights the per-namespace layout).
- **Frontend: `html/template` + htmx.** App is mostly file-tree navigation + download links; server rendering keeps it a single binary with no JS build. Rejected SPA as unnecessary complexity for a first from-scratch build.
- **stdlib `http.ServeMux`** (Go 1.22 pattern routing) — no router dependency needed.
- **No in-app auth.** Deployed behind an SSO reverse proxy.
- **Strictly read-only** against kopia repos/S3.
- **Secrets via env only**, `.env` gitignored.
