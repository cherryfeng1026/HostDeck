package service

import (
	"testing"
	"time"

	"hostdeck/server/internal/collector"
	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/storage"
)

func TestBuildDashboardHeadlineUsesLastSuccessAtForLastUpdated(t *testing.T) {
	now := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	lastSuccess := now.Add(-2 * time.Minute)
	failedFinish := now

	headline := buildDashboardHeadline(
		[]domain.Server{{ID: 1, Name: "prod-web-01"}},
		[]storage.LatestStatus{{
			ServerID:      1,
			Snapshot:      collector.Snapshot{Online: false, SSHOK: false},
			CollectStatus: storage.CollectStatusFailed,
			LastSuccessAt: lastSuccess,
			LastReportAt:  failedFinish,
		}},
		nil,
	)

	if !headline.LastUpdatedAt.Equal(lastSuccess) {
		t.Fatalf("expected last updated at %v, got %v", lastSuccess, headline.LastUpdatedAt)
	}
}

func TestBuildDashboardResourceSummaryUsesPreservedLatestMetricsAfterFailure(t *testing.T) {
	summary := buildDashboardResourceSummary([]storage.LatestStatus{
		{
			ServerID:      1,
			Snapshot:      collector.Snapshot{Online: true, SSHOK: true, CPUUsage: 80, MemoryUsage: 60, DiskUsage: 40},
			CollectStatus: storage.CollectStatusOK,
			LastSuccessAt: time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC),
			LastReportAt:  time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC),
		},
		{
			ServerID:      2,
			Snapshot:      collector.Snapshot{Online: false, SSHOK: false, CPUUsage: 20, MemoryUsage: 30, DiskUsage: 50},
			CollectStatus: storage.CollectStatusFailed,
			LastReportAt:  time.Date(2026, 4, 28, 10, 1, 0, 0, time.UTC),
		},
		{
			ServerID:      3,
			Snapshot:      collector.Snapshot{Online: false, SSHOK: false, CPUUsage: 0, MemoryUsage: 0, DiskUsage: 0},
			CollectStatus: storage.CollectStatusFailed,
			LastReportAt:  time.Date(2026, 4, 28, 10, 2, 0, 0, time.UTC),
		},
	})

	if summary.ReportingServers != 2 {
		t.Fatalf("expected 2 reporting servers, got %d", summary.ReportingServers)
	}
	if summary.AvgCPUUsage != 50 || summary.AvgMemoryUsage != 45 || summary.AvgDiskUsage != 45 {
		t.Fatalf("unexpected averages: %+v", summary)
	}
	if summary.CollectFailedServers != 2 {
		t.Fatalf("expected 2 failed servers, got %+v", summary)
	}
}

func TestBuildDashboardFallbackTrendPointsUsesLatestMetrics(t *testing.T) {
	sampledAt := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	trends := buildDashboardFallbackTrendPoints([]storage.LatestStatus{
		{
			ServerID:      1,
			Snapshot:      collector.Snapshot{CPUUsage: 10, MemoryUsage: 20, DiskUsage: 30, Load1: 0.5},
			CollectStatus: storage.CollectStatusFailed,
			LastReportAt:  sampledAt,
		},
		{
			ServerID:      2,
			Snapshot:      collector.Snapshot{CPUUsage: 0, MemoryUsage: 0, DiskUsage: 0},
			CollectStatus: storage.CollectStatusFailed,
			LastReportAt:  sampledAt.Add(time.Minute),
		},
	})

	if len(trends) != 1 {
		t.Fatalf("expected one fallback trend, got %d", len(trends))
	}
	if trends[0].SampleCount != 1 || trends[0].AvgCPUUsage != 10 || trends[0].AvgMemoryUsage != 20 || trends[0].AvgDiskUsage != 30 || trends[0].AvgLoad1 != 0.5 || !trends[0].Fallback {
		t.Fatalf("unexpected fallback trend: %+v", trends[0])
	}
}

func TestBuildDashboardFallbackTrendPointsReturnsEmptySliceWithoutSamples(t *testing.T) {
	trends := buildDashboardFallbackTrendPoints([]storage.LatestStatus{
		{
			ServerID:      1,
			Snapshot:      collector.Snapshot{CPUUsage: 0, MemoryUsage: 0, DiskUsage: 0},
			CollectStatus: storage.CollectStatusFailed,
			LastReportAt:  time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC),
		},
	})

	if trends == nil {
		t.Fatal("expected empty trend slice, got nil")
	}
	if len(trends) != 0 {
		t.Fatalf("expected no fallback trends, got %+v", trends)
	}
}
