// Package config loads and validates kopia-browser configuration from the
// environment (and a .env file in development). It performs no I/O against S3
// or kopia; it only assembles the typed Config used by the rest of the app.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration. See CLAUDE.md for the env var list.
type Config struct {
	S3Endpoint           string
	S3Region             string
	S3Bucket             string
	S3AccessKey          string
	S3SecretKey          string
	KopiaRepoPassword    string
	KopiaPrefix          string
	KopiaCacheDir        string
	ListenAddr           string
	StatsRefreshInterval time.Duration // how often the background stats cache refreshes
}

// Load reads configuration from the environment. In development a .env file in
// the working directory is loaded first if present (it is optional in prod).
// Required secrets have no defaults; missing ones are aggregated into a single
// error so the operator sees every problem at once.
func Load() (*Config, error) {
	// godotenv.Load only errors when an explicit file is missing; with no args
	// it silently does nothing if .env is absent. Ignore the error regardless:
	// real config comes from the environment in production.
	_ = godotenv.Load()

	refreshInterval, err := time.ParseDuration(getenv("STATS_REFRESH_INTERVAL", "15m"))
	if err != nil {
		refreshInterval = 15 * time.Minute
	}

	cfg := &Config{
		S3Endpoint:           os.Getenv("S3_ENDPOINT"),
		S3Region:             getenv("S3_REGION", "garage"),
		S3Bucket:             getenv("S3_BUCKET", "velero-backup"),
		S3AccessKey:          os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey:          os.Getenv("S3_SECRET_KEY"),
		KopiaRepoPassword:    os.Getenv("KOPIA_REPO_PASSWORD"),
		KopiaPrefix:          getenv("KOPIA_PREFIX", "kopia/"),
		KopiaCacheDir:        getenv("KOPIA_CACHE_DIR", ".kopia-cache"),
		ListenAddr:           getenv("LISTEN_ADDR", ":8080"),
		StatsRefreshInterval: refreshInterval,
	}

	var missing []string
	for _, r := range []struct {
		key string
		val string
	}{
		{"S3_ENDPOINT", cfg.S3Endpoint},
		{"S3_ACCESS_KEY", cfg.S3AccessKey},
		{"S3_SECRET_KEY", cfg.S3SecretKey},
		{"KOPIA_REPO_PASSWORD", cfg.KopiaRepoPassword},
	} {
		if r.val == "" {
			missing = append(missing, r.key)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	return cfg, nil
}

// getenv returns the env var for key, or def when it is unset or empty.
func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
