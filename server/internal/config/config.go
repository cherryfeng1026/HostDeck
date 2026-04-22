package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	HTTPAddr                    string `yaml:"http_addr"`
	DBDSN                       string `yaml:"db_dsn"`
	BootstrapAdminUsername      string `yaml:"bootstrap_admin_username"`
	BootstrapAdminPassword      string `yaml:"bootstrap_admin_password"`
	BootstrapAdminToken         string `yaml:"bootstrap_admin_token"`
	MasterKey                   string `yaml:"master_key"`
	SessionCookieName           string `yaml:"session_cookie_name"`
	SessionCookieSecure         bool   `yaml:"session_cookie_secure"`
	SessionTTLHours             int    `yaml:"session_ttl_hours"`
	WebDistDir                  string `yaml:"web_dist_dir"`
	PollIntervalSeconds         int    `yaml:"poll_interval_seconds"`
	PollConcurrency             int    `yaml:"poll_concurrency"`
	CleanupIntervalSeconds      int    `yaml:"cleanup_interval_seconds"`
	StatusHistoryRetentionHours int    `yaml:"status_history_retention_hours"`
	CommandLogRetentionHours    int    `yaml:"command_log_retention_hours"`
	AlertHistoryRetentionHours  int    `yaml:"alert_history_retention_hours"`
	AuthEventRetentionHours     int    `yaml:"auth_event_retention_hours"`
	AuditEventRetentionHours    int    `yaml:"audit_event_retention_hours"`
	APITokenRetentionHours      int    `yaml:"api_token_retention_hours"`
	AlertWebhookURL             string `yaml:"alert_webhook_url"`
	AlertWebhookTimeoutSeconds  int    `yaml:"alert_webhook_timeout_seconds"`
}

func Load(configPath string) (Config, error) {
	cfg := defaultConfig()
	baseDir := ""

	if strings.TrimSpace(configPath) != "" {
		absConfigPath, err := filepath.Abs(configPath)
		if err != nil {
			return Config{}, fmt.Errorf("resolve config path: %w", err)
		}

		content, err := os.ReadFile(absConfigPath)
		if err != nil {
			return Config{}, fmt.Errorf("read config file: %w", err)
		}
		if err := yaml.Unmarshal(content, &cfg); err != nil {
			return Config{}, fmt.Errorf("decode config file: %w", err)
		}

		baseDir = filepath.Dir(absConfigPath)
	}

	if err := applyEnvOverrides(&cfg); err != nil {
		return Config{}, err
	}

	normalizeConfig(&cfg)
	if err := validateConfig(cfg); err != nil {
		return Config{}, err
	}

	if baseDir != "" {
		cfg.WebDistDir = resolveRelativePath(baseDir, cfg.WebDistDir)
		return cfg, nil
	}

	cfg.WebDistDir = filepath.Clean(cfg.WebDistDir)
	return cfg, nil
}

func LoadFromEnv() (Config, error) {
	return Load("")
}

func defaultConfig() Config {
	return Config{
		HTTPAddr:                    ":18080",
		SessionCookieName:           "hostdeck_session",
		SessionCookieSecure:         false,
		SessionTTLHours:             24,
		WebDistDir:                  "../web/dist",
		PollIntervalSeconds:         60,
		PollConcurrency:             5,
		CleanupIntervalSeconds:      3600,
		StatusHistoryRetentionHours: 24 * 30,
		CommandLogRetentionHours:    24 * 30,
		AlertHistoryRetentionHours:  24 * 90,
		AuthEventRetentionHours:     24 * 90,
		AuditEventRetentionHours:    24 * 90,
		APITokenRetentionHours:      24 * 30,
		AlertWebhookTimeoutSeconds:  5,
	}
}

