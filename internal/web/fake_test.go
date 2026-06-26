package web

import (
	"context"

	"github.com/nicojeske/kopia-browser/internal/kopia"
)

// fakeBackups is an in-memory Backups implementation for handler + E2E tests.
// It needs no S3 or kopia, so tests run offline and deterministically.
type fakeBackups struct {
	namespaces []string
	snapshots  map[string][]kopia.SnapshotInfo
	err        error // when set, every method returns it
}

func (f fakeBackups) ListNamespaces(context.Context) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.namespaces, nil
}

func (f fakeBackups) ListSnapshots(_ context.Context, ns string) ([]kopia.SnapshotInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.snapshots[ns], nil
}
