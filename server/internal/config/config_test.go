package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hostdeck/server/internal/config"
)

func TestLoad_UsesConfigFileAndEnvOverride(t *testing.T) {
	t.Setenv("HOSTDECK_ADDR", ":18080")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(configPath, []byte("http_addr: \":9090\"\ndb_dsn: \"postgresql://config-user:config-pass@localhost:5432/configdb?sslmode=require\"\nmaster_key: \"test-master-key\"\nweb_dist_dir: \"../web/dist\"\npoll_interval_seconds: 30\npoll_concurrency: 3\ncleanup_interval_seconds: 600\nstatus_history_retention_hours: 48\ncommand_log_retention_hours: 72\nalert_history_retention_hours: 96\nauth_event_retention_hours: 120\naudit_event_retention_hours: 144\napi_token_retention_hours: 168\nalert_webhook_url: \"https://hooks.example.test/alerts\"\nalert_webhook_timeout_seconds: 9\n"), 0o644)
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
	if cfg.DBDSN != "postgresql://config-user:config-pass@localhost:5432/configdb?sslmode=require" {
		t.Fatalf("expected config dsn, got %q", cfg.DBDSN)
	}
	expectedWebDistDir := filepath.Clean(filepath.Join(dir, "../web/dist"))
	if cfg.WebDistDir != expectedWebDistDir {
		t.Fatalf("expected web dist dir %q, got %q", expectedWebDistDir, cfg.WebDistDir)
	}
	if cfg.CleanupIntervalSeconds != 600 {
		t.Fatalf("expected cleanup interval 600, got %d", cfg.CleanupIntervalSeconds)
	}
	if cfg.StatusHistoryRetentionHours != 48 || cfg.CommandLogRetentionHours != 72 || cfg.AlertHistoryRetentionHours != 96 {
		t.Fatalf("unexpected retention settings: %+v", cfg)
	}
	if cfg.AuthEventRetentionHours != 120 || cfg.AuditEventRetentionHours != 144 || cfg.APITokenRetentionHours != 168 {
		t.Fatalf("unexpected event retention settings: %+v", cfg)
	}
	if cfg.AlertWebhookURL != "https://hooks.example.test/alerts" || cfg.AlertWebhookTimeoutSeconds != 9 {
		t.Fatalf("unexpected webhook settings: %+v", cfg)
	}
}

func TestLoad_UsesPostgresDSNFromEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://user:pass@localhost:5432/hostdeck?sslmode=require")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(configPath, []byte("db_dsn: \"postgresql://config-user:config-pass@localhost:5432/configdb?sslmode=require\"\nweb_dist_dir: \"../web/dist\"\n"), 0o644)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.DBDSN != "postgresql://user:pass@localhost:5432/hostdeck?sslmode=require" {
		t.Fatalf("expected env override dsn, got %q", cfg.DBDSN)
	}
}

func TestLoad_RequiresPostgresDSN(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(configPath, []byte("web_dist_dir: \"../web/dist\"\n"), 0o644)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err = config.Load(configPath)
	if err == nil {
		t.Fatal("expected missing dsn error")
	}
	if !strings.Contains(err.Error(), "postgres dsn is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoad_RequiresMasterKeyWhenAlertWebhookConfigured(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(configPath, []byte("db_dsn: \"postgresql://config-user:config-pass@localhost:5432/configdb?sslmode=require\"\nalert_webhook_url: \"https://hooks.example.test/alerts\"\n"), 0o644)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err = config.Load(configPath)
	if err == nil {
		t.Fatal("expected missing master key error")
	}
	if !strings.Contains(err.Error(), "master_key is required when alert webhook is configured") {
		t.Fatalf("unexpected error: %v", err)
	}
}
