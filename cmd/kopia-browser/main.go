// Command kopia-browser starts the HTTP server that browses Velero/kopia
// backups on Garage S3. It wires config, the read-only kopia data layer, and
// the htmx UI.
package main

import (
	"context"
	"log"
	"net/http"

	assets "github.com/nicojeske/kopia-browser"
	"github.com/nicojeske/kopia-browser/internal/config"
	"github.com/nicojeske/kopia-browser/internal/kopia"
	"github.com/nicojeske/kopia-browser/internal/web"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	mgr, err := kopia.New(cfg)
	if err != nil {
		log.Fatalf("kopia: %v", err)
	}
	defer mgr.Close(context.Background())

	srv, err := web.NewServer(cfg, mgr, assets.Templates(), assets.Static())
	if err != nil {
		log.Fatalf("server: %v", err)
	}

	log.Printf("kopia-browser listening on %s", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, srv); err != nil {
		log.Fatalf("listen: %v", err)
	}
}
