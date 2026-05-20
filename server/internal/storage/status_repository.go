package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"hostdeck/server/internal/collector"
)

const (
	CollectStatusUnknown  = "unknown"
	CollectStatusOK       = "ok"
	CollectStatusFailed   = "failed"
	CollectStatusStale    = "stale"
	CollectStatusDisabled = "disabled"
)

type StatusRepository struct {
	db *sql.DB
}

type LatestStatus struct {
	ServerID int64 `json:"serverId"`
	collector.Snapshot
	LastReportAt          time.Time `json:"lastReportAt"`
	CollectStatus         string    `json:"collectStatus"`
	LastCollectStartedAt  time.Time `json:"lastCollectStartedAt"`
	LastCollectFinishedAt time.Time `json:"lastCollectFinishedAt"`
	LastSuccessAt         time.Time `json:"lastSuccessAt"`
	LastCollectError    string    `json:"lastCollectError"`
	CollectFailureCount int       `json:"collectFailureCount"`
	Stale               bool      `json:"stale"`
}

type HistoryPoint struct {
	SampledAt   time.Time `json:"sampledAt"`
	CPUUsage    float64   `json:"cpuUsage"`
	MemoryUsage float64   `json:"memoryUsage"`
	DiskUsage   float64   `json:"diskUsage"`
	Load1       float64   `json:"load1"`
	Load5       float64   `json:"load5"`
	Load15      float64   `json:"load15"`
}

type FleetHistoryPoint struct {
	SampledAt      time.Time `json:"sampledAt"`
	AvgCPUUsage    float64   `json:"avgCpuUsage"`
	AvgMemoryUsage float64   `json:"avgMemoryUsage"`
	AvgDiskUsage   float64   `json:"avgDiskUsage"`
	AvgLoad1       float64   `json:"avgLoad1"`
	AvgLoad5       float64   `json:"avgLoad5"`
	AvgLoad15      float64   `json:"avgLoad15"`
	SampleCount    int       `json:"sampleCount"`
}

func NewStatusRepository(db *sql.DB) *StatusRepository {
	return &StatusRepository{db: db}
}

func (r *StatusRepository) UpsertLatest(ctx context.Context, serverID int64, snapshot collector.Snapshot, sampledAt time.Time) error {
	sampledAtText := sampledAt.UTC().Format(time.RFC3339Nano)
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO server_status_latest (
			server_id, online, ssh_ok, cpu_usage, memory_usage, disk_usage, os_version,
			kernel_version, uptime_seconds, load_1, load_5, load_15, last_report_at, source,
			collect_status, last_collect_finished_at, last_success_at, last_collect_error, collect_failure_count, collect_duration_ms, stale
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
		ON CONFLICT(server_id) DO UPDATE SET
			online = excluded.online,
			ssh_ok = excluded.ssh_ok,
			cpu_usage = excluded.cpu_usage,
			memory_usage = excluded.memory_usage,
			disk_usage = excluded.disk_usage,
			os_version = excluded.os_version,
			kernel_version = excluded.kernel_version,
			uptime_seconds = excluded.uptime_seconds,
			load_1 = excluded.load_1,
			load_5 = excluded.load_5,
			load_15 = excluded.load_15,
			last_report_at = excluded.last_report_at,
			source = excluded.source,
			collect_status = excluded.collect_status,
			last_collect_finished_at = excluded.last_collect_finished_at,
			last_success_at = excluded.last_success_at,
			last_collect_error = excluded.last_collect_error,
			collect_failure_count = excluded.collect_failure_count,
			collect_duration_ms = excluded.collect_duration_ms,
			stale = excluded.stale`,
		serverID,
		boolToInt(snapshot.Online),
		boolToInt(snapshot.SSHOK),
		snapshot.CPUUsage,
		snapshot.MemoryUsage,
		snapshot.DiskUsage,
		snapshot.OSVersion,
		snapshot.KernelVersion,
		snapshot.UptimeSeconds,
		snapshot.Load1,
		snapshot.Load5,
		snapshot.Load15,
		sampledAtText,
		snapshot.Source,
		CollectStatusOK,
		sampledAtText,
		sampledAtText,
		"",
		0,
		snapshot.CollectDurationMS,
		0,
	)
	return err
}

func (r *StatusRepository) MarkCollectStarted(ctx context.Context, serverID int64, startedAt time.Time) error {
	startedAtText := startedAt.UTC().Format(time.RFC3339Nano)
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO server_status_latest (
			server_id, online, ssh_ok, cpu_usage, memory_usage, disk_usage, os_version,
			kernel_version, uptime_seconds, load_1, load_5, load_15, last_report_at, source,
			collect_status, last_collect_started_at, last_collect_finished_at, last_success_at,
			last_collect_error, collect_failure_count, collect_duration_ms, stale
		) VALUES ($1, 0, 0, 0, 0, 0, '', '', 0, 0, 0, 0, '', '', $3, $2, '', '', '', 0, 0, 0)
		ON CONFLICT(server_id) DO UPDATE SET
			last_collect_started_at = excluded.last_collect_started_at,
			collect_status = excluded.collect_status,
			last_collect_error = excluded.last_collect_error,
			collect_duration_ms = 0,
			stale = 0`,
		serverID,
		startedAtText,
		CollectStatusUnknown,
	)
	return err
}