func applyEnvOverrides(cfg *Config) error {
	if value := os.Getenv("HOSTDECK_ADDR"); value != "" {
		cfg.HTTPAddr = value
	}
	if value := os.Getenv("HOSTDECK_DB_DSN"); value != "" {
		cfg.DBDSN = value
	}
	if value := os.Getenv("DATABASE_URL"); value != "" {
		cfg.DBDSN = value
	}
	if value := os.Getenv("HOSTDECK_BOOTSTRAP_ADMIN_USERNAME"); value != "" {
		cfg.BootstrapAdminUsername = value
	}
	if value := os.Getenv("HOSTDECK_BOOTSTRAP_ADMIN_PASSWORD"); value != "" {
		cfg.BootstrapAdminPassword = value
	}
	if value := os.Getenv("HOSTDECK_BOOTSTRAP_ADMIN_TOKEN"); value != "" {
		cfg.BootstrapAdminToken = value
	}
	if value := os.Getenv("HOSTDECK_MASTER_KEY"); value != "" {
		cfg.MasterKey = value
	}
	if value := os.Getenv("HOSTDECK_SESSION_COOKIE_NAME"); value != "" {
		cfg.SessionCookieName = value
	}
	if value := os.Getenv("HOSTDECK_SESSION_COOKIE_SECURE"); value != "" {
		parsed, err := parseEnvBool("HOSTDECK_SESSION_COOKIE_SECURE", value)
		if err != nil {
			return err
		}
		cfg.SessionCookieSecure = parsed
	}
	if value := os.Getenv("HOSTDECK_WEB_DIST_DIR"); value != "" {
		cfg.WebDistDir = value
	}
	if value := os.Getenv("HOSTDECK_ALERT_WEBHOOK_URL"); value != "" {
		cfg.AlertWebhookURL = value
	}

	interval, err := parseEnvInt("HOSTDECK_POLL_INTERVAL_SECONDS", cfg.PollIntervalSeconds)
	if err != nil {
		return err
	}
	concurrency, err := parseEnvInt("HOSTDECK_POLL_CONCURRENCY", cfg.PollConcurrency)
	if err != nil {
		return err
	}
	sessionTTLHours, err := parseEnvInt("HOSTDECK_SESSION_TTL_HOURS", cfg.SessionTTLHours)
	if err != nil {
		return err
	}
	cleanupIntervalSeconds, err := parseEnvInt("HOSTDECK_CLEANUP_INTERVAL_SECONDS", cfg.CleanupIntervalSeconds)
	if err != nil {
		return err
	}
	statusHistoryRetentionHours, err := parseEnvInt("HOSTDECK_STATUS_HISTORY_RETENTION_HOURS", cfg.StatusHistoryRetentionHours)
	if err != nil {
		return err
	}
	commandLogRetentionHours, err := parseEnvInt("HOSTDECK_COMMAND_LOG_RETENTION_HOURS", cfg.CommandLogRetentionHours)
	if err != nil {
		return err
	}
	alertHistoryRetentionHours, err := parseEnvInt("HOSTDECK_ALERT_HISTORY_RETENTION_HOURS", cfg.AlertHistoryRetentionHours)
	if err != nil {
		return err
	}
	authEventRetentionHours, err := parseEnvInt("HOSTDECK_AUTH_EVENT_RETENTION_HOURS", cfg.AuthEventRetentionHours)
	if err != nil {
		return err
	}
	auditEventRetentionHours, err := parseEnvInt("HOSTDECK_AUDIT_EVENT_RETENTION_HOURS", cfg.AuditEventRetentionHours)
	if err != nil {
		return err
	}
	apiTokenRetentionHours, err := parseEnvInt("HOSTDECK_API_TOKEN_RETENTION_HOURS", cfg.APITokenRetentionHours)
	if err != nil {
		return err
	}
	alertWebhookTimeoutSeconds, err := parseEnvInt("HOSTDECK_ALERT_WEBHOOK_TIMEOUT_SECONDS", cfg.AlertWebhookTimeoutSeconds)
	if err != nil {
		return err
	}

	cfg.PollIntervalSeconds = interval
	cfg.PollConcurrency = concurrency
	cfg.SessionTTLHours = sessionTTLHours
	cfg.CleanupIntervalSeconds = cleanupIntervalSeconds
	cfg.StatusHistoryRetentionHours = statusHistoryRetentionHours
	cfg.CommandLogRetentionHours = commandLogRetentionHours
	cfg.AlertHistoryRetentionHours = alertHistoryRetentionHours
	cfg.AuthEventRetentionHours = authEventRetentionHours
	cfg.AuditEventRetentionHours = auditEventRetentionHours
	cfg.APITokenRetentionHours = apiTokenRetentionHours
	cfg.AlertWebhookTimeoutSeconds = alertWebhookTimeoutSeconds
	return nil
}

func normalizeConfig(cfg *Config) {
	if strings.TrimSpace(cfg.HTTPAddr) == "" {
		cfg.HTTPAddr = ":18080"
	}

	cfg.DBDSN = strings.TrimSpace(cfg.DBDSN)
	if strings.TrimSpace(cfg.SessionCookieName) == "" {
		cfg.SessionCookieName = "hostdeck_session"
	}
	if cfg.SessionTTLHours <= 0 {
		cfg.SessionTTLHours = 24
	}
	cfg.BootstrapAdminUsername = strings.TrimSpace(cfg.BootstrapAdminUsername)
	cfg.BootstrapAdminToken = strings.TrimSpace(cfg.BootstrapAdminToken)
	cfg.MasterKey = strings.TrimSpace(cfg.MasterKey)
	cfg.AlertWebhookURL = strings.TrimSpace(cfg.AlertWebhookURL)
	if strings.TrimSpace(cfg.WebDistDir) == "" {
		cfg.WebDistDir = "../web/dist"
	}
	if cfg.PollIntervalSeconds <= 0 {
		cfg.PollIntervalSeconds = 60
	}
	if cfg.PollConcurrency <= 0 {
		cfg.PollConcurrency = 5
	}
	if cfg.CleanupIntervalSeconds <= 0 {
		cfg.CleanupIntervalSeconds = 3600
	}
	if cfg.StatusHistoryRetentionHours <= 0 {
		cfg.StatusHistoryRetentionHours = 24 * 30
	}
	if cfg.CommandLogRetentionHours <= 0 {
		cfg.CommandLogRetentionHours = 24 * 30
	}
	if cfg.AlertHistoryRetentionHours <= 0 {
		cfg.AlertHistoryRetentionHours = 24 * 90
	}
	if cfg.AuthEventRetentionHours <= 0 {
		cfg.AuthEventRetentionHours = 24 * 90
	}
	if cfg.AuditEventRetentionHours <= 0 {
		cfg.AuditEventRetentionHours = 24 * 90
	}
	if cfg.APITokenRetentionHours <= 0 {
		cfg.APITokenRetentionHours = 24 * 30
	}
	if cfg.AlertWebhookTimeoutSeconds <= 0 {
		cfg.AlertWebhookTimeoutSeconds = 5
	}
}

func validateConfig(cfg Config) error {
	if cfg.DBDSN == "" {
		return errors.New("postgres dsn is required")
	}
	if cfg.AlertWebhookURL != "" && cfg.MasterKey == "" {
		return errors.New("master_key is required when alert webhook is configured")
	}
	return nil
}

func resolveRelativePath(baseDir string, value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return trimmed
	}
	if filepath.IsAbs(trimmed) {
		return filepath.Clean(trimmed)
	}
	return filepath.Clean(filepath.Join(baseDir, trimmed))
}

func parseEnvInt(key string, fallback int) (int, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}

func parseEnvBool(key string, value string) (bool, error) {
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}
