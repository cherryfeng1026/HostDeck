package storage_test

import (
	"context"
	"testing"
	"time"

	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/storage"
	"hostdeck/server/internal/testsupport"
)

func TestCommandLogRepository_ListHistorySupportsKeywordAndTimeFilters(t *testing.T) {
	db := testsupport.OpenPostgresTestDB(t)
	serverRepo := storage.NewServerRepository(db, "test-master-key")
	logRepo := storage.NewCommandLogRepository(db)

	servers := []domain.Server{
		{
			Name:          "prod-web-01",
			Hostname:      "prod-web-01.internal",
			IP:            "10.0.0.21",
			SSHPort:       22,
			Username:      "root",
			AuthType:      "password",
			Password:      "secret-1",
			CollectorMode: "ssh_only",
			Enabled:       true,
		},
		{
			Name:          "batch-node-02",
			Hostname:      "batch-node-02.internal",
			IP:            "10.0.0.22",
			SSHPort:       22,
			Username:      "root",
			AuthType:      "password",
			Password:      "secret-2",
			CollectorMode: "ssh_only",
			Enabled:       true,
		},
	}
	for _, server := range servers {
		if err := serverRepo.Create(context.Background(), server); err != nil {
			t.Fatalf("create server: %v", err)
		}
	}

	now := time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC)
	logs := []domain.CommandLog{
		{
			ServerID:         1,
			ExecutorUsername: "admin",
			Command:          "df -h",
			Stdout:           "ok",
			Stderr:           "",
			ExitCode:         0,
			DurationMS:       120,
			ExecutedAt:       now,
		},
		{
			ServerID:         2,
			ExecutorUsername: "viewer",
			Command:          "uptime",
			Stdout:           "load average",
			Stderr:           "",
			ExitCode:         0,
			DurationMS:       80,
			ExecutedAt:       now.Add(-time.Minute),
		},
	}
	for _, item := range logs {
		if err := logRepo.Create(context.Background(), item); err != nil {
			t.Fatalf("create command log: %v", err)
		}
	}

	start := now.Add(-30 * time.Second)
	end := now.Add(30 * time.Second)
	items, err := logRepo.ListHistory(context.Background(), domain.CommandHistoryFilter{
		Keyword:   "10.0.0.21",
		StartTime: &start,
		EndTime:   &end,
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("list history by ip keyword: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 history item, got %d", len(items))
	}
	if items[0].ServerID != 1 || items[0].ServerName != "prod-web-01" {
		t.Fatalf("unexpected history item: %+v", items[0])
	}

	items, err = logRepo.ListHistory(context.Background(), domain.CommandHistoryFilter{
		Keyword: "batch-node-02.internal",
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("list history by hostname keyword: %v", err)
	}
	if len(items) != 1 || items[0].ServerID != 2 {
		t.Fatalf("expected hostname filter to match server 2, got %+v", items)
	}
}

func TestCommandLogRepository_ListHistorySupportsSubsecondTimeRange(t *testing.T) {
	db := testsupport.OpenPostgresTestDB(t)
	serverRepo := storage.NewServerRepository(db, "test-master-key")
	logRepo := storage.NewCommandLogRepository(db)

	if err := serverRepo.Create(context.Background(), domain.Server{
		Name:          "prod-web-01",
		Hostname:      "prod-web-01.internal",
		IP:            "10.0.0.21",
		SSHPort:       22,
		Username:      "root",
		AuthType:      "password",
		Password:      "secret-1",
		CollectorMode: "ssh_only",
		Enabled:       true,
	}); err != nil {
		t.Fatalf("create server: %v", err)
	}

	base := time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC)
	for _, item := range []domain.CommandLog{
		{
			ServerID:   1,
			Command:    "cmd-a",
			Stdout:     "ok",
			Stderr:     "",
			ExitCode:   0,
			DurationMS: 10,
			ExecutedAt: base,
		},
		{
			ServerID:   1,
			Command:    "cmd-b",
			Stdout:     "ok",
			Stderr:     "",
			ExitCode:   0,
			DurationMS: 10,
			ExecutedAt: base.Add(100 * time.Millisecond),
		},
	} {
		if err := logRepo.Create(context.Background(), item); err != nil {
			t.Fatalf("create command log: %v", err)
		}
	}

	start := base
	end := base.Add(200 * time.Millisecond)
	items, err := logRepo.ListHistory(context.Background(), domain.CommandHistoryFilter{
		StartTime: &start,
		EndTime:   &end,
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("list history: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 history items, got %d", len(items))
	}
	if items[0].Command != "cmd-b" || items[1].Command != "cmd-a" {
		t.Fatalf("expected subsecond range to return both rows in desc order, got %+v", items)
	}
}
