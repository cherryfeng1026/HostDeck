package storage_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"hostdeck/server/internal/collector"
	"hostdeck/server/internal/storage"
)

func TestStatusRepository_ListFleetHistoryAggregatesSharedSamples(t *testing.T) {
	db := openTestDB(t)
	repo := storage.NewStatusRepository(db)
	sampledAt := time.Date(2026, 4, 15, 9, 0, 0, 0, time.UTC)

	first := collector.Snapshot{CPUUsage: 20, MemoryUsage: 40, DiskUsage: 60, Load1: 0.2, Load5: 0.3, Load15: 0.4}
	second := collector.Snapshot{CPUUsage: 40, MemoryUsage: 60, DiskUsage: 80, Load1: 0.4, Load5: 0.5, Load15: 0.6}
	if err := repo.AppendHistory(context.Background(), 1, first, sampledAt); err != nil {
		t.Fatalf("append first history: %v", err)
	}
	if err := repo.AppendHistory(context.Background(), 2, second, sampledAt); err != nil {
		t.Fatalf("append second history: %v", err)
	}

	points, err := repo.ListFleetHistory(context.Background(), sampledAt.Add(-time.Minute))
	if err != nil {
		t.Fatalf("list fleet history: %v", err)
	}
	if len(points) != 1 {
		t.Fatalf("expected one fleet point, got %d", len(points))
	}
	point := points[0]
	if point.SampleCount != 2 || point.AvgCPUUsage != 30 || point.AvgMemoryUsage != 50 || point.AvgDiskUsage != 70 || point.AvgLoad1 != 0.3 || point.AvgLoad5 != 0.4 || point.AvgLoad15 != 0.5 {
		t.Fatalf("unexpected fleet aggregate: %+v", point)
	}
}

func TestStatusRepository_MarkCollectFailureCreatesFirstLatestRow(t *testing.T) {
	db := openTestDB(t)
	repo := storage.NewStatusRepository(db)
	finishedAt := time.Date(2026, 4, 15, 9, 0, 0, 0, time.UTC)

	if err := repo.MarkCollectFailure(context.Background(), 99, errors.New("host key not trusted"), finishedAt); err != nil {
		t.Fatalf("mark collect failure: %v", err)
	}
	latest, err := repo.GetLatest(context.Background(), 99)
	if err != nil {
		t.Fatalf("get latest after failure: %v", err)
	}
	if latest.CollectStatus != storage.CollectStatusFailed || latest.CollectFailureCount != 1 || latest.LastCollectError != "host key not trusted" {
		t.Fatalf("unexpected failure status: %+v", latest)
	}
	if !latest.LastCollectFinishedAt.Equal(finishedAt) {
		t.Fatalf("expected finished at %v, got %v", finishedAt, latest.LastCollectFinishedAt)
	}
}

func TestStatusRepository_MarkDisabledCreatesFirstLatestRow(t *testing.T) {
	db := openTestDB(t)
	repo := storage.NewStatusRepository(db)
	finishedAt := time.Date(2026, 4, 15, 9, 30, 0, 0, time.UTC)

	if err := repo.MarkDisabled(context.Background(), 100, finishedAt); err != nil {
		t.Fatalf("mark disabled: %v", err)
	}
	latest, err := repo.GetLatest(context.Background(), 100)
	if err != nil {
		t.Fatalf("get latest after disabled: %v", err)
	}
	if latest.CollectStatus != storage.CollectStatusDisabled || latest.Online || latest.SSHOK {
		t.Fatalf("unexpected disabled status: %+v", latest)
	}
	if !latest.LastCollectFinishedAt.Equal(finishedAt) {
		t.Fatalf("expected finished at %v, got %v", finishedAt, latest.LastCollectFinishedAt)
	}
}

func TestStatusRepository_UpsertLatestAndAppendHistory(t *testing.T) {
	db := openTestDB(t)
	repo := storage.NewStatusRepository(db)

	snapshot := collector.Snapshot{
		Online:            true,
		SSHOK:             true,
		CPUUsage:          21.5,
		MemoryUsage:       48.2,
		DiskUsage:         61.0,
		OSVersion:         "Ubuntu 24.04 LTS",
		KernelVersion:     "6.8.0-31-generic",
		UptimeSeconds:     12345,
		Load1:             0.31,
		Load5:             0.27,
		Load15:            0.19,
		CollectDurationMS: 1350,
		Source:            "ssh",
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
	if latest.CollectDurationMS != snapshot.CollectDurationMS {
		t.Fatalf("expected collect duration %d, got %d", snapshot.CollectDurationMS, latest.CollectDurationMS)
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
