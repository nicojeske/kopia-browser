package kopia

import (
	"archive/tar"
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	kopiafs "github.com/kopia/kopia/fs"
	"github.com/kopia/kopia/repo"
	"github.com/kopia/kopia/repo/blob/s3"
	"github.com/kopia/kopia/repo/content"
	"github.com/kopia/kopia/repo/manifest"
	"github.com/kopia/kopia/snapshot"
	"github.com/kopia/kopia/snapshot/policy"
	"github.com/kopia/kopia/snapshot/snapshotfs"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/nicojeske/kopia-browser/internal/config"
)

// Manager is the read-only kopia data layer. It enumerates namespaces (one
// kopia repo each under KopiaPrefix) via S3, and opens repositories lazily,
// caching one open *repo.Repository per namespace for reuse across requests.
// All methods are safe for concurrent use.
type Manager struct {
	cfg      *config.Config
	cacheDir string // absolute; kopia's content cache misbehaves on relative paths

	mu    sync.Mutex
	repos map[string]repo.Repository // ns -> open read-only repo
}

// New builds a Manager. It performs no S3 I/O; repos are opened on first use.
func New(cfg *config.Config) (*Manager, error) {
	// kopia's content cache nil-derefs when given a relative CacheDirectory, so
	// resolve to an absolute path up front.
	cacheDir, err := filepath.Abs(cfg.KopiaCacheDir)
	if err != nil {
		return nil, fmt.Errorf("resolve cache dir %q: %w", cfg.KopiaCacheDir, err)
	}
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return nil, fmt.Errorf("create cache dir %q: %w", cacheDir, err)
	}
	return &Manager{cfg: cfg, cacheDir: cacheDir, repos: map[string]repo.Repository{}}, nil
}

// ListNamespaces returns the namespace set: the first path segment under
// KopiaPrefix. It uses an S3 delimiter listing (common prefixes) so it costs a
// single round trip rather than scanning every blob.
func (m *Manager) ListNamespaces(ctx context.Context) ([]string, error) {
	mc, err := minio.New(m.cfg.S3Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(m.cfg.S3AccessKey, m.cfg.S3SecretKey, ""),
		Secure: false, // garage is plain HTTP (see docs/KOPIA.md)
		Region: m.cfg.S3Region,
	})
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}

	var namespaces []string
	// Recursive:false applies the "/" delimiter, so directory-like keys (the
	// per-namespace repo prefixes) come back as common prefixes ending in "/".
	for obj := range mc.ListObjects(ctx, m.cfg.S3Bucket, minio.ListObjectsOptions{
		Prefix:    m.cfg.KopiaPrefix,
		Recursive: false,
	}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("list %q: %w", m.cfg.KopiaPrefix, obj.Err)
		}
		if !strings.HasSuffix(obj.Key, "/") {
			continue // a stray object, not a namespace directory
		}
		ns := strings.TrimSuffix(strings.TrimPrefix(obj.Key, m.cfg.KopiaPrefix), "/")
		if ns != "" {
			namespaces = append(namespaces, ns)
		}
	}
	sort.Strings(namespaces)
	return namespaces, nil
}

