package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	assets "github.com/nicojeske/kopia-browser"
	"github.com/nicojeske/kopia-browser/internal/config"
	"github.com/nicojeske/kopia-browser/internal/kopia"
)

// newTestServer builds the real handler against a fake data layer, using the
// real embedded templates and static assets.
func newTestServer(t *testing.T, b Backups) http.Handler {
	t.Helper()
	srv, err := NewServer(&config.Config{}, b, assets.Templates(), assets.Static())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return srv
}

// uglySourcePath is the kind of Velero host-pod path that must never reach the UI.
const uglySourcePath = "/host_pods/abc/volumes/kubernetes.io~empty-dir/backup"

func sampleData() fakeBackups {
	now := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)
	return fakeBackups{
		namespaces: []string{"paperless", "gitea"},
		snapshots: map[string][]kopia.SnapshotInfo{
			"paperless": {
				{
					ID:         "snap-1",
					BackupName: "velero-daily-20260626",
					Volume:     "data-pvc",
					StartTime:  time.Date(2026, 6, 26, 1, 2, 3, 0, time.UTC),
					TotalSize:  3 * 1024 * 1024,
					FileCount:  42,
					Tags:       map[string]string{"backup": "velero-daily-20260626", "volume": "data-pvc", "path": uglySourcePath},
				},
				{
					ID:         "snap-2",
					BackupName: "velero-daily-20260625",
					Volume:     "config-pvc",
					StartTime:  time.Date(2026, 6, 25, 1, 2, 3, 0, time.UTC),
					TotalSize:  1 * 1024 * 1024,
					FileCount:  10,
					Tags:       map[string]string{"backup": "velero-daily-20260625", "volume": "config-pvc", "path": uglySourcePath},
				},
			},
		},
		dirs: map[string][]kopia.DirEntry{
			"snap-1|": {
				{Name: "data", IsDir: true, ModTime: now},
				{Name: "logs", IsDir: true, ModTime: now},
				{Name: "config.yaml", IsDir: false, Size: 1024, ModTime: now},
			},
			"snap-1|data": {
				{Name: "documents", IsDir: true, ModTime: now},
				{Name: "export.csv", IsDir: false, Size: 512 * 1024, ModTime: now},
			},
		},
		files: map[string][]byte{
			"snap-1|config.yaml":      []byte("key: value\n"),
			"snap-1|data/export.csv":  []byte("col1,col2\n1,2\n"),
		},
	}
}

