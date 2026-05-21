package scheduler

import (
	"context"
	"log/slog"
	"time"
)

type StatusHistoryCleaner interface {
	DeleteHistoryBefore(ctx context.Context, cutoff time.Time) error
}

type CommandLogCleaner interface {
	DeleteBefore(ctx context.Context, cutoff time.Time) error
}

type AlertHistoryCleaner interface {
	DeleteHistoryBefore(ctx context.Context, cutoff time.Time) error
}

type AuthEventCleaner interface {
	DeleteBefore(ctx context.Context, cutoff time.Time) error
}

type AuditEventCleaner interface {
	DeleteBefore(ctx context.Context, cutoff time.Time) error
}

type APITokenCleaner interface {
	CleanupAPITokens(ctx context.Context, now time.Time, retention time.Duration) error
}

type CleanupSchedule struct {
	StatusHistoryRetention time.Duration
	CommandLogRetention    time.Duration
	AlertHistoryRetention  time.Duration
	AuthEventRetention     time.Duration
	AuditEventRetention    time.Duration
	APITokenRetention      time.Duration
}

type CleanupRunner struct {
	statuses  StatusHistoryCleaner
	commands  CommandLogCleaner
	alerts    AlertHistoryCleaner
	auth      AuthEventCleaner
	audit     AuditEventCleaner
	apiTokens APITokenCleaner
	interval  time.Duration
	schedule  CleanupSchedule
}

func NewCleanupRunner(
	statuses StatusHistoryCleaner,
	commands CommandLogCleaner,
	alerts AlertHistoryCleaner,
	auth AuthEventCleaner,
	audit AuditEventCleaner,
	apiTokens APITokenCleaner,
	interval time.Duration,
	schedule CleanupSchedule,
) *CleanupRunner {
	if interval <= 0 {
		interval = time.Hour
	}
	return &CleanupRunner{
		statuses:  statuses,
		commands:  commands,
		alerts:    alerts,
		auth:      auth,
		audit:     audit,
		apiTokens: apiTokens,
		interval:  interval,
		schedule:  schedule,
	}
}

func (r *CleanupRunner) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.runOnce(ctx, time.Now().UTC())
		}
	}
}

func (r *CleanupRunner) RunNow(ctx context.Context) {
	r.runOnce(ctx, time.Now().UTC())
}

func (r *CleanupRunner) runOnce(ctx context.Context, now time.Time) {
	if r.statuses != nil && r.schedule.StatusHistoryRetention > 0 {
		if err := r.statuses.DeleteHistoryBefore(ctx, now.Add(-r.schedule.StatusHistoryRetention)); err != nil {
			slog.Warn("cleanup status history failed", "error", err)
		}
	}
	if r.commands != nil && r.schedule.CommandLogRetention > 0 {
		if err := r.commands.DeleteBefore(ctx, now.Add(-r.schedule.CommandLogRetention)); err != nil {
			slog.Warn("cleanup command logs failed", "error", err)
		}
	}
	if r.alerts != nil && r.schedule.AlertHistoryRetention > 0 {
		if err := r.alerts.DeleteHistoryBefore(ctx, now.Add(-r.schedule.AlertHistoryRetention)); err != nil {
			slog.Warn("cleanup alert history failed", "error", err)
		}
	}
	if r.auth != nil && r.schedule.AuthEventRetention > 0 {
		if err := r.auth.DeleteBefore(ctx, now.Add(-r.schedule.AuthEventRetention)); err != nil {
			slog.Warn("cleanup auth events failed", "error", err)
		}
	}
	if r.audit != nil && r.schedule.AuditEventRetention > 0 {
		if err := r.audit.DeleteBefore(ctx, now.Add(-r.schedule.AuditEventRetention)); err != nil {
			slog.Warn("cleanup audit events failed", "error", err)
		}
	}
	if r.apiTokens != nil && r.schedule.APITokenRetention > 0 {
		if err := r.apiTokens.CleanupAPITokens(ctx, now, r.schedule.APITokenRetention); err != nil {
			slog.Warn("cleanup api tokens failed", "error", err)
		}
	}
}