func (r *StatusRepository) MarkCollectFailure(ctx context.Context, serverID int64, err error, finishedAt time.Time) error {
	message := ""
	if err != nil {
		message = strings.TrimSpace(err.Error())
	}
	_, execErr := r.db.ExecContext(
		ctx,
		`INSERT INTO server_status_latest (
			server_id, online, ssh_ok, cpu_usage, memory_usage, disk_usage, os_version,
			kernel_version, uptime_seconds, load_1, load_5, load_15, last_report_at, source,
			collect_status, last_collect_started_at, last_collect_finished_at, last_success_at,
			last_collect_error, collect_failure_count, collect_duration_ms, stale
		) VALUES ($1, 0, 0, 0, 0, 0, '', '', 0, 0, 0, 0, '', $3, $4, '', $2, '', $5, 1, 0, 0)
		ON CONFLICT(server_id) DO UPDATE SET
			online = 0,
			ssh_ok = 0,
			source = excluded.source,
			collect_status = excluded.collect_status,
			last_collect_finished_at = excluded.last_collect_finished_at,
			last_collect_error = excluded.last_collect_error,
			collect_failure_count = server_status_latest.collect_failure_count + 1,
			collect_duration_ms = 0,
			stale = 0`,
		serverID,
		finishedAt.UTC().Format(time.RFC3339Nano),
		"ssh",
		CollectStatusFailed,
		message,
	)
	return execErr
}

func (r *StatusRepository) MarkDisabled(ctx context.Context, serverID int64, finishedAt time.Time) error {
	finishedAtText := finishedAt.UTC().Format(time.RFC3339Nano)
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO server_status_latest (
			server_id, online, ssh_ok, cpu_usage, memory_usage, disk_usage, os_version,
			kernel_version, uptime_seconds, load_1, load_5, load_15, last_report_at, source,
			collect_status, last_collect_started_at, last_collect_finished_at, last_success_at,
			last_collect_error, collect_failure_count, collect_duration_ms, stale
		) VALUES ($1, 0, 0, 0, 0, 0, '', '', 0, 0, 0, 0, '', '', $3, '', $2, '', '', 0, 0, 0)
		ON CONFLICT(server_id) DO UPDATE SET
			online = 0,
			ssh_ok = 0,
			collect_status = excluded.collect_status,
			last_collect_finished_at = excluded.last_collect_finished_at,
			last_collect_error = excluded.last_collect_error,
			collect_duration_ms = 0,
			stale = 0`,
		serverID,
		finishedAtText,
		CollectStatusDisabled,
	)
	return err
}

func (r *StatusRepository) MarkStaleBefore(ctx context.Context, cutoff time.Time) error {
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE server_status_latest
		    SET collect_status = $1,
		        stale = 1
		  WHERE last_success_at <> ''
		    AND last_success_at < $2
		    AND collect_status NOT IN ($3, $4)`,
		CollectStatusStale,
		cutoff.UTC().Format(time.RFC3339Nano),
		CollectStatusFailed,
		CollectStatusDisabled,
	)
	return err
}

func (r *StatusRepository) AppendHistory(ctx context.Context, serverID int64, snapshot collector.Snapshot, sampledAt time.Time) error {
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO server_status_history (
			server_id, sampled_at, cpu_usage, memory_usage, disk_usage, load_1, load_5, load_15
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		serverID,
		sampledAt.UTC().Format(time.RFC3339Nano),
		snapshot.CPUUsage,
		snapshot.MemoryUsage,
		snapshot.DiskUsage,
		snapshot.Load1,
		snapshot.Load5,
		snapshot.Load15,
	)
	return err
}

func (r *StatusRepository) GetLatest(ctx context.Context, serverID int64) (LatestStatus, error) {
	row := r.db.QueryRowContext(
		ctx,
		latestStatusSelectSQL+` WHERE server_id = $1`,
		serverID,
	)

	return scanLatestStatus(row)
}