// ListSnapshots returns all snapshots in the namespace's repo, newest first.
func (m *Manager) ListSnapshots(ctx context.Context, ns string) ([]SnapshotInfo, error) {
	rep, err := m.open(ctx, ns)
	if err != nil {
		return nil, err
	}

	ids, err := snapshot.ListSnapshotManifests(ctx, rep, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("list snapshot manifests for %q: %w", ns, err)
	}
	mans, err := snapshot.LoadSnapshots(ctx, rep, ids)
	if err != nil {
		return nil, fmt.Errorf("load snapshots for %q: %w", ns, err)
	}
	// RetentionReasons has json:"-" and is not stored in the manifest — it must
	// be computed from the effective retention policy for each source group.
	for _, group := range snapshot.GroupBySource(mans) {
		pol, _, _, polErr := policy.GetEffectivePolicy(ctx, rep, group[0].Source)
		if polErr == nil {
			pol.RetentionPolicy.ComputeRetentionReasons(group)
		}
	}

	mans = snapshot.SortByTime(mans, true) // reverse = newest first; SortByTime returns a new slice

	out := make([]SnapshotInfo, 0, len(mans))
	for _, man := range mans {
		// Pod-volume-backup sets Tags["volume"]; data-mover snapshots do not.
		// For data-mover, the PVC name is the last segment of source.path:
		//   "snapshot-data-upload-download/kopia/<ns>/<pvc-name>"
		volume := man.Tags["volume"]
		if volume == "" && man.Source.Path != "" {
			base := path.Base(man.Source.Path)
			if base != "." && base != "/" {
				volume = base
			}
		}

		// Prefer RootEntry.DirSummary for full-tree totals: man.Stats only
		// counts files actually uploaded in this run, so unchanged subtrees
		// are missing and the count is too low on incremental snapshots.
		size := man.Stats.TotalFileSize
		count := int64(man.Stats.TotalFileCount)
		if man.RootEntry != nil && man.RootEntry.DirSummary != nil {
			size = man.RootEntry.DirSummary.TotalFileSize
			count = man.RootEntry.DirSummary.TotalFileCount
		}

		out = append(out, SnapshotInfo{
			ID:             string(man.ID),
			BackupName:     man.Tags["backup"],
			Volume:         volume,
			StartTime:      man.StartTime.ToTime(),
			EndTime:        man.EndTime.ToTime(),
			TotalSize:      size,
			FileCount:      count,
			Tags:           man.Tags,
			RetentionRoles: retentionRoles(man.RetentionReasons),
			Pinned:         len(man.Pins) > 0,
			ErrorCount:     man.Stats.ErrorCount,
		})
	}
	return out, nil
}

// open returns the cached read-only repository for ns, opening it on first use.
func (m *Manager) open(ctx context.Context, ns string) (repo.Repository, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if rep, ok := m.repos[ns]; ok {
		slog.Debug("kopia: repo cache hit", "ns", ns)
		return rep, nil
	}

	slog.Debug("kopia: opening repo", "ns", ns)
	st, err := s3.New(ctx, &s3.Options{
		BucketName:      m.cfg.S3Bucket,
		Endpoint:        m.cfg.S3Endpoint,
		AccessKeyID:     m.cfg.S3AccessKey,
		SecretAccessKey: m.cfg.S3SecretKey,
		Region:          m.cfg.S3Region,
		Prefix:          m.cfg.KopiaPrefix + ns + "/",
		DoNotUseTLS:     true, // garage is plain HTTP
	}, false)
	if err != nil {
		return nil, fmt.Errorf("s3 storage for %q: %w", ns, err)
	}

	nsDir := filepath.Join(m.cacheDir, ns)
	if err := os.MkdirAll(nsDir, 0o700); err != nil {
		return nil, fmt.Errorf("cache dir for %q: %w", ns, err)
	}
	cfgPath := filepath.Join(nsDir, "repo.config")

	cachingOpts := content.CachingOptions{
		CacheDirectory:        filepath.Join(nsDir, "cache"),
		ContentCacheSizeBytes: m.cfg.KopiaContentCacheMB * 1024 * 1024,
	}

	// Connect writes the config file; only do it once. On a persistent cache the
	// file survives restarts, so reuse it and just Open.
	if _, statErr := os.Stat(cfgPath); os.IsNotExist(statErr) {
		err = repo.Connect(ctx, cfgPath, st, m.cfg.KopiaRepoPassword, &repo.ConnectOptions{
			ClientOptions:  repo.ClientOptions{ReadOnly: true},
			CachingOptions: cachingOpts,
		})
		if err != nil {
			return nil, fmt.Errorf("connect repo %q: %w", ns, err)
		}
	} else {
		// Patch existing config so cache settings stay current across upgrades.
		if serr := repo.SetCachingOptions(ctx, cfgPath, &cachingOpts); serr != nil {
			slog.Warn("kopia: could not update cache settings", "ns", ns, "err", serr)
		}
	}

	rep, err := repo.Open(ctx, cfgPath, m.cfg.KopiaRepoPassword, &repo.Options{})
	if err != nil {
		return nil, fmt.Errorf("open repo %q: %w", ns, err)
	}

	m.repos[ns] = rep
	return rep, nil
}

