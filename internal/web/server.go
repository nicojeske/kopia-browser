// Package web wires the HTTP routes and renders the html/template UI. It is a
// thin layer: handlers call the read-only data layer (the Backups interface)
// and render templates. It owns no kopia logic, so it can be tested against a
// fake Backups implementation.
package web

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/nicojeske/kopia-browser/internal/config"
	"github.com/nicojeske/kopia-browser/internal/kopia"
)

// Backups is the read-only data layer the handlers depend on. *kopia.Manager
// implements it in production; tests supply an in-memory fake.
type Backups interface {
	ListNamespaces(ctx context.Context) ([]string, error)
	ListSnapshots(ctx context.Context, ns string) ([]kopia.SnapshotInfo, error)
	Dir(ctx context.Context, ns, snapID, path string) ([]kopia.DirEntry, error)
	// OpenFile returns a seekable stream for a single file within a snapshot
	// plus its metadata. Caller must Close the reader. Returns kopia.ErrNotAFile
	// when the path resolves to a directory (including empty path = root).
	OpenFile(ctx context.Context, ns, snapID, path string) (io.ReadSeekCloser, kopia.DirEntry, error)
	// TarDir streams a plain tar archive of the directory subtree rooted at path
	// (empty = snapshot root) into w.
	TarDir(ctx context.Context, ns, snapID, path string, w io.Writer) error
}

// crumb is one level of the directory breadcrumb.
type crumb struct {
	Name string
	Href string // empty = current location (rendered as plain text, not a link)
}

// server holds parsed templates, config and the data layer for handlers.
type server struct {
	cfg     *config.Config
	backups Backups
	tpl     *template.Template
}

// NewServer builds the HTTP handler. templates holds the *.html files and
// static holds the embedded JS/CSS (see package assets). Templates are parsed
// once at startup.
func NewServer(cfg *config.Config, backups Backups, templates, static fs.FS) (http.Handler, error) {
	tpl, err := template.New("").Funcs(templateFuncs).ParseFS(templates, "*.html")
	if err != nil {
		return nil, err
	}
	s := &server{cfg: cfg, backups: backups, tpl: tpl}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /repo/{ns}", s.handleSnapshots)
	mux.HandleFunc("GET /repo/{ns}/snap/{id}/browse/{path...}", s.handleBrowse)
	mux.HandleFunc("GET /repo/{ns}/snap/{id}/download/{path...}", s.handleDownload)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(static))))
	return mux, nil
}

func (s *server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}

// handleIndex lists the namespaces (one kopia repo each).
func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	namespaces, err := s.backups.ListNamespaces(r.Context())
	if err != nil {
		s.renderError(w, http.StatusInternalServerError, "listing namespaces", err)
		return
	}
	s.render(w, "namespaces.html", map[string]any{"Title": "Namespaces", "Namespaces": namespaces})
}

// handleSnapshots lists the snapshots of one namespace.
func (s *server) handleSnapshots(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("ns")
	snaps, err := s.backups.ListSnapshots(r.Context(), ns)
	if err != nil {
		s.renderError(w, http.StatusInternalServerError, "listing snapshots", err)
		return
	}
	s.render(w, "snapshots.html", map[string]any{"Title": ns, "Namespace": ns, "Snapshots": snaps})
}