func (r *StatusRepository) ListLatest(ctx context.Context) ([]LatestStatus, error) {
	rows, err := r.db.QueryContext(
		ctx,
		latestStatusSelectSQL+` ORDER BY server_id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]LatestStatus, 0)
	for rows.Next() {
		item, err := scanLatestStatus(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *StatusRepository) ListHistory(ctx context.Context, serverID int64, since time.Time) ([]HistoryPoint, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT sampled_at, cpu_usage, memory_usage, disk_usage, load_1, load_5, load_15
		   FROM server_status_history
		  WHERE server_id = $1 AND sampled_at >= $2
		  ORDER BY sampled_at ASC`,
		serverID,
		since.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	points := make([]HistoryPoint, 0)
	for rows.Next() {
		point, err := scanHistoryPoint(rows)
		if err != nil {
			return nil, err
		}
		points = append(points, point)
	}
	return points, rows.Err()
}

func (r *StatusRepository) ListFleetHistory(ctx context.Context, since time.Time) ([]FleetHistoryPoint, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT sampled_at,
		        AVG(cpu_usage) AS avg_cpu_usage,
		        AVG(memory_usage) AS avg_memory_usage,
		        AVG(disk_usage) AS avg_disk_usage,
		        AVG(load_1) AS avg_load_1,
		        AVG(load_5) AS avg_load_5,
		        AVG(load_15) AS avg_load_15,
		        COUNT(*) AS sample_count
		   FROM server_status_history
		  WHERE sampled_at >= $1
		  GROUP BY sampled_at
		  ORDER BY sampled_at ASC`,
		since.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	points := make([]FleetHistoryPoint, 0)
	for rows.Next() {
		var (
			point     FleetHistoryPoint
			sampledAt string
		)
		if err := rows.Scan(
			&sampledAt,
			&point.AvgCPUUsage,
			&point.AvgMemoryUsage,
			&point.AvgDiskUsage,
			&point.AvgLoad1,
			&point.AvgLoad5,
			&point.AvgLoad15,
			&point.SampleCount,
		); err != nil {
			return nil, err
		}
		point.SampledAt, err = time.Parse(time.RFC3339Nano, sampledAt)
		if err != nil {
			return nil, err
		}
		points = append(points, point)
	}
	return points, rows.Err()
}

func (r *StatusRepository) DeleteHistoryBefore(ctx context.Context, cutoff time.Time) error {
	_, err := r.db.ExecContext(
		ctx,
		`DELETE FROM server_status_history WHERE sampled_at < $1`,
		cutoff.UTC().Format(time.RFC3339Nano),
	)
	return err
}

const latestStatusSelectSQL = `SELECT server_id, online, ssh_ok, cpu_usage, memory_usage, disk_usage, os_version,
		kernel_version, uptime_seconds, load_1, load_5, load_15, last_report_at, source,
		collect_status, last_collect_started_at, last_collect_finished_at, last_success_at,
		last_collect_error, collect_failure_count, collect_duration_ms, stale
	   FROM server_status_latest`

func scanHistoryPoint(scanner interface {
	Scan(dest ...any) error
}) (HistoryPoint, error) {
	var (
		point     HistoryPoint
		sampledAt string
	)
	if err := scanner.Scan(
		&sampledAt,
		&point.CPUUsage,
		&point.MemoryUsage,
		&point.DiskUsage,
		&point.Load1,
		&point.Load5,
		&point.Load15,
	); err != nil {
		return HistoryPoint{}, err
	}
	parsed, err := time.Parse(time.RFC3339Nano, sampledAt)
	if err != nil {
		return HistoryPoint{}, err
	}
	point.SampledAt = parsed
	return point, nil
}

func scanLatestStatus(scanner interface {
	Scan(dest ...any) error
}) (LatestStatus, error) {
	var (
		item                  LatestStatus
		online                int
		sshOK                 int
		lastReportAt          string
		lastCollectStartedAt  string
		lastCollectFinishedAt string
		lastSuccessAt         string
		collectDurationMS     int64
		stale                 int
	)

	err := scanner.Scan(
		&item.ServerID,
		&online,
		&sshOK,
		&item.CPUUsage,
		&item.MemoryUsage,
		&item.DiskUsage,
		&item.OSVersion,
		&item.KernelVersion,
		&item.UptimeSeconds,
		&item.Load1,
		&item.Load5,
		&item.Load15,
		&lastReportAt,
		&item.Source,
		&item.CollectStatus,
		&lastCollectStartedAt,
		&lastCollectFinishedAt,
		&lastSuccessAt,
		&item.LastCollectError,
		&item.CollectFailureCount,
		&collectDurationMS,
		&stale,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return LatestStatus{}, err
		}
		return LatestStatus{}, err
	}

	item.Online = online == 1
	item.SSHOK = sshOK == 1
	item.CollectDurationMS = collectDurationMS
	item.Stale = stale == 1
	item.CollectStatus = normalizeCollectStatus(item.CollectStatus)
	item.LastReportAt, err = parseRequiredStatusTime(lastReportAt)
	if err != nil {
		return LatestStatus{}, err
	}
	item.LastCollectStartedAt, err = parseOptionalStatusTime(lastCollectStartedAt)
	if err != nil {
		return LatestStatus{}, err
	}
	item.LastCollectFinishedAt, err = parseOptionalStatusTime(lastCollectFinishedAt)
	if err != nil {
		return LatestStatus{}, err
	}
	item.LastSuccessAt, err = parseOptionalStatusTime(lastSuccessAt)
	if err != nil {
		return LatestStatus{}, err
	}
	return item, nil
}

func normalizeCollectStatus(value string) string {
	switch strings.TrimSpace(value) {
	case CollectStatusOK, CollectStatusFailed, CollectStatusStale, CollectStatusDisabled:
		return strings.TrimSpace(value)
	default:
		return CollectStatusUnknown
	}
}

func parseRequiredStatusTime(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, value)
}

func parseOptionalStatusTime(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, value)
}