// ErrNotFound is returned when any segment of the path does not exist in the snapshot.
var ErrNotFound = errors.New("not found")

// ErrNotAFile is returned by OpenFile when the final path segment resolves to
// a directory. The download handler uses this to branch to the tar streaming path.
var ErrNotAFile = errors.New("path is a directory, not a file")

// ErrNotADirectory is returned by TarDir when the path resolves to a file
// rather than a directory.
var ErrNotADirectory = errors.New("path is not a directory")

// descendToDir opens the snapshot root and walks dirPath segment-by-segment,
// returning the kopiafs.Directory at that location (empty dirPath = root).
// Maps kopiafs.ErrEntryNotFound → ErrNotFound; a non-directory segment → ErrNotADirectory.
func (m *Manager) descendToDir(ctx context.Context, ns, snapID, dirPath string) (kopiafs.Directory, error) {
	rep, err := m.open(ctx, ns)
	if err != nil {
		return nil, err
	}

	man, err := snapshot.LoadSnapshot(ctx, rep, manifest.ID(snapID))
	if err != nil {
		return nil, fmt.Errorf("load snapshot %q: %w", snapID, err)
	}

	root, err := snapshotfs.SnapshotRoot(rep, man)
	if err != nil {
		return nil, fmt.Errorf("snapshot root for %q: %w", snapID, err)
	}

	dir, ok := root.(kopiafs.Directory)
	if !ok {
		return nil, fmt.Errorf("snapshot root of %q is not a directory", snapID)
	}

	if dirPath == "" {
		return dir, nil
	}

	for _, seg := range strings.Split(dirPath, "/") {
		child, err := dir.Child(ctx, seg)
		if err != nil {
			if errors.Is(err, kopiafs.ErrEntryNotFound) {
				return nil, fmt.Errorf("navigate to %q: %w", seg, ErrNotFound)
			}
			return nil, fmt.Errorf("navigate to %q: %w", seg, err)
		}
		childDir, ok := child.(kopiafs.Directory)
		if !ok {
			return nil, fmt.Errorf("%q is not a directory: %w", seg, ErrNotADirectory)
		}
		dir = childDir
	}

	return dir, nil
}

// Dir lists the entries of a directory within a snapshot. path is the
// slash-separated path from the snapshot root (empty string = root). The caller
// must supply a clean path with no ".." segments (see web.cleanBrowsePath).
func (m *Manager) Dir(ctx context.Context, ns, snapID, path string) ([]DirEntry, error) {
	dir, err := m.descendToDir(ctx, ns, snapID, path)
	if err != nil {
		return nil, err
	}

	entries, err := kopiafs.GetAllEntries(ctx, dir)
	if err != nil {
		return nil, fmt.Errorf("list entries: %w", err)
	}

	// Directories first, then alphabetically within each group.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})

	out := make([]DirEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, DirEntry{
			Name:    e.Name(),
			IsDir:   e.IsDir(),
			Size:    e.Size(),
			ModTime: e.ModTime(),
		})
	}
	return out, nil
}

