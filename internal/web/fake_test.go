package web

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
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
	// A path that matches a dirs key is a directory — not a file.
	if _, isDir := f.dirs[snapID+"|"+path]; isDir {
		return nil, kopia.DirEntry{}, fmt.Errorf("%q is a directory: %w", path, kopia.ErrNotAFile)
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

// TarDir writes a plain tar of the directory subtree into w by recursively
// walking the in-memory dirs/files maps.
func (f fakeBackups) TarDir(_ context.Context, _, snapID, dirPath string, w io.Writer) error {
	if f.err != nil {
		return f.err
	}
	tw := tar.NewWriter(w)
	if err := f.tarDir(tw, snapID, dirPath, ""); err != nil {
		return err
	}
	return tw.Close()
}

func (f fakeBackups) tarDir(tw *tar.Writer, snapID, dirPath, prefix string) error {
	key := snapID + "|" + dirPath
	for _, e := range f.dirs[key] {
		var fullPath string
		if dirPath == "" {
			fullPath = e.Name
		} else {
			fullPath = dirPath + "/" + e.Name
		}
		tarName := e.Name
		if prefix != "" {
			tarName = prefix + "/" + e.Name
		}
		if e.IsDir {
			hdr := &tar.Header{
				Typeflag: tar.TypeDir,
				Name:     tarName + "/",
				ModTime:  e.ModTime,
				Mode:     0o755,
			}
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
			if err := f.tarDir(tw, snapID, fullPath, tarName); err != nil {
				return err
			}
		} else {
			data := f.files[snapID+"|"+fullPath]
			hdr := &tar.Header{
				Typeflag: tar.TypeReg,
				Name:     tarName,
				Size:     int64(len(data)),
				ModTime:  e.ModTime,
				Mode:     0o644,
			}
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
			if _, err := tw.Write(data); err != nil {
				return err
			}
		}
	}
	return nil
}
