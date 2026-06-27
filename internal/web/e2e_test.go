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

// TestE2ENamespaceToVolumes checks that clicking a namespace shows the volume list.
func TestE2ENamespaceToVolumes(t *testing.T) {
	srv := httptest.NewServer(newTestServer(t, sampleData(), sampleStats()))
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
		chromedp.WaitVisible(`table.data-table`, chromedp.ByQuery),
		chromedp.Text(`table.data-table`, &tableText, chromedp.ByQuery),
	)
	if err != nil {
		if isNoBrowser(err) {
			t.Skipf("no Chrome/Chromium available for E2E: %v", err)
		}
		t.Fatalf("chromedp run: %v", err)
	}

	if !strings.Contains(tableText, "data-pvc") {
		t.Errorf("volumes table missing expected volume name; got:\n%s", tableText)
	}
}

// TestE2ENamespaceToSnapshots navigates namespace → volume → snapshot list.
func TestE2ENamespaceToSnapshots(t *testing.T) {
	srv := httptest.NewServer(newTestServer(t, sampleData(), sampleStats()))
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
		chromedp.WaitVisible(`table.data-table`, chromedp.ByQuery),
		chromedp.Click(`table.data-table a[href*="/vol/data-pvc"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`table.data-table`, chromedp.ByQuery),
		chromedp.Text(`table.data-table`, &tableText, chromedp.ByQuery),
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

// TestE2EBrowseDir navigates into a snapshot's directory tree and back.
func TestE2EBrowseDir(t *testing.T) {
	srv := httptest.NewServer(newTestServer(t, sampleData(), sampleStats()))
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

	var listingText, subText, urlAfterNav string
	err := chromedp.Run(ctx,
		// Navigate to namespace → volumes → snapshots.
		chromedp.Navigate(srv.URL+"/"),
		chromedp.WaitVisible(`a[href="/repo/paperless"]`, chromedp.ByQuery),
		chromedp.Click(`a[href="/repo/paperless"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`table.data-table`, chromedp.ByQuery),
		chromedp.Click(`table.data-table a[href*="/vol/data-pvc"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`table.data-table`, chromedp.ByQuery),

		// Click the snapshot browse link → root dir listing.
		chromedp.Click(`table.data-table a[href*="/browse/"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`table.data-table`, chromedp.ByQuery),
		chromedp.Text(`table.data-table`, &listingText, chromedp.ByQuery),

		// Click into the "data" subdir (htmx partial swap).
		// Wait for "documents" link — it only exists in data/, confirming the swap completed.
		chromedp.Click(`table.data-table a.entry-dir-link`, chromedp.ByQuery),
		chromedp.WaitVisible(`table.data-table a.entry-dir-link[href*="documents"]`, chromedp.ByQuery),
		chromedp.Text(`table.data-table`, &subText, chromedp.ByQuery),
		chromedp.Location(&urlAfterNav),
	)
	if err != nil {
		if isNoBrowser(err) {
			t.Skipf("no Chrome/Chromium available for E2E: %v", err)
		}
		t.Fatalf("chromedp run: %v", err)
	}

	if !strings.Contains(listingText, "data") {
		t.Errorf("root listing missing 'data' dir; got:\n%s", listingText)
	}
	if !strings.Contains(subText, "export.csv") {
		t.Errorf("subdir listing missing 'export.csv'; got:\n%s", subText)
	}
	if !strings.Contains(urlAfterNav, "/browse/") {
		t.Errorf("URL after nav = %q, expected to contain /browse/", urlAfterNav)
	}
}

// TestE2EDownloadLink checks that file entries in the browse listing have a
// download link pointing to the /download/ route (not hx-get driven).
func TestE2EDownloadLink(t *testing.T) {
	srv := httptest.NewServer(newTestServer(t, sampleData(), sampleStats()))
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

	var linkHref string
	err := chromedp.Run(ctx,
		// Navigate to the root browse listing.
		chromedp.Navigate(srv.URL+"/repo/paperless/snap/snap-1/browse/"),
		chromedp.WaitVisible(`table.data-table`, chromedp.ByQuery),
		// Get the href of the file download link.
		chromedp.AttributeValue(`table.data-table a.entry-file-link`, "href", &linkHref, nil, chromedp.ByQuery),
	)
	if err != nil {
		if isNoBrowser(err) {
			t.Skipf("no Chrome/Chromium available for E2E: %v", err)
		}
		t.Fatalf("chromedp run: %v", err)
	}

	if !strings.Contains(linkHref, "/download/") {
		t.Errorf("file entry link href = %q, expected to contain /download/", linkHref)
	}
}

// TestE2EFolderTarLink checks that directory rows have a tar download link
// and that the current-folder download button is present on the browse page.
func TestE2EFolderTarLink(t *testing.T) {
	srv := httptest.NewServer(newTestServer(t, sampleData(), sampleStats()))
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

	var tarLinkHref, folderBtnHref string
	err := chromedp.Run(ctx,
		chromedp.Navigate(srv.URL+"/repo/paperless/snap/snap-1/browse/"),
		chromedp.WaitVisible(`table.data-table`, chromedp.ByQuery),
		// tar link on the first dir row.
		chromedp.AttributeValue(`.btn-tar`, "href", &tarLinkHref, nil, chromedp.ByQuery),
		// current-folder download button.
		chromedp.AttributeValue(`.btn-download-folder`, "href", &folderBtnHref, nil, chromedp.ByQuery),
	)
	if err != nil {
		if isNoBrowser(err) {
			t.Skipf("no Chrome/Chromium available for E2E: %v", err)
		}
		t.Fatalf("chromedp run: %v", err)
	}

	if !strings.Contains(tarLinkHref, "/download/") {
		t.Errorf("dir tar link href = %q, expected to contain /download/", tarLinkHref)
	}
	if !strings.Contains(folderBtnHref, "/download/") {
		t.Errorf("folder download button href = %q, expected to contain /download/", folderBtnHref)
	}
}

// TestE2EDashboardSearch checks that the namespace search input filters cards.
func TestE2EDashboardSearch(t *testing.T) {
	srv := httptest.NewServer(newTestServer(t, sampleData(), sampleStats()))
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

	var giteaDisplay string
	err := chromedp.Run(ctx,
		chromedp.Navigate(srv.URL+"/"),
		chromedp.WaitVisible(`#ns-search`, chromedp.ByQuery),
		// Type "paperless" into the search box — "gitea" card should be hidden.
		chromedp.SendKeys(`#ns-search`, "paperless", chromedp.ByQuery),
		// Get computed display style of the gitea card; it should be "none".
		chromedp.Evaluate(`document.querySelector('.ns-card[data-name="gitea"]').style.display`, &giteaDisplay),
	)
	if err != nil {
		if isNoBrowser(err) {
			t.Skipf("no Chrome/Chromium available for E2E: %v", err)
		}
		t.Fatalf("chromedp run: %v", err)
	}

	if giteaDisplay != "none" {
		t.Errorf("gitea card display = %q after searching 'paperless', want 'none'", giteaDisplay)
	}
}

// TestE2EDashboardSortPill checks that clicking the Name sort pill reorders cards.
func TestE2EDashboardSortPill(t *testing.T) {
	srv := httptest.NewServer(newTestServer(t, sampleData(), sampleStats()))
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

	var firstCardName string
	err := chromedp.Run(ctx,
		chromedp.Navigate(srv.URL+"/"),
		chromedp.WaitVisible(`#sort-pills`, chromedp.ByQuery),
		// Click the "Name" sort pill (ascending alphabetical).
		chromedp.Click(`.sort-pill[data-sort="name"]`, chromedp.ByQuery),
		// First card should be "gitea" (alphabetically before "paperless").
		chromedp.Evaluate(`document.querySelector('#ns-grid .ns-card[data-name]').dataset.name`, &firstCardName),
	)
	if err != nil {
		if isNoBrowser(err) {
			t.Skipf("no Chrome/Chromium available for E2E: %v", err)
		}
		t.Fatalf("chromedp run: %v", err)
	}

	if firstCardName != "gitea" {
		t.Errorf("first card after Name sort = %q, want 'gitea'", firstCardName)
	}
}

func isNoBrowser(err error) bool {
	s := err.Error()
	return strings.Contains(s, "executable file not found") ||
		strings.Contains(s, "no chrome") ||
		strings.Contains(s, "exec:")
}
