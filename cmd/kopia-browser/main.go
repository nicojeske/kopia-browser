// Command kopia-browser starts the HTTP server that browses Velero/kopia
// backups on Garage S3. It wires config, the read-only kopia data layer, and
// the htmx UI.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

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

	// Background stats cache: refreshes periodically; handlers read from it
	// without blocking on S3. Cancelled on shutdown so the goroutine exits.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cache := kopia.NewStatsCache(mgr, cfg.StatsRefreshInterval, cfg.StatsCacheFile)
	go cache.Run(ctx)

	defer mgr.Close(context.Background())

	srv, err := web.NewServer(cfg, mgr, cache, assets.Templates(), assets.Static())
	if err != nil {
		log.Fatalf("server: %v", err)
	}

	// Graceful shutdown on SIGINT / SIGTERM.
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		log.Println("kopia-browser: shutting down")
		cancel()
	}()

	log.Printf("kopia-browser listening on %s (stats refresh every %s)", cfg.ListenAddr, cfg.StatsRefreshInterval)
	if err := http.ListenAndServe(cfg.ListenAddr, srv); err != nil {
		log.Fatalf("listen: %v", err)
	}
}
