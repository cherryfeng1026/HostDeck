package scheduler

import (
	"context"
	"testing"
	"time"
)

type recordingStatusHistoryCleaner struct {
	cutoff time.Time
}

func (r *recordingStatusHistoryCleaner) DeleteHistoryBefore(ctx context.Context, cutoff time.Time) error {
	r.cutoff = cutoff
	return nil
}

type recordingCommandLogCleaner struct {
	cutoff time.Time
}

func (r *recordingCommandLogCleaner) DeleteBefore(ctx context.Context, cutoff time.Time) error {
	r.cutoff = cutoff
	return nil
}

type recordingAlertHistoryCleaner struct {
	cutoff time.Time
}

func (r *recordingAlertHistoryCleaner) DeleteHistoryBefore(ctx context.Context, cutoff time.Time) error {
	r.cutoff = cutoff
	return nil
}

type recordingAuthEventCleaner struct {
	cutoff time.Time
}

func (r *recordingAuthEventCleaner) DeleteBefore(ctx context.Context, cutoff time.Time) error {
	r.cutoff = cutoff
	return nil
}

type recordingAuditEventCleaner struct {
	cutoff time.Time
}

func (r *recordingAuditEventCleaner) DeleteBefore(ctx context.Context, cutoff time.Time) error {
	r.cutoff = cutoff
	return nil
}

type recordingAPITokenCleaner struct {
	now       time.Time
	retention time.Duration
}

func (r *recordingAPITokenCleaner) CleanupAPITokens(ctx context.Context, now time.Time, retention time.Duration) error {
	r.now = now
	r.retention = retention
	return nil
}

func TestCleanupRunnerRunNowAppliesAllRetentionPolicies(t *testing.T) {
	statusCleaner := &recordingStatusHistoryCleaner{}
	commandCleaner := &recordingCommandLogCleaner{}
	alertCleaner := &recordingAlertHistoryCleaner{}
	authCleaner := &recordingAuthEventCleaner{}
	auditCleaner := &recordingAuditEventCleaner{}
	apiTokenCleaner := &recordingAPITokenCleaner{}

	runner := NewCleanupRunner(
		statusCleaner,
		commandCleaner,
		alertCleaner,
		authCleaner,
		auditCleaner,
		apiTokenCleaner,
		time.Hour,
		CleanupSchedule{
			StatusHistoryRetention: 24 * time.Hour,
			CommandLogRetention:    48 * time.Hour,
			AlertHistoryRetention:  72 * time.Hour,
			AuthEventRetention:     96 * time.Hour,
			AuditEventRetention:    120 * time.Hour,
			APITokenRetention:      144 * time.Hour,
		},
	)

	now := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)
	runner.runOnce(context.Background(), now)

	if !statusCleaner.cutoff.Equal(now.Add(-24 * time.Hour)) {
		t.Fatalf("unexpected status cutoff: %s", statusCleaner.cutoff)
	}
	if !commandCleaner.cutoff.Equal(now.Add(-48 * time.Hour)) {
		t.Fatalf("unexpected command cutoff: %s", commandCleaner.cutoff)
	}
	if !alertCleaner.cutoff.Equal(now.Add(-72 * time.Hour)) {
		t.Fatalf("unexpected alert cutoff: %s", alertCleaner.cutoff)
	}
	if !authCleaner.cutoff.Equal(now.Add(-96 * time.Hour)) {
		t.Fatalf("unexpected auth cutoff: %s", authCleaner.cutoff)
	}
	if !auditCleaner.cutoff.Equal(now.Add(-120 * time.Hour)) {
		t.Fatalf("unexpected audit cutoff: %s", auditCleaner.cutoff)
	}
	if !apiTokenCleaner.now.Equal(now) || apiTokenCleaner.retention != 144*time.Hour {
		t.Fatalf("unexpected api token cleanup args: now=%s retention=%s", apiTokenCleaner.now, apiTokenCleaner.retention)
	}
}
