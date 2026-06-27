//go:build screenshots

package web

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// screenshotNoBrowser returns true when err indicates Chrome/Chromium is absent.
func screenshotNoBrowser(err error) bool {
	s := err.Error()
	return strings.Contains(s, "executable file not found") ||
		strings.Contains(s, "no chrome") ||
		strings.Contains(s, "exec:")
}

// TestCaptureScreenshots boots the server with fake data and captures full-page
// PNGs of the key routes into docs/screenshots/. Requires Chrome/Chromium.
// Run: make screenshots
func TestCaptureScreenshots(t *testing.T) {
	srv := httptest.NewServer(newTestServer(t, sampleData(), sampleStats()))
	defer srv.Close()

	outDir := filepath.Join("..", "..", "docs", "screenshots")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir docs/screenshots: %v", err)
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.NoSandbox,
		chromedp.WindowSize(1280, 900),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()
	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	defer cancelCtx()
	ctx, cancelTimeout := context.WithTimeout(ctx, 60*time.Second)
	defer cancelTimeout()

	captures := []struct {
		route   string
		wait    string
		file    string
	}{
		{
			route: srv.URL + "/",
			wait:  `.ns-card`,
			file:  "dashboard.png",
		},
		{
			route: srv.URL + "/repo/paperless",
			wait:  `table.data-table`,
			file:  "volumes.png",
		},
		{
			route: srv.URL + "/repo/paperless/vol/data-pvc",
			wait:  `table.data-table`,
			file:  "snapshots.png",
		},
		{
			route: srv.URL + "/repo/paperless/snap/snap-1/browse/",
			wait:  `table.data-table`,
			file:  "browse.png",
		},
	}

	for _, c := range captures {
		t.Run(c.file, func(t *testing.T) {
			var buf []byte
			err := chromedp.Run(ctx,
				chromedp.Navigate(c.route),
				chromedp.WaitVisible(c.wait, chromedp.ByQuery),
				chromedp.FullScreenshot(&buf, 90),
			)
			if err != nil {
				if screenshotNoBrowser(err) {
					t.Skipf("no Chrome/Chromium available: %v", err)
				}
				t.Fatalf("capture %s: %v", c.file, err)
			}
			path := filepath.Join(outDir, c.file)
			if err := os.WriteFile(path, buf, 0o644); err != nil {
				t.Fatalf("write %s: %v", path, err)
			}
			t.Logf("saved %s (%d bytes)", path, len(buf))
		})
	}
}
