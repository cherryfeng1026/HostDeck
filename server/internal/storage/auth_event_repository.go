package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"hostdeck/server/internal/domain"
)

type AuthEventRepository struct {
	db *sql.DB
}

func NewAuthEventRepository(db *sql.DB) *AuthEventRepository {
	return &AuthEventRepository{db: db}
}

func (r *AuthEventRepository) Create(ctx context.Context, event domain.AuthEvent) error {
	createdAt := event.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO auth_events (user_id, username, event_type, detail, ip, user_agent, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		event.UserID,
		event.Username,
		event.EventType,
		event.Detail,
		event.IP,
		event.UserAgent,
		createdAt.Format(time.RFC3339Nano),
	)
	return err
}

func (r *AuthEventRepository) ListRecent(ctx context.Context, limit int, keyword string, eventTypes ...string) ([]domain.AuthEvent, error) {
	if limit <= 0 {
		limit = 20
	}

	var query strings.Builder
	query.WriteString(`SELECT id, user_id, username, event_type, detail, ip, user_agent, created_at FROM auth_events`)

	args := make([]any, 0, len(eventTypes)+4)
	clauses := make([]string, 0, 2)
	nextIndex := 1

	if len(eventTypes) > 0 {
		placeholders := make([]string, 0, len(eventTypes))
		for _, eventType := range eventTypes {
			placeholders = append(placeholders, fmt.Sprintf("$%d", nextIndex))
			args = append(args, eventType)
			nextIndex++
		}
		clauses = append(clauses, "event_type IN ("+strings.Join(placeholders, ", ")+")")
	}
	if strings.TrimSpace(keyword) != "" {
		clauses = append(clauses, fmt.Sprintf("(username LIKE $%d OR event_type LIKE $%d OR detail LIKE $%d)", nextIndex, nextIndex+1, nextIndex+2))
		pattern := "%" + strings.TrimSpace(keyword) + "%"
		args = append(args, pattern, pattern, pattern)
		nextIndex += 3
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

	items := make([]domain.AuthEvent, 0, limit)
	for rows.Next() {
		item, err := scanAuthEvent(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *AuthEventRepository) DeleteBefore(ctx context.Context, cutoff time.Time) error {
	_, err := r.db.ExecContext(
		ctx,
		`DELETE FROM auth_events WHERE created_at < $1`,
		cutoff.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func scanAuthEvent(scanner interface {
	Scan(dest ...any) error
}) (domain.AuthEvent, error) {
	var (
		item      domain.AuthEvent
		createdAt string
	)

	if err := scanner.Scan(
		&item.ID,
		&item.UserID,
		&item.Username,
		&item.EventType,
		&item.Detail,
		&item.IP,
		&item.UserAgent,
		&createdAt,
	); err != nil {
		return domain.AuthEvent{}, err
	}

	parsed, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return domain.AuthEvent{}, err
	}
	item.CreatedAt = parsed
	return item, nil
}