// TarDir streams a plain tar archive of the directory subtree rooted at dirPath
// into w. dirPath is the slash-separated path from the snapshot root (empty = root).
// Returns ErrNotFound when any path segment is absent, ErrNotADirectory when
// the path resolves to a file rather than a directory.
func (m *Manager) TarDir(ctx context.Context, ns, snapID, dirPath string, w io.Writer) error {
	dir, err := m.descendToDir(ctx, ns, snapID, dirPath)
	if err != nil {
		return err
	}
	bw := bufio.NewWriterSize(w, 4<<20) // 4 MB — batch small tar writes into large TCP segments
	tw := tar.NewWriter(bw)
	buf := make([]byte, 1<<20) // 1 MB copy buffer, reused across all files
	if err := writeTarTree(ctx, tw, dir, "", buf); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return bw.Flush()
}

// writeTarTree recursively writes the directory dir into tw.
// prefix is the slash-separated path prefix prepended to each entry name.
// buf is a reusable copy buffer; must be at least a few KB for efficiency.
func writeTarTree(ctx context.Context, tw *tar.Writer, dir kopiafs.Directory, prefix string, buf []byte) error {
	return kopiafs.IterateEntries(ctx, dir, func(ctx context.Context, e kopiafs.Entry) error {
		name := e.Name()
		if prefix != "" {
			name = prefix + "/" + name
		}

		switch v := e.(type) {
		case kopiafs.Directory:
			hdr, err := tar.FileInfoHeader(e, "")
			if err != nil {
				return fmt.Errorf("tar header for dir %q: %w", name, err)
			}
			hdr.Name = name + "/"
			if err := tw.WriteHeader(hdr); err != nil {
				return fmt.Errorf("write tar header for dir %q: %w", name, err)
			}
			return writeTarTree(ctx, tw, v, name, buf)

		case kopiafs.File:
			hdr, err := tar.FileInfoHeader(e, "")
			if err != nil {
				return fmt.Errorf("tar header for file %q: %w", name, err)
			}
			hdr.Name = name
			hdr.Size = e.Size()
			if err := tw.WriteHeader(hdr); err != nil {
				return fmt.Errorf("write tar header for file %q: %w", name, err)
			}
			rc, err := v.Open(ctx)
			if err != nil {
				return fmt.Errorf("open file %q: %w", name, err)
			}
			_, copyErr := io.CopyBuffer(tw, rc, buf)
			closeErr := rc.Close()
			if copyErr != nil {
				return fmt.Errorf("copy file %q: %w", name, copyErr)
			}
			return closeErr

		case kopiafs.Symlink:
			link, err := v.Readlink(ctx)
			if err != nil {
				slog.Warn("kopia/tar: readlink failed, skipping", "name", name, "err", err)
				return nil
			}
			hdr := &tar.Header{
				Typeflag: tar.TypeSymlink,
				Name:     name,
				Linkname: link,
				ModTime:  e.ModTime(),
				Mode:     0o777,
			}
			if err := tw.WriteHeader(hdr); err != nil {
				return fmt.Errorf("write tar header for symlink %q: %w", name, err)
			}
			return nil

		default:
			// StreamingFile and other unknown types are uncommon in volume backups.
			slog.Warn("kopia/tar: skipping unknown entry type", "name", name, "type", fmt.Sprintf("%T", e))
			return nil
		}
	})
}

