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
	return fakeBackups{
		namespaces: []string{"paperless", "gitea"},
		snapshots: map[string][]kopia.SnapshotInfo{
			"paperless": {{
				ID:         "snap-1",
				BackupName: "velero-daily-20260626",
				StartTime:  time.Date(2026, 6, 26, 1, 2, 3, 0, time.UTC),
				TotalSize:  3 * 1024 * 1024,
				FileCount:  42,
				Tags:       map[string]string{"backup": "velero-daily-20260626", "path": uglySourcePath},
			}},
		},
	}
}

func TestHandlers(t *testing.T) {
	tests := []struct {
		name        string
		target      string
		backups     Backups
		wantStatus  int
		wantContain []string
		wantAbsent  []string
	}{
		{
			name:        "index lists namespaces",
			target:      "/",
			backups:     sampleData(),
			wantStatus:  http.StatusOK,
			wantContain: []string{`href="/repo/paperless"`, `href="/repo/gitea"`, "Namespaces"},
		},
		{
			name:       "snapshot table shows backup, human size, no source path",
			target:     "/repo/paperless",
			backups:    sampleData(),
			wantStatus: http.StatusOK,
			wantContain: []string{
				"velero-daily-20260626",
				"2026-06-26 01:02:03",
				"3.0 MiB",
				"42",
			},
			wantAbsent: []string{uglySourcePath, "host_pods"},
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
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newTestServer(t, tc.backups)
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.target, nil))

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