// handleBrowse renders the directory listing for a path inside a snapshot.
// When the request carries an HX-Request header (htmx), only the inner
// browse-content fragment is returned so htmx can swap it in-place.
func (s *server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("ns")
	snapID := r.PathValue("id")
	rawPath := r.PathValue("path")

	cleanPath, segs, err := cleanBrowsePath(rawPath)
	if err != nil {
		s.renderError(w, http.StatusBadRequest, "invalid path", err)
		return
	}

	entries, err := s.backups.Dir(r.Context(), ns, snapID, cleanPath)
	if err != nil {
		s.renderError(w, http.StatusInternalServerError, "listing directory", err)
		return
	}

	// BrowseBase is the URL prefix used by dir-entry links in the template.
	browseBase := fmt.Sprintf("/repo/%s/snap/%s/browse", ns, snapID)
	if cleanPath != "" {
		browseBase += "/" + cleanPath
	}

	// DownloadBase is the URL prefix used by file-entry download links.
	downloadBase := fmt.Sprintf("/repo/%s/snap/%s/download", ns, snapID)
	if cleanPath != "" {
		downloadBase += "/" + cleanPath
	}

	// Build breadcrumb: Namespaces → ns → snap → seg1 → seg2 …
	snapDisplay := snapID
	if len(snapDisplay) > 8 {
		snapDisplay = snapDisplay[:8]
	}
	rootHref := fmt.Sprintf("/repo/%s/snap/%s/browse/", ns, snapID)
	crumbs := []crumb{
		{Name: "Namespaces", Href: "/"},
		{Name: ns, Href: fmt.Sprintf("/repo/%s", ns)},
	}
	if len(segs) == 0 {
		crumbs = append(crumbs, crumb{Name: snapDisplay}) // current, no link
	} else {
		crumbs = append(crumbs, crumb{Name: snapDisplay, Href: rootHref})
		acc := ""
		for i, seg := range segs {
			if acc != "" {
				acc += "/"
			}
			acc += seg
			href := ""
			if i < len(segs)-1 {
				href = fmt.Sprintf("/repo/%s/snap/%s/browse/%s", ns, snapID, acc)
			}
			crumbs = append(crumbs, crumb{Name: seg, Href: href})
		}
	}

	title := ns + " / " + snapDisplay
	if cleanPath != "" {
		title += " / " + cleanPath
	}
	data := map[string]any{
		"Title":        title,
		"Namespace":    ns,
		"SnapID":       snapID,
		"Path":         cleanPath,
		"Crumbs":       crumbs,
		"Entries":      entries,
		"BrowseBase":   browseBase,
		"DownloadBase": downloadBase,
	}

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := s.tpl.ExecuteTemplate(w, "browse-content", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	s.render(w, "browse.html", data)
}

// handleDownload serves a snapshot entry as a download.
//   - Regular file: Content-Disposition + http.ServeContent (Range / sniffed Content-Type).
//   - Directory (incl. empty path = snapshot root): plain tar archive streamed directly.
//
// The same route handles both cases; the distinction is made by calling OpenFile
// first — ErrNotAFile signals a directory and triggers the tar path.
func (s *server) handleDownload(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("ns")
	snapID := r.PathValue("id")
	rawPath := r.PathValue("path")

	cleanPath, _, err := cleanBrowsePath(rawPath)
	if err != nil {
		s.renderError(w, http.StatusBadRequest, "invalid path", err)
		return
	}
	// Empty cleanPath is valid: it means "download the entire snapshot root as tar".
	// OpenFile returns ErrNotAFile for any directory (including root), which
	// triggers the tar branch below.

	rc, entry, fileErr := s.backups.OpenFile(r.Context(), ns, snapID, cleanPath)
	switch {
	case fileErr == nil:
		// Regular file: serve with full HTTP machinery (Range, Content-Type, etc.).
		defer rc.Close()
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, entry.Name))
		http.ServeContent(w, r, entry.Name, entry.ModTime, rc)

	case errors.Is(fileErr, kopia.ErrNotAFile):
		// Directory: stream a plain tar archive.
		base := path.Base(cleanPath)
		if cleanPath == "" {
			base = ns // root download → "<namespace>.tar"
		}
		tarName := base + ".tar"
		w.Header().Set("Content-Type", "application/x-tar")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, tarName))
		if terr := s.backups.TarDir(r.Context(), ns, snapID, cleanPath, w); terr != nil {
			// Headers (and possibly some bytes) are already sent; cannot change status.
			log.Printf("handleDownload: tar %s/%s/%s: %v", ns, snapID, cleanPath, terr)
		}

	case errors.Is(fileErr, kopia.ErrNotFound):
		s.renderError(w, http.StatusNotFound, "not found", fileErr)

	default:
		s.renderError(w, http.StatusInternalServerError, "opening file", fileErr)
	}
}

// cleanBrowsePath normalises a user-supplied path value from the URL.
// It returns the clean path (empty = root) and the non-empty path segments.
// ".." segments are resolved away by rooting the path; any remaining invalid
// segment (empty, ".", "..") is rejected to guard against traversal.
func cleanBrowsePath(raw string) (string, []string, error) {
	if raw == "" {
		return "", nil, nil
	}
	// Prefix "/" so path.Clean treats it as absolute — prevents ".." escaping root.
	cleaned := strings.TrimPrefix(path.Clean("/"+raw), "/")
	if cleaned == "" || cleaned == "." {
		return "", nil, nil
	}
	segs := strings.Split(cleaned, "/")
	for _, seg := range segs {
		if seg == ".." || seg == "." || seg == "" {
			return "", nil, fmt.Errorf("invalid path segment %q", seg)
		}
	}
	return cleaned, segs, nil
}

func (s *server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *server) renderError(w http.ResponseWriter, code int, what string, err error) {
	http.Error(w, fmt.Sprintf("%s: %v", what, err), code)
}

// templateFuncs are helpers available in all templates.
var templateFuncs = template.FuncMap{
	"humanBytes":    humanBytes,
	"humanTime":     func(t time.Time) string { return t.Format("2006-01-02 15:04:05") },
	"urlPathEscape": url.PathEscape,
}

// humanBytes renders a byte count as a human-readable size.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
