package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"hostdeck/server/internal/domain"
)

type AuditEventRepository struct {
	db *sql.DB
}

func NewAuditEventRepository(db *sql.DB) *AuditEventRepository {
	return &AuditEventRepository{db: db}
}

func (r *AuditEventRepository) Create(ctx context.Context, event domain.AuditEvent) error {
	createdAt := event.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO audit_events (kind, severity, title, summary, server_id, server_name, username, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		event.Kind,
		event.Severity,
		event.Title,
		event.Summary,
		event.ServerID,
		event.ServerName,
		event.Username,
		createdAt.Format(time.RFC3339Nano),
	)
	return err
}

func (r *AuditEventRepository) ListRecent(ctx context.Context, limit int, keyword string, kinds ...string) ([]domain.AuditEvent, error) {
	if limit <= 0 {
		limit = 20
	}

	var query strings.Builder
	query.WriteString(`SELECT id, kind, severity, title, summary, server_id, server_name, username, created_at FROM audit_events`)

	args := make([]any, 0, len(kinds)+5)
	clauses := make([]string, 0, 2)
	nextIndex := 1
	if len(kinds) > 0 {
		placeholders := make([]string, 0, len(kinds))
		for _, kind := range kinds {
			placeholders = append(placeholders, fmt.Sprintf("$%d", nextIndex))
			args = append(args, kind)
			nextIndex++
		}
		clauses = append(clauses, "kind IN ("+strings.Join(placeholders, ", ")+")")
	}
	if strings.TrimSpace(keyword) != "" {
		pattern := "%" + strings.TrimSpace(keyword) + "%"
		clauses = append(clauses, fmt.Sprintf("(title LIKE $%d OR summary LIKE $%d OR server_name LIKE $%d OR username LIKE $%d)", nextIndex, nextIndex+1, nextIndex+2, nextIndex+3))
		args = append(args, pattern, pattern, pattern, pattern)
		nextIndex += 4
	}
	if len(clauses) > 0 {
		query.WriteString(" WHERE ")
		query.WriteString(strings.Join(clauses, " AND "))
	}
	query.WriteString(fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", nextIndex))
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, query.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.AuditEvent, 0, limit)
	for rows.Next() {
		item, err := scanAuditEvent(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *AuditEventRepository) DeleteBefore(ctx context.Context, cutoff time.Time) error {
	_, err := r.db.ExecContext(
		ctx,
		`DELETE FROM audit_events WHERE created_at < $1`,
		cutoff.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func scanAuditEvent(scanner interface{ Scan(dest ...any) error }) (domain.AuditEvent, error) {
	var (
		item      domain.AuditEvent
		createdAt string
	)
	if err := scanner.Scan(
		&item.ID,
		&item.Kind,
		&item.Severity,
		&item.Title,
		&item.Summary,
		&item.ServerID,
		&item.ServerName,
		&item.Username,
		&createdAt,
	); err != nil {
		return domain.AuditEvent{}, err
	}
	parsed, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return domain.AuditEvent{}, err
	}
	item.CreatedAt = parsed
	return item, nil
}
