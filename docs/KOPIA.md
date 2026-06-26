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

## Go library notes (to confirm during implementation)

- Storage: `repo/blob/s3` `s3.New` with `BucketName, Endpoint, AccessKeyID, SecretAccessKey, Prefix, Region, DoNotUseTLS`.
- Open: `repo.Connect` then `repo.Open` with the password.
- Snapshots: `snapshot.ListSnapshots`.
- Walk: `snapshot/snapshotfs` + `fs` to get directory entries from `rootEntry.obj`.
- Read file: object reader from the repository; directory → build tar by walking `fs.Directory`.
- Confirm exact APIs against the installed kopia version (CLI `0.22.3`).
