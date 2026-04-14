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
	ID         int64
	ServerID   int64
	ServerName string
	Command    string
	ExitCode   int
	DurationMS int64
	ExecutedAt time.Time
}

func NewCommandLogRepository(db *sql.DB) *CommandLogRepository {
	return &CommandLogRepository{db: db}
}

func (r *CommandLogRepository) Create(ctx context.Context, log domain.CommandLog) error {
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO command_logs (server_id, command, stdout, stderr, exit_code, duration_ms, executed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		log.ServerID,
		log.Command,
		log.Stdout,
		log.Stderr,
		log.ExitCode,
		log.DurationMS,
		log.ExecutedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (r *CommandLogRepository) ListByServerID(ctx context.Context, serverID int64) ([]domain.CommandLog, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, server_id, command, stdout, stderr, exit_code, duration_ms, executed_at
		   FROM command_logs
		  WHERE server_id = $1
		  ORDER BY executed_at DESC`,
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
			executedAt string
		)
		if err := rows.Scan(
			&item.ID,
			&item.ServerID,
			&item.Command,
			&item.Stdout,
			&item.Stderr,
			&item.ExitCode,
			&item.DurationMS,
			&executedAt,
		); err != nil {
			return nil, err
		}

		item.ExecutedAt, err = time.Parse(time.RFC3339Nano, executedAt)
		if err != nil {
			return nil, err
		}
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
		c.command,
		c.exit_code,
		c.duration_ms,
		c.executed_at
	FROM command_logs c
	LEFT JOIN servers s ON s.id = c.server_id`)

	args := make([]any, 0, 4)
	nextIndex := 1
	if strings.TrimSpace(keyword) != "" {
		pattern := "%" + strings.TrimSpace(keyword) + "%"
		query.WriteString(fmt.Sprintf(` WHERE (c.command LIKE $%d OR COALESCE(s.name, '') LIKE $%d OR COALESCE(s.hostname, '') LIKE $%d OR COALESCE(s.ip, '') LIKE $%d)`, nextIndex, nextIndex+1, nextIndex+2, nextIndex+3))
		args = append(args, pattern, pattern, pattern, pattern)
		nextIndex += 4
	}
	query.WriteString(fmt.Sprintf(` ORDER BY c.executed_at DESC LIMIT $%d`, nextIndex))
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
			executedAt string
		)
		if err := rows.Scan(
			&item.ID,
			&item.ServerID,
			&item.ServerName,
			&item.Command,
			&item.ExitCode,
			&item.DurationMS,
			&executedAt,
		); err != nil {
			return nil, err
		}

		item.ExecutedAt, err = time.Parse(time.RFC3339Nano, executedAt)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
