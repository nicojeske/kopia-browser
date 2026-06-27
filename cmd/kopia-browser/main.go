// Command kopia-browser starts the HTTP server that browses Velero/kopia
// backups on Garage S3. It wires config, the read-only kopia data layer, and
// the htmx UI.
package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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

	// Configure the default slog logger from LOG_LEVEL. All subsequent log calls
	// (in this process) use the leveled text handler on stderr.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel})))

	mgr, err := kopia.New(cfg)
	if err != nil {
		slog.Error("kopia init failed", "err", err)
		os.Exit(1)
	}

	// Background stats cache: refreshes periodically; handlers read from it
	// without blocking on S3. Cancelled on shutdown so the goroutine exits.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cache := kopia.NewStatsCache(mgr, cfg.StatsRefreshInterval, cfg.StatsCacheFile)
	go cache.Run(ctx)

	defer mgr.Close(context.Background())

	handler, err := web.NewServer(cfg, mgr, cache, assets.Templates(), assets.Static())
	if err != nil {
		slog.Error("server init failed", "err", err)
		os.Exit(1)
	}

	httpSrv := &http.Server{Addr: cfg.ListenAddr, Handler: handler}

	// Graceful shutdown on SIGINT / SIGTERM.
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		slog.Info("kopia-browser: shutting down")
		cancel()
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutCancel()
		if err := httpSrv.Shutdown(shutCtx); err != nil {
			slog.Error("shutdown error", "err", err)
		}
	}()

	slog.Info("kopia-browser listening", "addr", cfg.ListenAddr, "log_level", cfg.LogLevel, "stats_refresh", cfg.StatsRefreshInterval)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("listen failed", "err", err)
		os.Exit(1)
	}
}
