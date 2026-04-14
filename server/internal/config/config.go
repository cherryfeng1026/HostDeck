package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	HTTPAddr               string `yaml:"http_addr"`
	DBDriver               string `yaml:"db_driver"`
	DBPath                 string `yaml:"db_path"`
	DBDSN                  string `yaml:"db_dsn"`
	BootstrapAdminUsername string `yaml:"bootstrap_admin_username"`
	BootstrapAdminPassword string `yaml:"bootstrap_admin_password"`
	BootstrapAdminToken    string `yaml:"bootstrap_admin_token"`
	MasterKey              string `yaml:"master_key"`
	SessionCookieName      string `yaml:"session_cookie_name"`
	SessionCookieSecure    bool   `yaml:"session_cookie_secure"`
	SessionTTLHours        int    `yaml:"session_ttl_hours"`
	WebDistDir             string `yaml:"web_dist_dir"`
	PollIntervalSeconds    int    `yaml:"poll_interval_seconds"`
	PollConcurrency        int    `yaml:"poll_concurrency"`
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
	if baseDir != "" {
		if cfg.DBDriver == "sqlite" {
			cfg.DBPath = resolveRelativePath(baseDir, cfg.DBPath)
		}
		cfg.WebDistDir = resolveRelativePath(baseDir, cfg.WebDistDir)
		return cfg, nil
	}

	if cfg.DBDriver == "sqlite" {
		cfg.DBPath = filepath.Clean(cfg.DBPath)
	}
	cfg.WebDistDir = filepath.Clean(cfg.WebDistDir)
	return cfg, nil
}

func LoadFromEnv() (Config, error) {
	return Load("")
}

func defaultConfig() Config {
	return Config{
		HTTPAddr:            ":18080",
		DBDriver:            "sqlite",
		DBPath:              "./data/app.db",
		SessionCookieName:   "hostdeck_session",
		SessionCookieSecure: false,
		SessionTTLHours:     24,
		WebDistDir:          "../web/dist",
		PollIntervalSeconds: 60,
		PollConcurrency:     5,
	}
}

func applyEnvOverrides(cfg *Config) error {
	if value := os.Getenv("HOSTDECK_ADDR"); value != "" {
		cfg.HTTPAddr = value
	}
	if value := os.Getenv("HOSTDECK_DB_DRIVER"); value != "" {
		cfg.DBDriver = value
	}
	if value := os.Getenv("HOSTDECK_DB_PATH"); value != "" {
		cfg.DBPath = value
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

	cfg.PollIntervalSeconds = interval
	cfg.PollConcurrency = concurrency
	cfg.SessionTTLHours = sessionTTLHours
	return nil
}

func normalizeConfig(cfg *Config) {
	if strings.TrimSpace(cfg.HTTPAddr) == "" {
		cfg.HTTPAddr = ":18080"
	}

	cfg.DBDriver = strings.ToLower(strings.TrimSpace(cfg.DBDriver))
	if cfg.DBDriver == "" {
		if strings.TrimSpace(cfg.DBDSN) != "" {
			cfg.DBDriver = "postgres"
		} else {
			cfg.DBDriver = "sqlite"
		}
	}

	if cfg.DBDriver == "postgres" {
		cfg.DBPath = ""
	} else {
		cfg.DBDSN = ""
		if strings.TrimSpace(cfg.DBPath) == "" {
			cfg.DBPath = "./data/app.db"
		}
	}
	if strings.TrimSpace(cfg.SessionCookieName) == "" {
		cfg.SessionCookieName = "hostdeck_session"
	}
	if cfg.SessionTTLHours <= 0 {
		cfg.SessionTTLHours = 24
	}
	cfg.BootstrapAdminUsername = strings.TrimSpace(cfg.BootstrapAdminUsername)
	cfg.BootstrapAdminToken = strings.TrimSpace(cfg.BootstrapAdminToken)
	cfg.MasterKey = strings.TrimSpace(cfg.MasterKey)
	if strings.TrimSpace(cfg.WebDistDir) == "" {
		cfg.WebDistDir = "../web/dist"
	}
	if cfg.PollIntervalSeconds <= 0 {
		cfg.PollIntervalSeconds = 60
	}
	if cfg.PollConcurrency <= 0 {
		cfg.PollConcurrency = 5
	}
}

func resolveRelativePath(baseDir string, value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return trimmed
	}
	if trimmed == ":memory:" || strings.HasPrefix(trimmed, "file:") || filepath.IsAbs(trimmed) {
		return trimmed
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
