// Package web wires the HTTP routes and renders the html/template UI. It is a
// thin layer: handlers call the read-only data layer (the Backups interface)
// and render templates. It owns no kopia logic, so it can be tested against a
// fake Backups implementation.
package web

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"time"

	"github.com/nicojeske/kopia-browser/internal/config"
	"github.com/nicojeske/kopia-browser/internal/kopia"
)

// Backups is the read-only data layer the handlers depend on. *kopia.Manager
// implements it in production; tests supply an in-memory fake.
type Backups interface {
	ListNamespaces(ctx context.Context) ([]string, error)
	ListSnapshots(ctx context.Context, ns string) ([]kopia.SnapshotInfo, error)
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
	"humanBytes": humanBytes,
	"humanTime":  func(t time.Time) string { return t.Format("2006-01-02 15:04:05") },
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
