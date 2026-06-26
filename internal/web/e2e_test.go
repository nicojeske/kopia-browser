//go:build e2e

package web

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// E2E drives the real UI in headless Chrome against the fake data layer, so it
// is deterministic and needs no S3. Run: make e2e (requires Chrome/Chromium).
//
// It boots the full server on a random port, loads the namespace page, clicks a
// namespace, and asserts the snapshot table renders the expected backup.
func TestE2ENamespaceToSnapshots(t *testing.T) {
	srv := httptest.NewServer(newTestServer(t, sampleData()))
	defer srv.Close()

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.NoSandbox,
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()
	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	defer cancelCtx()
	ctx, cancelTimeout := context.WithTimeout(ctx, 30*time.Second)
	defer cancelTimeout()

	var tableText string
	err := chromedp.Run(ctx,
		chromedp.Navigate(srv.URL+"/"),
		chromedp.WaitVisible(`a[href="/repo/paperless"]`, chromedp.ByQuery),
		chromedp.Click(`a[href="/repo/paperless"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`table.snapshots`, chromedp.ByQuery),
		chromedp.Text(`table.snapshots`, &tableText, chromedp.ByQuery),
	)
	if err != nil {
		if isNoBrowser(err) {
			t.Skipf("no Chrome/Chromium available for E2E: %v", err)
		}
		t.Fatalf("chromedp run: %v", err)
	}

	if !strings.Contains(tableText, "velero-daily-20260626") {
		t.Errorf("snapshot table missing expected backup name; got:\n%s", tableText)
	}
}

func isNoBrowser(err error) bool {
	s := err.Error()
	return strings.Contains(s, "executable file not found") ||
		strings.Contains(s, "no chrome") ||
		strings.Contains(s, "exec:")
}
