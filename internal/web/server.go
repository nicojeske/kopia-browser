// Package web wires the HTTP routes and renders the html/template UI. M0 serves
// only a health check and a hello page; later milestones add namespace/snapshot
// browsing and downloads.
package web

import (
	"html/template"
	"io/fs"
	"net/http"

	"github.com/nicojeske/kopia-browser/internal/config"
)

// server holds parsed templates and config for handlers.
type server struct {
	cfg *config.Config
	tpl *template.Template
}

// NewServer builds the HTTP handler. templates is the fs.FS containing the
// *.html files (see package assets). It parses templates once at startup.
func NewServer(cfg *config.Config, templates fs.FS) (http.Handler, error) {
	tpl, err := template.ParseFS(templates, "*.html")
	if err != nil {
		return nil, err
	}
	s := &server{cfg: cfg, tpl: tpl}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /{$}", s.handleIndex)
	return mux, nil
}

func (s *server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if err := s.tpl.ExecuteTemplate(w, "index.html", nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
