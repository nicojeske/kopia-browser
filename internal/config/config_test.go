package config

import (
	"strings"
	"testing"
)

// setRequired sets the four required env vars to dummy values for a test.
func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("S3_ENDPOINT", "host:3900")
	t.Setenv("S3_ACCESS_KEY", "key")
	t.Setenv("S3_SECRET_KEY", "secret")
	t.Setenv("KOPIA_REPO_PASSWORD", "pw")
}

func TestLoadDefaults(t *testing.T) {
	setRequired(t)
	// Ensure optional vars are unset so defaults apply.
	t.Setenv("S3_REGION", "")
	t.Setenv("S3_BUCKET", "")
	t.Setenv("KOPIA_PREFIX", "")
	t.Setenv("LISTEN_ADDR", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	checks := map[string]struct{ got, want string }{
		"S3Region":    {cfg.S3Region, "garage"},
		"S3Bucket":    {cfg.S3Bucket, "velero-backup"},
		"KopiaPrefix": {cfg.KopiaPrefix, "kopia/"},
		"ListenAddr":  {cfg.ListenAddr, ":8080"},
		"S3Endpoint":  {cfg.S3Endpoint, "host:3900"},
	}
	for name, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", name, c.got, c.want)
		}
	}
}

func TestLoadOverrides(t *testing.T) {
	setRequired(t)
	t.Setenv("S3_REGION", "us-east-1")
	t.Setenv("LISTEN_ADDR", ":9090")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.S3Region != "us-east-1" {
		t.Errorf("S3Region = %q, want %q", cfg.S3Region, "us-east-1")
	}
	if cfg.ListenAddr != ":9090" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, ":9090")
	}
}

func TestLoadMissingRequired(t *testing.T) {
	// Only set two of four required vars.
	t.Setenv("S3_ENDPOINT", "host:3900")
	t.Setenv("S3_ACCESS_KEY", "key")
	t.Setenv("S3_SECRET_KEY", "")
	t.Setenv("KOPIA_REPO_PASSWORD", "")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want error for missing required vars")
	}
	for _, want := range []string{"S3_SECRET_KEY", "KOPIA_REPO_PASSWORD"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name missing var %q", err.Error(), want)
		}
	}
}