func TestHandlers(t *testing.T) {
	tests := []struct {
		name        string
		target      string
		headers     map[string]string
		backups     Backups
		wantStatus  int
		wantContain []string
		wantAbsent  []string
		wantHeader  map[string]string // response header key → substring of expected value
	}{
		{
			name:        "index lists namespaces",
			target:      "/",
			backups:     sampleData(),
			wantStatus:  http.StatusOK,
			wantContain: []string{`href="/repo/paperless"`, `href="/repo/gitea"`, "Namespaces"},
		},
		{
			name:        "volumes page lists volume names, no source path",
			target:      "/repo/paperless",
			backups:     sampleData(),
			wantStatus:  http.StatusOK,
			wantContain: []string{"config-pvc", "data-pvc", "Namespaces"},
			wantAbsent:  []string{uglySourcePath, "host_pods"},
		},
		{
			name:       "snapshot list for a volume shows its backups",
			target:     "/repo/paperless/vol/data-pvc",
			backups:    sampleData(),
			wantStatus: http.StatusOK,
			wantContain: []string{
				"velero-daily-20260626",
				"2026-06-26 01:02:03",
				"3.0 MiB",
				"42",
				"data-pvc", // volume displayed in heading/breadcrumb
			},
			wantAbsent: []string{uglySourcePath, "host_pods", "velero-daily-20260625"},
		},
		{
			name:        "empty namespace renders empty state",
			target:      "/repo/empty",
			backups:     sampleData(),
			wantStatus:  http.StatusOK,
			wantContain: []string{"No snapshots"},
		},
		{
			name:        "data error yields 500",
			target:      "/",
			backups:     fakeBackups{err: errors.New("boom")},
			wantStatus:  http.StatusInternalServerError,
			wantContain: []string{"boom"},
		},
		{
			name:        "healthz",
			target:      "/healthz",
			backups:     sampleData(),
			wantStatus:  http.StatusOK,
			wantContain: []string{"ok"},
		},
		{
			name:       "browse root lists dirs and files",
			target:     "/repo/paperless/snap/snap-1/browse/",
			backups:    sampleData(),
			wantStatus: http.StatusOK,
			wantContain: []string{
				"data", "logs", "config.yaml",
				"Namespaces", // breadcrumb root link
				"paperless",  // breadcrumb ns link
			},
			wantAbsent: []string{uglySourcePath, "host_pods"},
		},
		{
			name:        "browse subdir lists its entries",
			target:      "/repo/paperless/snap/snap-1/browse/data",
			backups:     sampleData(),
			wantStatus:  http.StatusOK,
			wantContain: []string{"documents", "export.csv"},
		},
		{
			name:       "browse htmx request returns fragment without doctype",
			target:     "/repo/paperless/snap/snap-1/browse/",
			headers:    map[string]string{"HX-Request": "true"},
			backups:    sampleData(),
			wantStatus: http.StatusOK,
			wantAbsent: []string{"<!DOCTYPE"},
		},
		{
			name:        "browse full page has doctype",
			target:      "/repo/paperless/snap/snap-1/browse/",
			backups:     sampleData(),
			wantStatus:  http.StatusOK,
			wantContain: []string{"<!DOCTYPE"},
		},
		{
			name:       "browse data error yields 500",
			target:     "/repo/paperless/snap/snap-1/browse/",
			backups:    fakeBackups{err: errors.New("repo down")},
			wantStatus: http.StatusInternalServerError,
			wantContain: []string{"repo down"},
		},
		{
			name:       "browse root listing has download link for files",
			target:     "/repo/paperless/snap/snap-1/browse/",
			backups:    sampleData(),
			wantStatus: http.StatusOK,
			wantContain: []string{
				"/download/config.yaml", // file download link in root listing
			},
		},
		{
			name:       "browse subdir listing has download link for files",
			target:     "/repo/paperless/snap/snap-1/browse/data",
			backups:    sampleData(),
			wantStatus: http.StatusOK,
			wantContain: []string{
				"/download/data/export.csv", // file download link in data/ listing
			},
		},
		{
			name:       "download file success",
			target:     "/repo/paperless/snap/snap-1/download/config.yaml",
			backups:    sampleData(),
			wantStatus: http.StatusOK,
			wantContain: []string{"key: value"},
			wantHeader:  map[string]string{"Content-Disposition": "config.yaml"},
		},
		{
			name:       "download subdir file success",
			target:     "/repo/paperless/snap/snap-1/download/data/export.csv",
			backups:    sampleData(),
			wantStatus: http.StatusOK,
			wantContain: []string{"col1,col2"},
			wantHeader:  map[string]string{"Content-Disposition": "export.csv"},
		},
		{
			name:       "download root path yields whole-snapshot tar",
			target:     "/repo/paperless/snap/snap-1/download/",
			backups:    sampleData(),
			wantStatus: http.StatusOK,
			wantHeader: map[string]string{
				"Content-Type":        "application/x-tar",
				"Content-Disposition": "paperless.tar",
			},
		},
		{
			name:       "download folder path yields tar",
			target:     "/repo/paperless/snap/snap-1/download/data",
			backups:    sampleData(),
			wantStatus: http.StatusOK,
			wantHeader: map[string]string{
				"Content-Type":        "application/x-tar",
				"Content-Disposition": "data.tar",
			},
		},
		{
			name:       "download missing file yields 404",
			target:     "/repo/paperless/snap/snap-1/download/data/missing.txt",
			backups:    sampleData(),
			wantStatus: http.StatusNotFound,
		},
		{
			name:        "download data error yields 500",
			target:      "/repo/paperless/snap/snap-1/download/config.yaml",
			backups:     fakeBackups{err: errors.New("storage down")},
			wantStatus:  http.StatusInternalServerError,
			wantContain: []string{"storage down"},
		},
		{
			name:       "browse root listing has folder tar links",
			target:     "/repo/paperless/snap/snap-1/browse/",
			backups:    sampleData(),
			wantStatus: http.StatusOK,
			wantContain: []string{
				"/download/data",       // tar link for "data" dir row
				"btn-download-folder",  // current-folder download button
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newTestServer(t, tc.backups)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.target, nil)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			srv.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
			body := rec.Body.String()
			for _, want := range tc.wantContain {
				if !strings.Contains(body, want) {
					t.Errorf("body missing %q\n%s", want, body)
				}
			}
			for _, absent := range tc.wantAbsent {
				if strings.Contains(body, absent) {
					t.Errorf("body unexpectedly contains %q", absent)
				}
			}
			for k, v := range tc.wantHeader {
				got := rec.Header().Get(k)
				if !strings.Contains(got, v) {
					t.Errorf("header %q = %q, want to contain %q", k, got, v)
				}
			}
		})
	}
}

func TestCleanBrowsePath(t *testing.T) {
	tests := []struct {
		in        string
		wantClean string
		wantSegs  []string
	}{
		{in: "", wantClean: "", wantSegs: nil},
		{in: "data", wantClean: "data", wantSegs: []string{"data"}},
		{in: "data/sub", wantClean: "data/sub", wantSegs: []string{"data", "sub"}},
		{in: "..", wantClean: "", wantSegs: nil},           // resolves to root, safe
		{in: "a/../b", wantClean: "b", wantSegs: []string{"b"}}, // resolved
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			clean, segs, err := cleanBrowsePath(tc.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if clean != tc.wantClean {
				t.Errorf("clean = %q, want %q", clean, tc.wantClean)
			}
			if len(segs) != len(tc.wantSegs) {
				t.Errorf("segs = %v, want %v", segs, tc.wantSegs)
			}
		})
	}
}

func TestStaticServed(t *testing.T) {
	srv := newTestServer(t, sampleData())
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static/htmx.min.js", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("static htmx status = %d, want 200", rec.Code)
	}
}
