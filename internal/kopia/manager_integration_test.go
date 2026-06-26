//go:build integration

package kopia_test

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/nicojeske/kopia-browser/internal/config"
	"github.com/nicojeske/kopia-browser/internal/kopia"
)

// These tests hit the real Garage S3 backend. They skip themselves when creds
// are absent. They also serve to verify the facts flagged in docs/KOPIA.md
// (blob-path namespace shape, Velero tag/stat values).
//
// Run: make test-integration  (needs S3_* + KOPIA_REPO_PASSWORD in env/.env)

func testManager(t *testing.T) (*kopia.Manager, *config.Config) {
	t.Helper()
	if os.Getenv("S3_ENDPOINT") == "" {
		t.Skip("S3_ENDPOINT not set; skipping integration test")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Skipf("config not available: %v", err)
	}
	// Keep integration cache out of the repo dir.
	cfg.KopiaCacheDir = t.TempDir()
	mgr, err := kopia.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Close(context.Background()) })
	return mgr, cfg
}

func TestListNamespacesLive(t *testing.T) {
	mgr, _ := testManager(t)

	got, err := mgr.ListNamespaces(context.Background())
	if err != nil {
		t.Fatalf("ListNamespaces: %v", err)
	}
	t.Logf("namespaces: %v", got)

	if len(got) == 0 {
		t.Fatal("expected at least one namespace")
	}
	if !contains(got, "paperless") {
		t.Errorf("expected 'paperless' among namespaces, got %v", got)
	}
}

func TestListSnapshotsLive(t *testing.T) {
	mgr, _ := testManager(t)

	snaps, err := mgr.ListSnapshots(context.Background(), "paperless")
	if err != nil {
		t.Fatalf("ListSnapshots(paperless): %v", err)
	}
	if len(snaps) == 0 {
		t.Fatal("expected at least one snapshot in paperless")
	}

	s := snaps[0]
	t.Logf("newest snapshot: id=%s backup=%q start=%s size=%d files=%d tags=%v",
		s.ID, s.BackupName, s.StartTime, s.TotalSize, s.FileCount, s.Tags)

	if s.ID == "" {
		t.Error("snapshot ID is empty")
	}
	if s.StartTime.IsZero() {
		t.Error("snapshot StartTime is zero")
	}
}

func TestDirLive(t *testing.T) {
	mgr, _ := testManager(t)
	ctx := context.Background()

	snaps, err := mgr.ListSnapshots(ctx, "paperless")
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snaps) == 0 {
		t.Skip("no snapshots in paperless")
	}
	snapID := snaps[0].ID

	// Root listing.
	entries, err := mgr.Dir(ctx, "paperless", snapID, "")
	if err != nil {
		t.Fatalf("Dir(root): %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one entry at root")
	}
	t.Logf("root entries (%d): %v", len(entries), entryNames(entries))

	// Descend into first subdir.
	for _, e := range entries {
		if e.IsDir {
			sub, err := mgr.Dir(ctx, "paperless", snapID, e.Name)
			if err != nil {
				t.Fatalf("Dir(%q): %v", e.Name, err)
			}
			t.Logf("subdir %q entries (%d): %v", e.Name, len(sub), entryNames(sub))
			return
		}
	}
	t.Log("no subdirectory found at root")
}

func TestOpenFileLive(t *testing.T) {
	mgr, _ := testManager(t)
	ctx := context.Background()

	snaps, err := mgr.ListSnapshots(ctx, "paperless")
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snaps) == 0 {
		t.Skip("no snapshots in paperless")
	}
	snapID := snaps[0].ID

	// Find a file entry at the root (skip dirs).
	entries, err := mgr.Dir(ctx, "paperless", snapID, "")
	if err != nil {
		t.Fatalf("Dir(root): %v", err)
	}
	var fileEntry kopia.DirEntry
	for _, e := range entries {
		if !e.IsDir {
			fileEntry = e
			break
		}
	}
	if fileEntry.Name == "" {
		// No file at root — descend into the first subdir to find one.
		for _, e := range entries {
			if e.IsDir {
				sub, err := mgr.Dir(ctx, "paperless", snapID, e.Name)
				if err != nil {
					t.Fatalf("Dir(%q): %v", e.Name, err)
				}
				for _, se := range sub {
					if !se.IsDir {
						fileEntry = kopia.DirEntry{Name: e.Name + "/" + se.Name, IsDir: false, Size: se.Size}
						break
					}
				}
				if fileEntry.Name != "" {
					break
				}
			}
		}
	}
	if fileEntry.Name == "" {
		t.Skip("no file entry found in paperless snapshot")
	}

	t.Logf("opening file %q (size %d)", fileEntry.Name, fileEntry.Size)
	rc, entry, err := mgr.OpenFile(ctx, "paperless", snapID, fileEntry.Name)
	if err != nil {
		t.Fatalf("OpenFile(%q): %v", fileEntry.Name, err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read all: %v", err)
	}
	t.Logf("read %d bytes; entry.Name=%q entry.Size=%d", len(data), entry.Name, entry.Size)

	if len(data) == 0 {
		t.Error("read zero bytes from file")
	}
	if int64(len(data)) != entry.Size {
		t.Errorf("read %d bytes, expected entry.Size %d", len(data), entry.Size)
	}
}

func TestTarDirLive(t *testing.T) {
	mgr, _ := testManager(t)
	ctx := context.Background()

	snaps, err := mgr.ListSnapshots(ctx, "paperless")
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snaps) == 0 {
		t.Skip("no snapshots in paperless")
	}
	snapID := snaps[0].ID

	// Find a subdirectory to tar (fall back to root if none).
	entries, err := mgr.Dir(ctx, "paperless", snapID, "")
	if err != nil {
		t.Fatalf("Dir(root): %v", err)
	}
	var subDirName string
	for _, e := range entries {
		if e.IsDir {
			subDirName = e.Name
			break
		}
	}

	t.Logf("taring %q", subDirName)
	var buf bytes.Buffer
	if err := mgr.TarDir(ctx, "paperless", snapID, subDirName, &buf); err != nil {
		t.Fatalf("TarDir(%q): %v", subDirName, err)
	}

	tarBytes := buf.Len()
	t.Logf("tar size: %d bytes", tarBytes)
	if tarBytes == 0 {
		t.Fatal("tar output is empty")
	}

	// Parse the tar and count entries.
	tr := tar.NewReader(&buf)
	count := 0
	for {
		_, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar.Next: %v", err)
		}
		count++
	}
	t.Logf("tar entries: %d", count)
	if count == 0 {
		t.Error("tar has no entries")
	}
}

func entryNames(entries []kopia.DirEntry) []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name
		if e.IsDir {
			names[i] += "/"
		}
	}
	return names
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
