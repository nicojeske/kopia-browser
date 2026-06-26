package kopia

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"os"
	"testing"
	"time"

	kopiafs "github.com/kopia/kopia/fs"
)

// --- in-memory fakes for kopiafs.Entry, Directory, File, Reader ---

// fakeEntry is the shared base; implements kopiafs.Entry via os.FileInfo plus extras.
type fakeEntry struct {
	name    string
	size    int64
	modTime time.Time
	isDir   bool
}

func (e fakeEntry) Name() string       { return e.name }
func (e fakeEntry) Size() int64        { return e.size }
func (e fakeEntry) Mode() os.FileMode  {
	if e.isDir {
		return os.ModeDir | 0o755
	}
	return 0o644
}
func (e fakeEntry) ModTime() time.Time          { return e.modTime }
func (e fakeEntry) IsDir() bool                 { return e.isDir }
func (e fakeEntry) Sys() any                    { return nil }
func (e fakeEntry) Owner() kopiafs.OwnerInfo    { return kopiafs.OwnerInfo{} }
func (e fakeEntry) Device() kopiafs.DeviceInfo  { return kopiafs.DeviceInfo{} }
func (e fakeEntry) LocalFilesystemPath() string { return "" }
func (e fakeEntry) Close()                      {}

// fakeDir implements kopiafs.Directory.
type fakeDir struct {
	fakeEntry
	children []kopiafs.Entry
}

func (d *fakeDir) Child(_ context.Context, name string) (kopiafs.Entry, error) {
	for _, c := range d.children {
		if c.Name() == name {
			return c, nil
		}
	}
	return nil, kopiafs.ErrEntryNotFound
}

func (d *fakeDir) Iterate(_ context.Context) (kopiafs.DirectoryIterator, error) {
	return &fakeIter{entries: d.children}, nil
}

func (d *fakeDir) SupportsMultipleIterations() bool { return true }

// fakeIter implements kopiafs.DirectoryIterator.
type fakeIter struct {
	entries []kopiafs.Entry
	pos     int
}

func (it *fakeIter) Next(_ context.Context) (kopiafs.Entry, error) {
	if it.pos >= len(it.entries) {
		return nil, nil
	}
	e := it.entries[it.pos]
	it.pos++
	return e, nil
}

func (it *fakeIter) Close() {}

// fakeFile implements kopiafs.File.
type fakeFile struct {
	fakeEntry
	data []byte
}

func (f *fakeFile) Open(_ context.Context) (kopiafs.Reader, error) {
	return &fakeReader{Reader: bytes.NewReader(f.data)}, nil
}

// fakeReader implements kopiafs.Reader (ReadCloser + Seeker + Entry()).
type fakeReader struct {
	*bytes.Reader
}

func (r *fakeReader) Close() error                      { return nil }
func (r *fakeReader) Entry() (kopiafs.Entry, error)     { return nil, nil }

// --- tests ---

func TestWriteTarTree(t *testing.T) {
	now := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)

	// Tree:
	//   (root)
	//     file.txt       ("hello")
	//     sub/
	//       nested.txt   ("nested content")
	nestedFile := &fakeFile{
		fakeEntry: fakeEntry{name: "nested.txt", size: 14, modTime: now},
		data:      []byte("nested content"),
	}
	subDir := &fakeDir{
		fakeEntry: fakeEntry{name: "sub", isDir: true, modTime: now},
		children:  []kopiafs.Entry{nestedFile},
	}
	rootFile := &fakeFile{
		fakeEntry: fakeEntry{name: "file.txt", size: 5, modTime: now},
		data:      []byte("hello"),
	}
	root := &fakeDir{
		fakeEntry: fakeEntry{name: "", isDir: true, modTime: now},
		children:  []kopiafs.Entry{rootFile, subDir},
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := writeTarTree(context.Background(), tw, root, ""); err != nil {
		t.Fatalf("writeTarTree: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tw.Close: %v", err)
	}

	got := readTarEntries(t, &buf)

	want := map[string]string{
		"file.txt":       "hello",
		"sub/":           "",
		"sub/nested.txt": "nested content",
	}
	if len(got) != len(want) {
		t.Fatalf("tar has %d entries, want %d; entries: %v", len(got), len(want), tarKeys(got))
	}
	for name, content := range want {
		actual, ok := got[name]
		if !ok {
			t.Errorf("missing tar entry %q", name)
			continue
		}
		if actual != content {
			t.Errorf("entry %q: got content %q, want %q", name, actual, content)
		}
	}
}

// readTarEntries reads all entries from a tar stream.
// Keys are entry names; values are file content (empty string for dirs).
func readTarEntries(t *testing.T, r io.Reader) map[string]string {
	t.Helper()
	entries := map[string]string{}
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar.Next: %v", err)
		}
		if hdr.Typeflag == tar.TypeReg {
			data, err := io.ReadAll(tr)
			if err != nil {
				t.Fatalf("read tar entry %q: %v", hdr.Name, err)
			}
			entries[hdr.Name] = string(data)
		} else {
			entries[hdr.Name] = ""
		}
	}
	return entries
}

func tarKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
