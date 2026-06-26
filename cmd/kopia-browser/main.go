// Command kopia-browser starts the HTTP server that browses Velero/kopia
// backups on Garage S3. M0 wires config + a hello page; kopia logic lands in
// later milestones.
package main

import (
	"log"
	"net/http"

	assets "github.com/nicojeske/kopia-browser"
	"github.com/nicojeske/kopia-browser/internal/config"
	"github.com/nicojeske/kopia-browser/internal/web"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	srv, err := web.NewServer(cfg, assets.Templates())
	if err != nil {
		log.Fatalf("server: %v", err)
	}

	log.Printf("kopia-browser listening on %s", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, srv); err != nil {
		log.Fatalf("listen: %v", err)
	}
}
