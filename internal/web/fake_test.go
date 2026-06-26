package web

import (
	"bytes"
	"context"
	"io"
	"strings"

	"github.com/nicojeske/kopia-browser/internal/kopia"
)

// fakeBackups is an in-memory Backups implementation for handler + E2E tests.
// It needs no S3 or kopia, so tests run offline and deterministically.
type fakeBackups struct {
	namespaces []string
	snapshots  map[string][]kopia.SnapshotInfo
	// dirs keys are "{snapID}|{path}" where path is empty string for root.
	dirs map[string][]kopia.DirEntry
	// files keys are "{snapID}|{path}" where path is the full clean slash-path.
	files map[string][]byte
	err   error // when set, every method returns it
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

func (f fakeBackups) Dir(_ context.Context, _, snapID, path string) ([]kopia.DirEntry, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.dirs[snapID+"|"+path], nil
}

// nopReadSeekCloser wraps *bytes.Reader to satisfy io.ReadSeekCloser.
type nopReadSeekCloser struct{ *bytes.Reader }

func (nopReadSeekCloser) Close() error { return nil }

func (f fakeBackups) OpenFile(_ context.Context, _, snapID, path string) (io.ReadSeekCloser, kopia.DirEntry, error) {
	if f.err != nil {
		return nil, kopia.DirEntry{}, f.err
	}
	data, ok := f.files[snapID+"|"+path]
	if !ok {
		return nil, kopia.DirEntry{}, kopia.ErrNotFound
	}
	// Derive the file name from the last path segment.
	name := path
	if i := strings.LastIndex(path, "/"); i >= 0 {
		name = path[i+1:]
	}
	entry := kopia.DirEntry{Name: name, Size: int64(len(data))}
	return nopReadSeekCloser{bytes.NewReader(data)}, entry, nil
}
