package storage

import (
	"context"
	"database/sql"
	"time"

	"hostdeck/server/internal/domain"
)

type AlertRepository struct {
	db *sql.DB
}

func NewAlertRepository(db *sql.DB) *AlertRepository {
	return &AlertRepository{db: db}
}

func (r *AlertRepository) List(ctx context.Context) ([]domain.AlertRule, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, metric, operator, threshold, duration_seconds, enabled, created_at, updated_at
		   FROM alert_rules
		  ORDER BY id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.AlertRule, 0)
	for rows.Next() {
		item, err := scanAlertRule(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *AlertRepository) Create(ctx context.Context, rule domain.AlertRule) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO alert_rules (metric, operator, threshold, duration_seconds, enabled, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		rule.Metric,
		rule.Operator,
		rule.Threshold,
		rule.DurationSeconds,
		boolToInt(rule.Enabled),
		now,
		now,
	)
	return err
}

func (r *AlertRepository) Update(ctx context.Context, rule domain.AlertRule) error {
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE alert_rules
		    SET metric = $1, operator = $2, threshold = $3, duration_seconds = $4, enabled = $5, updated_at = $6
		  WHERE id = $7`,
		rule.Metric,
		rule.Operator,
		rule.Threshold,
		rule.DurationSeconds,
		boolToInt(rule.Enabled),
		time.Now().UTC().Format(time.RFC3339Nano),
		rule.ID,
	)
	return err
}

func scanAlertRule(scanner interface {
	Scan(dest ...any) error
}) (domain.AlertRule, error) {
	var (
		item      domain.AlertRule
		enabled   int
		createdAt string
		updatedAt string
		err       error
	)

	if err = scanner.Scan(
		&item.ID,
		&item.Metric,
		&item.Operator,
		&item.Threshold,
		&item.DurationSeconds,
		&enabled,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domain.AlertRule{}, err
	}

	item.Enabled = enabled == 1
	item.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return domain.AlertRule{}, err
	}
	item.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return domain.AlertRule{}, err
	}
	return item, nil
}
