package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"hostdeck/server/internal/domain"
)

type CommandLogRepository struct {
	db *sql.DB
}

type CommandLogListItem struct {
	ID               int64
	ServerID         int64
	ServerName       string
	ExecutorUsername string
	Command          string
	ExitCode         int
	DurationMS       int64
	ExecutedAt       time.Time
}

func NewCommandLogRepository(db *sql.DB) *CommandLogRepository {
	return &CommandLogRepository{db: db}
}

func (r *CommandLogRepository) Create(ctx context.Context, log domain.CommandLog) error {
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO command_logs (server_id, executor_username, command, stdout, stderr, exit_code, duration_ms, executed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		log.ServerID,
		strings.TrimSpace(log.ExecutorUsername),
		log.Command,
		log.Stdout,
		log.Stderr,
		log.ExitCode,
		log.DurationMS,
		log.ExecutedAt.UTC(),
	)
	return err
}

func (r *CommandLogRepository) ListByServerID(ctx context.Context, serverID int64) ([]domain.CommandLog, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, server_id, executor_username, command, stdout, stderr, exit_code, duration_ms, executed_at
		   FROM command_logs
		  WHERE server_id = $1
		  ORDER BY executed_at DESC, id DESC`,
		serverID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.CommandLog, 0)
	for rows.Next() {
		var (
			item       domain.CommandLog
			executedAt time.Time
		)
		if err := rows.Scan(
			&item.ID,
			&item.ServerID,
			&item.ExecutorUsername,
			&item.Command,
			&item.Stdout,
			&item.Stderr,
			&item.ExitCode,
			&item.DurationMS,
			&executedAt,
		); err != nil {
			return nil, err
		}

		item.ExecutedAt = executedAt.UTC()
		items = append(items, item)
	}

	return items, rows.Err()
}

func (r *CommandLogRepository) ListHistory(ctx context.Context, filter domain.CommandHistoryFilter) ([]domain.CommandLog, error) {
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}

	var query strings.Builder
	query.WriteString(`SELECT
		c.id,
		c.server_id,
		COALESCE(s.name, ''),
		COALESCE(c.executor_username, ''),
		c.command,
		c.stdout,
		c.stderr,
		c.exit_code,
		c.duration_ms,
		c.executed_at
	FROM command_logs c
	LEFT JOIN servers s ON s.id = c.server_id`)

	args := make([]any, 0, 6)
	clauses := make([]string, 0, 5)
	nextIndex := 1
	if filter.ServerID > 0 {
		clauses = append(clauses, fmt.Sprintf("c.server_id = $%d", nextIndex))
		args = append(args, filter.ServerID)
		nextIndex++
	}
	if executor := strings.TrimSpace(filter.ExecutorUsername); executor != "" {
		clauses = append(clauses, fmt.Sprintf("c.executor_username = $%d", nextIndex))
		args = append(args, executor)
		nextIndex++
	}
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		clauses = append(clauses, fmt.Sprintf("(LOWER(c.command) LIKE LOWER($%d) OR LOWER(COALESCE(s.name, '')) LIKE LOWER($%d) OR LOWER(COALESCE(s.hostname, '')) LIKE LOWER($%d) OR LOWER(COALESCE(s.ip, '')) LIKE LOWER($%d))", nextIndex, nextIndex+1, nextIndex+2, nextIndex+3))
		pattern := "%" + keyword + "%"
		args = append(args, pattern, pattern, pattern, pattern)
		nextIndex += 4
	}
	if filter.StartTime != nil {
		clauses = append(clauses, fmt.Sprintf("c.executed_at >= $%d", nextIndex))
		args = append(args, filter.StartTime.UTC())
		nextIndex++
	}
	if filter.EndTime != nil {
		clauses = append(clauses, fmt.Sprintf("c.executed_at <= $%d", nextIndex))
		args = append(args, filter.EndTime.UTC())
		nextIndex++
	}
	if len(clauses) > 0 {
		query.WriteString(" WHERE ")
		query.WriteString(strings.Join(clauses, " AND "))
	}
	query.WriteString(fmt.Sprintf(" ORDER BY c.executed_at DESC, c.id DESC LIMIT $%d", nextIndex))
	args = append(args, filter.Limit)

	rows, err := r.db.QueryContext(ctx, query.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.CommandLog, 0, filter.Limit)
	for rows.Next() {
		var (
			item       domain.CommandLog
			executedAt time.Time
		)
		if err := rows.Scan(
			&item.ID,
			&item.ServerID,
			&item.ServerName,
			&item.ExecutorUsername,
			&item.Command,
			&item.Stdout,
			&item.Stderr,
			&item.ExitCode,
			&item.DurationMS,
			&executedAt,
		); err != nil {
			return nil, err
		}
		item.ExecutedAt = executedAt.UTC()
		items = append(items, item)
	}

	return items, rows.Err()
}

func (r *CommandLogRepository) ListRecent(ctx context.Context, limit int, keyword string) ([]CommandLogListItem, error) {
	if limit <= 0 {
		limit = 20
	}

	var query strings.Builder
	query.WriteString(`SELECT
		c.id,
		c.server_id,
		COALESCE(s.name, ''),
		COALESCE(c.executor_username, ''),
		c.command,
		c.exit_code,
		c.duration_ms,
		c.executed_at
	FROM command_logs c
	LEFT JOIN servers s ON s.id = c.server_id`)

	args := make([]any, 0, 5)
	nextIndex := 1
	if strings.TrimSpace(keyword) != "" {
		pattern := "%" + strings.TrimSpace(keyword) + "%"
		query.WriteString(fmt.Sprintf(` WHERE (c.command LIKE $%d OR COALESCE(s.name, '') LIKE $%d OR COALESCE(s.hostname, '') LIKE $%d OR COALESCE(s.ip, '') LIKE $%d OR COALESCE(c.executor_username, '') LIKE $%d)`, nextIndex, nextIndex+1, nextIndex+2, nextIndex+3, nextIndex+4))
		args = append(args, pattern, pattern, pattern, pattern, pattern)
		nextIndex += 5
	}
	query.WriteString(fmt.Sprintf(` ORDER BY c.executed_at DESC, c.id DESC LIMIT $%d`, nextIndex))
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, query.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]CommandLogListItem, 0, limit)
	for rows.Next() {
		var (
			item       CommandLogListItem
			executedAt time.Time
		)
		if err := rows.Scan(
			&item.ID,
			&item.ServerID,
			&item.ServerName,
			&item.ExecutorUsername,
			&item.Command,
			&item.ExitCode,
			&item.DurationMS,
			&executedAt,
		); err != nil {
			return nil, err
		}

		item.ExecutedAt = executedAt.UTC()
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *CommandLogRepository) DeleteBefore(ctx context.Context, cutoff time.Time) error {
	_, err := r.db.ExecContext(
		ctx,
		`DELETE FROM command_logs WHERE executed_at < $1`,
		cutoff.UTC(),
	)
	return err
}
