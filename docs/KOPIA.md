# Kopia / Velero / Garage domain facts

> Verified facts about the backup storage. Update when you learn something new.
> **Never put real secret values in this file.**

## Storage layout (verified live 2026-06-26)

- **Backend:** self-hosted Garage S3.
- **Endpoint:** `tailscale-garage-s3:3900`, **HTTP** (no TLS → kopia `--disable-tls`, `DoNotUseTLS`).
- **Region:** `garage`.
- **Addressing:** path-style.
- **Bucket:** `velero-backup`.
- **Repos:** one kopia repository per k8s namespace at prefix `kopia/<namespace>/`.
- **Password:** all repos share one Velero-set password (NOT the `static-passw0rd` default). Supplied via `KOPIA_REPO_PASSWORD` env only.
- **Access:** S3 access key + secret via `S3_ACCESS_KEY` / `S3_SECRET_KEY` env only.

## Verified working CLI connect (read-only probe)

```bash
export KOPIA_CONFIG_PATH="$(mktemp -d)/repo.config"
export KOPIA_PASSWORD='<repo-password>'
kopia repository connect s3 \
  --bucket velero-backup --prefix "kopia/<namespace>/" \
  --endpoint "tailscale-garage-s3:3900" --disable-tls \
  --region garage \
  --access-key "<key>" --secret-access-key "<secret>"
kopia snapshot list --json
kopia repository disconnect
```

## Snapshot JSON shape (`kopia snapshot list --json`)

Each snapshot object:
- `id` — snapshot manifest id
- `source` — `{host, userName, path}`; for Velero `path` is an ugly host-pod path like
  `/host_pods/<uid>/volumes/kubernetes.io~empty-dir/backup` → **do not show in UI**
- `startTime` / `endTime`
- `stats` — `{totalSize, fileCount, dirCount, ...}`
- `rootEntry` — `{name, type, mode, mtime, obj, summ}`; **`obj`** (e.g. `k63a091b9...`) is the object id to walk the tree from
- `tags` — friendly Velero metadata: `backup` (backup name), `ns`, `pod`, `volume`, `snapshot-uploader` → **show these in UI**
- `pins`, `retentionReason`

## Go library notes (verified M1 against `github.com/kopia/kopia v0.22.3`)

- **Library pinned:** `github.com/kopia/kopia v0.22.3` (matches CLI 0.22.3).
- **Namespace listing:** done via **minio-go** delimiter `ListObjects` (common prefixes), NOT kopia's `blob.Storage` (which has no delimiter listing). Verified live: 30 namespaces incl. `paperless` (`ae2web ark camunda … paperless … website`).
- **Storage:** `s3.New(ctx, *s3.Options, isCreate bool)` — pass `isCreate=false`. Fields used: `BucketName, Endpoint, AccessKeyID, SecretAccessKey, Region, Prefix (=KOPIA_PREFIX+ns+"/"), DoNotUseTLS:true`.
- **Open:** `repo.Connect(ctx, cfgPath, st, password, &repo.ConnectOptions{ClientOptions:{ReadOnly:true}, CachingOptions:{CacheDirectory:<abs>}})` then `repo.Open(ctx, cfgPath, password, &repo.Options{})`. ⚠️ **`CacheDirectory` MUST be absolute** — a relative path causes a nil-deref SIGSEGV in kopia's content cache. Connect only when the config file is absent; reuse otherwise.
- **Snapshots:** `snapshot.ListSnapshotManifests(ctx, rep, nil, nil)` → `snapshot.LoadSnapshots(ctx, rep, ids)` → `snapshot.SortByTime(mans, true)` (newest first). Source-agnostic.
- **Manifest fields used:** `man.ID` (string), `man.Tags["backup"]`, `man.StartTime.ToTime()`, `man.Stats.TotalFileSize` (int64), `man.Stats.TotalFileCount` (int32).
- **Verified Velero tag set** (live `paperless` snapshot): `backup`, `backup-uid`, `ns`, `pod`, `pod-uid`, `snapshot-requester` (`pod-volume-backup-restore`), `snapshot-uploader` (`kopia`), `volume`. `backup` is the friendly name shown in the UI.
- **Dir walk (verified M2):** `snapshot.LoadSnapshot(ctx, rep, manifest.ID(snapID))` fetches one snapshot manifest. `snapshotfs.SnapshotRoot(rep, man) (fs.Entry, error)` — root entry; assert to `kopiafs.Directory`. Descend with `dir.Child(ctx, seg) (fs.Entry, error)` per path segment; re-assert to `kopiafs.Directory`. List with `kopiafs.GetAllEntries(ctx, dir) ([]fs.Entry, error)`. Each `fs.Entry` implements `os.FileInfo`: `Name()`, `IsDir()`, `Size()`, `ModTime()`. `kopiafs.ErrEntryNotFound` returned when child missing.
- **File streaming (verified M3):** `kopiafs.File` interface has `Open(ctx context.Context) (Reader, error)`. `kopiafs.Reader` embeds `io.ReadCloser` + `io.Seeker` + `Entry() (Entry, error)` — satisfies `io.ReadSeekCloser` directly (no wrapper). `kopiafs.ErrEntryNotFound` is the sentinel for missing children; check with `errors.Is`. `http.ServeContent(w, r, name, modTime, rc)` works directly since `Reader` is an `io.ReadSeeker` — gives free Range/If-Modified-Since/Content-Type sniff.
- **Still to confirm (M4+):** tar streaming for folder download (`archive/tar` + `kopiafs.GetAllEntries` recursive walk).
