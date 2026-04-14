package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"hostdeck/server/internal/config"
)

func TestLoad_UsesConfigFileAndEnvOverride(t *testing.T) {
	t.Setenv("HOSTDECK_ADDR", ":18080")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(configPath, []byte("http_addr: \":9090\"\ndb_driver: \"sqlite\"\ndb_path: \"./data/app.db\"\nweb_dist_dir: \"../web/dist\"\npoll_interval_seconds: 30\npoll_concurrency: 3\n"), 0o644)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.HTTPAddr != ":18080" {
		t.Fatalf("expected env override addr, got %q", cfg.HTTPAddr)
	}
	if cfg.DBDriver != "sqlite" {
		t.Fatalf("expected db driver sqlite, got %q", cfg.DBDriver)
	}
	expectedDBPath := filepath.Join(dir, "data", "app.db")
	if cfg.DBPath != expectedDBPath {
		t.Fatalf("expected db path %q, got %q", expectedDBPath, cfg.DBPath)
	}
	expectedWebDistDir := filepath.Clean(filepath.Join(dir, "../web/dist"))
	if cfg.WebDistDir != expectedWebDistDir {
		t.Fatalf("expected web dist dir %q, got %q", expectedWebDistDir, cfg.WebDistDir)
	}
}

func TestLoad_UsesPostgresDSNFromEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://user:pass@localhost:5432/hostdeck?sslmode=require")
	t.Setenv("HOSTDECK_DB_DRIVER", "postgres")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(configPath, []byte("db_driver: \"postgres\"\ndb_dsn: \"postgresql://config-user:config-pass@localhost:5432/configdb?sslmode=require\"\nweb_dist_dir: \"../web/dist\"\n"), 0o644)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.DBDriver != "postgres" {
		t.Fatalf("expected db driver postgres, got %q", cfg.DBDriver)
	}
	if cfg.DBDSN != "postgresql://user:pass@localhost:5432/hostdeck?sslmode=require" {
		t.Fatalf("expected env override dsn, got %q", cfg.DBDSN)
	}
	if cfg.DBPath != "" {
		t.Fatalf("expected empty sqlite path for postgres config, got %q", cfg.DBPath)
	}
}
