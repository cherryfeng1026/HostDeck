package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

type Config struct {
	File       string
	Level      string
	MaxSizeMB  int
	MaxBackups int
	MaxAgeDays int
	Compress   bool
}

func Configure(cfg Config) (func() error, error) {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, err
	}

	var levelVar slog.LevelVar
	levelVar.Set(level)

	output := io.Writer(os.Stdout)
	var closer io.Closer
	if strings.TrimSpace(cfg.File) != "" {
		writer, err := NewRotatingFileWriter(RotatingFileConfig{
			Filename:   cfg.File,
			MaxSizeMB:  cfg.MaxSizeMB,
			MaxBackups: cfg.MaxBackups,
			MaxAgeDays: cfg.MaxAgeDays,
			Compress:   cfg.Compress,
		})
		if err != nil {
			return nil, err
		}
		output = io.MultiWriter(os.Stdout, writer)
		closer = writer
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{
		Level: &levelVar,
	})))

	if closer == nil {
		return func() error { return nil }, nil
	}
	return closer.Close, nil
}

func parseLevel(value string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("日志级别无效: %s", value)
	}
}
