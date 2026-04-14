package storage_test

import (
	"context"
	"testing"
	"time"

	"hostdeck/server/internal/collector"
	"hostdeck/server/internal/storage"
)

func TestStatusRepository_UpsertLatestAndAppendHistory(t *testing.T) {
	db := openTestDB(t)
	repo := storage.NewStatusRepository(db)

	snapshot := collector.Snapshot{
		Online:        true,
		SSHOK:         true,
		CPUUsage:      21.5,
		MemoryUsage:   48.2,
		DiskUsage:     61.0,
		OSVersion:     "Ubuntu 24.04 LTS",
		KernelVersion: "6.8.0-31-generic",
		UptimeSeconds: 12345,
		Load1:         0.31,
		Load5:         0.27,
		Load15:        0.19,
		Source:        "ssh",
	}
	sampledAt := time.Date(2026, 4, 15, 9, 0, 0, 0, time.UTC)

	if err := repo.UpsertLatest(context.Background(), 1, snapshot, sampledAt); err != nil {
		t.Fatalf("upsert latest failed: %v", err)
	}
	if err := repo.AppendHistory(context.Background(), 1, snapshot, sampledAt); err != nil {
		t.Fatalf("append history failed: %v", err)
	}

	latest, err := repo.GetLatest(context.Background(), 1)
	if err != nil {
		t.Fatalf("get latest failed: %v", err)
	}
	if !latest.Online || latest.MemoryUsage != 48.2 {
		t.Fatalf("unexpected latest snapshot: %+v", latest)
	}

	points, err := repo.ListHistory(context.Background(), 1, sampledAt.Add(-time.Hour))
	if err != nil {
		t.Fatalf("list history failed: %v", err)
	}
	if len(points) != 1 {
		t.Fatalf("expected 1 history point, got %d", len(points))
	}
	if points[0].CPUUsage != 21.5 {
		t.Fatalf("unexpected history point: %+v", points[0])
	}
}
