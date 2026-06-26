package kopia

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/kopia/kopia/repo"
	"github.com/kopia/kopia/repo/blob/s3"
	"github.com/kopia/kopia/repo/content"
	"github.com/kopia/kopia/snapshot"
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
	snapshot.SortByTime(mans, true) // reverse = newest first

	out := make([]SnapshotInfo, 0, len(mans))
	for _, man := range mans {
		out = append(out, SnapshotInfo{
			ID:         string(man.ID),
			BackupName: man.Tags["backup"],
			StartTime:  man.StartTime.ToTime(),
			EndTime:    man.EndTime.ToTime(),
			TotalSize:  man.Stats.TotalFileSize,
			FileCount:  int64(man.Stats.TotalFileCount),
			Tags:       man.Tags,
		})
	}
	return out, nil
}

// open returns the cached read-only repository for ns, opening it on first use.
func (m *Manager) open(ctx context.Context, ns string) (repo.Repository, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if rep, ok := m.repos[ns]; ok {
		return rep, nil
	}

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

	// Connect writes the config file; only do it once. On a persistent cache the
	// file survives restarts, so reuse it and just Open.
	if _, statErr := os.Stat(cfgPath); os.IsNotExist(statErr) {
		err = repo.Connect(ctx, cfgPath, st, m.cfg.KopiaRepoPassword, &repo.ConnectOptions{
			ClientOptions:  repo.ClientOptions{ReadOnly: true},
			CachingOptions: content.CachingOptions{CacheDirectory: filepath.Join(nsDir, "cache")},
		})
		if err != nil {
			return nil, fmt.Errorf("connect repo %q: %w", ns, err)
		}
	}

	rep, err := repo.Open(ctx, cfgPath, m.cfg.KopiaRepoPassword, &repo.Options{})
	if err != nil {
		return nil, fmt.Errorf("open repo %q: %w", ns, err)
	}

	m.repos[ns] = rep
	return rep, nil
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