// OpenFile opens a single file within a snapshot for reading. path must be a
// slash-separated path from the snapshot root pointing at a regular file (not
// a directory, and not the empty root). The caller must Close the returned
// reader. Returns ErrNotFound when any path segment is absent, ErrNotAFile
// when the final segment is a directory.
func (m *Manager) OpenFile(ctx context.Context, ns, snapID, path string) (io.ReadSeekCloser, DirEntry, error) {
	if path == "" {
		return nil, DirEntry{}, fmt.Errorf("path is empty: %w", ErrNotAFile)
	}

	rep, err := m.open(ctx, ns)
	if err != nil {
		return nil, DirEntry{}, err
	}

	man, err := snapshot.LoadSnapshot(ctx, rep, manifest.ID(snapID))
	if err != nil {
		return nil, DirEntry{}, fmt.Errorf("load snapshot %q: %w", snapID, err)
	}

	root, err := snapshotfs.SnapshotRoot(rep, man)
	if err != nil {
		return nil, DirEntry{}, fmt.Errorf("snapshot root for %q: %w", snapID, err)
	}

	dir, ok := root.(kopiafs.Directory)
	if !ok {
		return nil, DirEntry{}, fmt.Errorf("snapshot root of %q is not a directory", snapID)
	}

	segs := strings.Split(path, "/")

	// Descend into every parent directory (all segments except the last).
	for _, seg := range segs[:len(segs)-1] {
		child, err := dir.Child(ctx, seg)
		if err != nil {
			if errors.Is(err, kopiafs.ErrEntryNotFound) {
				return nil, DirEntry{}, fmt.Errorf("navigate to %q: %w", seg, ErrNotFound)
			}
			return nil, DirEntry{}, fmt.Errorf("navigate to %q: %w", seg, err)
		}
		childDir, ok := child.(kopiafs.Directory)
		if !ok {
			return nil, DirEntry{}, fmt.Errorf("%q is not a directory: %w", seg, ErrNotFound)
		}
		dir = childDir
	}

	// Resolve the final segment.
	last := segs[len(segs)-1]
	child, err := dir.Child(ctx, last)
	if err != nil {
		if errors.Is(err, kopiafs.ErrEntryNotFound) {
			return nil, DirEntry{}, fmt.Errorf("open %q: %w", last, ErrNotFound)
		}
		return nil, DirEntry{}, fmt.Errorf("open %q: %w", last, err)
	}

	// Reject directories — those will be handled by M4's tar route.
	if child.IsDir() {
		return nil, DirEntry{}, fmt.Errorf("%q is a directory: %w", last, ErrNotAFile)
	}

	f, ok := child.(kopiafs.File)
	if !ok {
		return nil, DirEntry{}, fmt.Errorf("%q cannot be opened as a file", last)
	}

	rc, err := f.Open(ctx)
	if err != nil {
		return nil, DirEntry{}, fmt.Errorf("open file %q: %w", last, err)
	}

	entry := DirEntry{
		Name:    child.Name(),
		IsDir:   false,
		Size:    child.Size(),
		ModTime: child.ModTime(),
	}
	return rc, entry, nil
}

// retentionRoles converts raw kopia retention-reason strings (e.g. "latest-4",
// "monthly-2") into deduplicated, capitalised category names emitted in canonical
// order: Latest, Hourly, Daily, Weekly, Monthly, Annual. Any unrecognised
// category is appended after the canonical ones.
func retentionRoles(reasons []string) []string {
	// "latest" is omitted: Velero data-mover sets KeepLatest >= total count so
	// every snapshot carries it, making it indistinguishable noise in the UI.
	canonicalOrder := []string{"hourly", "daily", "weekly", "monthly", "annual"}
	seen := make(map[string]bool, len(reasons))
	for _, r := range reasons {
		// Each reason is "<category>-<n>"; strip the numeric suffix.
		cat := r
		if i := strings.LastIndex(r, "-"); i >= 0 {
			cat = r[:i]
		}
		seen[strings.ToLower(cat)] = true
	}
	var out []string
	for _, cat := range canonicalOrder {
		if seen[cat] {
			out = append(out, strings.ToUpper(cat[:1])+cat[1:])
			delete(seen, cat)
		}
	}
	// Append any unrecognised categories in sorted order.
	var extra []string
	for cat := range seen {
		extra = append(extra, strings.ToUpper(cat[:1])+cat[1:])
	}
	sort.Strings(extra)
	return append(out, extra...)
}

// Close closes every cached repository. Safe to call once at shutdown.
func (m *Manager) Close(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var firstErr error
	for ns, rep := range m.repos {
		if err := rep.Close(ctx); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("close repo %q: %w", ns, err)
		}
		delete(m.repos, ns)
	}
	return firstErr
}
