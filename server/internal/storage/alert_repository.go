package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"hostdeck/server/internal/domain"
)

type AlertRepository struct {
	db *sql.DB
}

var ErrAlertActionNotAllowed = errors.New("当前告警状态不支持该操作")

type AlertActionNotAllowedError struct {
	Action string
	Status string
}

func (e AlertActionNotAllowedError) Error() string {
	status := strings.TrimSpace(e.Status)
	action := strings.TrimSpace(e.Action)
	if status == "" && action == "" {
		return ErrAlertActionNotAllowed.Error()
	}
	if status == "" {
		return fmt.Sprintf("%s: %s", ErrAlertActionNotAllowed.Error(), action)
	}
	if action == "" {
		return fmt.Sprintf("%s: %s", ErrAlertActionNotAllowed.Error(), status)
	}
	return fmt.Sprintf("%s: %s -> %s", ErrAlertActionNotAllowed.Error(), status, action)
}

func (e AlertActionNotAllowedError) Is(target error) bool {
	return target == ErrAlertActionNotAllowed
}

type AlertEvaluationRecord struct {
	RuleID          int64
	ServerID        int64
	Metric          string
	Operator        string
	Threshold       float64
	CurrentValue    float64
	Severity        string
	Message         string
	DurationSeconds int
	TriggeredAt     time.Time
	LastTriggeredAt time.Time
	Status          string
	AcknowledgedAt  *time.Time
	AcknowledgedBy  string
	MutedUntil      *time.Time
}

type AlertMutationRecord struct {
	AlertID       int64
	RuleID        int64
	ServerID      int64
	EventType     string
	Metric        string
	Operator      string
	Threshold     float64
	CurrentValue  float64
	Severity      string
	Message       string
	Status        string
	TriggeredAt   time.Time
	CreatedAt     time.Time
	ActorUsername string
	Detail        string
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

func (r *AlertRepository) UpsertEvaluation(ctx context.Context, record AlertEvaluationRecord) (domain.AlertState, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.AlertState{}, false, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	existing, err := scanOptionalAlertState(tx.QueryRowContext(
		ctx,
		`SELECT id, rule_id, server_id, metric, operator, threshold, current_value, severity, message,
		        status, duration_seconds, first_triggered_at, last_triggered_at, acknowledged_at,
		        acknowledged_by, muted_until, created_at, updated_at
		   FROM alert_states
		  WHERE rule_id = $1 AND server_id = $2`,
		record.RuleID,
		record.ServerID,
	))
	if err != nil {
		return domain.AlertState{}, false, err
	}

	now := time.Now().UTC()
	created := false
	if existing.ID == 0 {
		created = true
		_, err = tx.ExecContext(
			ctx,
			`INSERT INTO alert_states (
				rule_id, server_id, metric, operator, threshold, current_value, severity, message,
				status, duration_seconds, first_triggered_at, last_triggered_at, acknowledged_at,
				acknowledged_by, muted_until, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NULL, '', $13, $14, $15)`,
			record.RuleID,
			record.ServerID,
			record.Metric,
			record.Operator,
			record.Threshold,
			record.CurrentValue,
			record.Severity,
			record.Message,
			record.Status,
			record.DurationSeconds,
			record.TriggeredAt.UTC().Format(time.RFC3339Nano),
			record.LastTriggeredAt.UTC().Format(time.RFC3339Nano),
			nullableRFC3339(record.MutedUntil),
			now.Format(time.RFC3339Nano),
			now.Format(time.RFC3339Nano),
		)
		if err != nil {
			return domain.AlertState{}, false, err
		}
	} else {
		acknowledgedAt := existing.AcknowledgedAt
		acknowledgedBy := existing.AcknowledgedBy
		mutedUntil := existing.MutedUntil
		status := existing.Status
		if record.Status != "" {
			status = record.Status
		}
		if status == domain.AlertStatusActive {
			acknowledgedAt = nil
			acknowledgedBy = ""
		}
		if mutedUntil != nil && mutedUntil.Before(now) {
			mutedUntil = nil
			if status == domain.AlertStatusMuted {
				status = domain.AlertStatusActive
			}
		}
		_, err = tx.ExecContext(
			ctx,
			`UPDATE alert_states
			    SET metric = $1,
			        operator = $2,
			        threshold = $3,
			        current_value = $4,
			        severity = $5,
			        message = $6,
			        status = $7,
			        duration_seconds = $8,
			        first_triggered_at = $9,
			        last_triggered_at = $10,
			        acknowledged_at = $11,
			        acknowledged_by = $12,
			        muted_until = $13,
			        updated_at = $14
			  WHERE id = $15`,
			record.Metric,
			record.Operator,
			record.Threshold,
			record.CurrentValue,
			record.Severity,
			record.Message,
			status,
			record.DurationSeconds,
			existing.FirstTriggeredAt.UTC().Format(time.RFC3339Nano),
			record.LastTriggeredAt.UTC().Format(time.RFC3339Nano),
			nullableRFC3339(acknowledgedAt),
			acknowledgedBy,
			nullableRFC3339(mutedUntil),
			now.Format(time.RFC3339Nano),
			existing.ID,
		)
		if err != nil {
			return domain.AlertState{}, false, err
		}
	}

	state, err := scanAlertState(tx.QueryRowContext(
		ctx,
		`SELECT id, rule_id, server_id, metric, operator, threshold, current_value, severity, message,
		        status, duration_seconds, first_triggered_at, last_triggered_at, acknowledged_at,
		        acknowledged_by, muted_until, created_at, updated_at
		   FROM alert_states
		  WHERE rule_id = $1 AND server_id = $2`,
		record.RuleID,
		record.ServerID,
	))
	if err != nil {
		return domain.AlertState{}, false, err
	}

	if created && state.Status != domain.AlertStatusPending {
		if err := insertAlertHistoryTx(ctx, tx, AlertMutationRecord{
			AlertID:      state.ID,
			RuleID:       state.RuleID,
			ServerID:     state.ServerID,
			EventType:    domain.AlertEventTriggered,
			Metric:       state.Metric,
			Operator:     state.Operator,
			Threshold:    state.Threshold,
			CurrentValue: state.CurrentValue,
			Severity:     state.Severity,
			Message:      state.Message,
			Status:       state.Status,
			TriggeredAt:  state.FirstTriggeredAt,
			CreatedAt:    state.CreatedAt,
		}); err != nil {
			return domain.AlertState{}, false, err
		}
	}
	if !created && existing.Status == domain.AlertStatusPending && state.Status != domain.AlertStatusPending {
		if err := insertAlertHistoryTx(ctx, tx, AlertMutationRecord{
			AlertID:      state.ID,
			RuleID:       state.RuleID,
			ServerID:     state.ServerID,
			EventType:    domain.AlertEventTriggered,
			Metric:       state.Metric,
			Operator:     state.Operator,
			Threshold:    state.Threshold,
			CurrentValue: state.CurrentValue,
			Severity:     state.Severity,
			Message:      state.Message,
			Status:       state.Status,
			TriggeredAt:  state.FirstTriggeredAt,
			CreatedAt:    now,
		}); err != nil {
			return domain.AlertState{}, false, err
		}
	}

	if err := tx.Commit(); err != nil {
		return domain.AlertState{}, false, err
	}
	return state, created, nil
}

func (r *AlertRepository) ResolveByRuleAndServer(ctx context.Context, ruleID int64, serverID int64, detail string) (bool, error) {
	state, err := r.GetStateByRuleAndServer(ctx, ruleID, serverID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, r.Resolve(ctx, state.ID, detail)
}

func (r *AlertRepository) Resolve(ctx context.Context, alertID int64, detail string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	state, err := scanAlertState(tx.QueryRowContext(
		ctx,
		`SELECT id, rule_id, server_id, metric, operator, threshold, current_value, severity, message,
		        status, duration_seconds, first_triggered_at, last_triggered_at, acknowledged_at,
		        acknowledged_by, muted_until, created_at, updated_at
		   FROM alert_states
		  WHERE id = $1`,
		alertID,
	))
	if err != nil {
		return err
	}

	resolvedAt := time.Now().UTC()
	if state.Status != domain.AlertStatusPending {
		if err := insertAlertHistoryTx(ctx, tx, AlertMutationRecord{
			AlertID:      state.ID,
			RuleID:       state.RuleID,
			ServerID:     state.ServerID,
			EventType:    domain.AlertEventResolved,
			Metric:       state.Metric,
			Operator:     state.Operator,
			Threshold:    state.Threshold,
			CurrentValue: state.CurrentValue,
			Severity:     state.Severity,
			Message:      state.Message,
			Status:       state.Status,
			TriggeredAt:  state.FirstTriggeredAt,
			CreatedAt:    resolvedAt,
			Detail:       strings.TrimSpace(detail),
		}); err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM alert_states WHERE id = $1`, state.ID); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *AlertRepository) Acknowledge(ctx context.Context, alertID int64, username string) (domain.AlertState, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.AlertState{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	state, err := scanAlertState(tx.QueryRowContext(
		ctx,
		`SELECT id, rule_id, server_id, metric, operator, threshold, current_value, severity, message,
		        status, duration_seconds, first_triggered_at, last_triggered_at, acknowledged_at,
		        acknowledged_by, muted_until, created_at, updated_at
		   FROM alert_states
		  WHERE id = $1`,
		alertID,
	))
	if err != nil {
		return domain.AlertState{}, err
	}

	if state.Status != domain.AlertStatusActive {
		return domain.AlertState{}, AlertActionNotAllowedError{Action: domain.AlertEventAcknowledged, Status: state.Status}
	}

	now := time.Now().UTC()
	state.Status = domain.AlertStatusAcknowledged
	state.AcknowledgedAt = &now
	state.AcknowledgedBy = strings.TrimSpace(username)
	state.UpdatedAt = now
	if _, err := tx.ExecContext(
		ctx,
		`UPDATE alert_states
		    SET status = $1, acknowledged_at = $2, acknowledged_by = $3, updated_at = $4
		  WHERE id = $5`,
		state.Status,
		now.Format(time.RFC3339Nano),
		state.AcknowledgedBy,
		now.Format(time.RFC3339Nano),
		state.ID,
	); err != nil {
		return domain.AlertState{}, err
	}
	if err := insertAlertHistoryTx(ctx, tx, AlertMutationRecord{
		AlertID:       state.ID,
		RuleID:        state.RuleID,
		ServerID:      state.ServerID,
		EventType:     domain.AlertEventAcknowledged,
		Metric:        state.Metric,
		Operator:      state.Operator,
		Threshold:     state.Threshold,
		CurrentValue:  state.CurrentValue,
		Severity:      state.Severity,
		Message:       state.Message,
		Status:        state.Status,
		TriggeredAt:   state.FirstTriggeredAt,
		CreatedAt:     now,
		ActorUsername: state.AcknowledgedBy,
	}); err != nil {
		return domain.AlertState{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.AlertState{}, err
	}
	return state, nil
}

func (r *AlertRepository) Mute(ctx context.Context, alertID int64, username string, mutedUntil time.Time) (domain.AlertState, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.AlertState{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	state, err := scanAlertState(tx.QueryRowContext(
		ctx,
		`SELECT id, rule_id, server_id, metric, operator, threshold, current_value, severity, message,
		        status, duration_seconds, first_triggered_at, last_triggered_at, acknowledged_at,
		        acknowledged_by, muted_until, created_at, updated_at
		   FROM alert_states
		  WHERE id = $1`,
		alertID,
	))
	if err != nil {
		return domain.AlertState{}, err
	}

	if state.Status != domain.AlertStatusActive && state.Status != domain.AlertStatusAcknowledged {
		return domain.AlertState{}, AlertActionNotAllowedError{Action: domain.AlertEventMuted, Status: state.Status}
	}

	now := time.Now().UTC()
	mutedUntil = mutedUntil.UTC()
	state.Status = domain.AlertStatusMuted
	state.MutedUntil = &mutedUntil
	state.UpdatedAt = now
	if _, err := tx.ExecContext(
		ctx,
		`UPDATE alert_states
		    SET status = $1, muted_until = $2, updated_at = $3
		  WHERE id = $4`,
		state.Status,
		mutedUntil.Format(time.RFC3339Nano),
		now.Format(time.RFC3339Nano),
		state.ID,
	); err != nil {
		return domain.AlertState{}, err
	}
	if err := insertAlertHistoryTx(ctx, tx, AlertMutationRecord{
		AlertID:       state.ID,
		RuleID:        state.RuleID,
		ServerID:      state.ServerID,
		EventType:     domain.AlertEventMuted,
		Metric:        state.Metric,
		Operator:      state.Operator,
		Threshold:     state.Threshold,
		CurrentValue:  state.CurrentValue,
		Severity:      state.Severity,
		Message:       state.Message,
		Status:        state.Status,
		TriggeredAt:   state.FirstTriggeredAt,
		CreatedAt:     now,
		ActorUsername: strings.TrimSpace(username),
		Detail:        fmt.Sprintf("muted until %s", mutedUntil.Format(time.RFC3339Nano)),
	}); err != nil {
		return domain.AlertState{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.AlertState{}, err
	}
	return state, nil
}

func (r *AlertRepository) GetStateByRuleAndServer(ctx context.Context, ruleID int64, serverID int64) (domain.AlertState, error) {
	return scanAlertState(r.db.QueryRowContext(
		ctx,
		`SELECT id, rule_id, server_id, metric, operator, threshold, current_value, severity, message,
		        status, duration_seconds, first_triggered_at, last_triggered_at, acknowledged_at,
		        acknowledged_by, muted_until, created_at, updated_at
		   FROM alert_states
		  WHERE rule_id = $1 AND server_id = $2`,
		ruleID,
		serverID,
	))
}

func (r *AlertRepository) ListCurrentStates(ctx context.Context) ([]domain.AlertState, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, rule_id, server_id, metric, operator, threshold, current_value, severity, message,
		        status, duration_seconds, first_triggered_at, last_triggered_at, acknowledged_at,
		        acknowledged_by, muted_until, created_at, updated_at
		   FROM alert_states
		  ORDER BY severity DESC, last_triggered_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.AlertState, 0)
	for rows.Next() {
		item, err := scanAlertState(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].LastTriggeredAt.After(items[j].LastTriggeredAt)
	})
	return items, rows.Err()
}

func (r *AlertRepository) ListHistory(ctx context.Context, limit int) ([]domain.AlertHistoryEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT h.id, h.alert_id, h.rule_id, h.server_id, COALESCE(s.name, ''), h.event_type, h.metric,
		        h.operator, h.threshold, h.current_value, h.severity, h.message, h.status,
		        h.triggered_at, h.created_at, h.actor_username, h.detail
		   FROM alert_history h
		   LEFT JOIN servers s ON s.id = h.server_id
		  ORDER BY h.created_at DESC, h.id DESC
		  LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.AlertHistoryEvent, 0)
	for rows.Next() {
		item, err := scanAlertHistoryEvent(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *AlertRepository) DeleteHistoryBefore(ctx context.Context, cutoff time.Time) error {
	_, err := r.db.ExecContext(
		ctx,
		`DELETE FROM alert_history WHERE created_at < $1`,
		cutoff.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func insertAlertHistoryTx(ctx context.Context, tx *sql.Tx, event AlertMutationRecord) error {
	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO alert_history (
			alert_id, rule_id, server_id, event_type, metric, operator, threshold, current_value,
			severity, message, status, triggered_at, created_at, actor_username, detail
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
		event.AlertID,
		event.RuleID,
		event.ServerID,
		event.EventType,
		event.Metric,
		event.Operator,
		event.Threshold,
		event.CurrentValue,
		event.Severity,
		event.Message,
		event.Status,
		event.TriggeredAt.UTC().Format(time.RFC3339Nano),
		event.CreatedAt.UTC().Format(time.RFC3339Nano),
		event.ActorUsername,
		event.Detail,
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

func scanOptionalAlertState(scanner interface {
	Scan(dest ...any) error
}) (domain.AlertState, error) {
	item, err := scanAlertState(scanner)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.AlertState{}, nil
		}
		return domain.AlertState{}, err
	}
	return item, nil
}

func scanAlertState(scanner interface {
	Scan(dest ...any) error
}) (domain.AlertState, error) {
	var (
		item             domain.AlertState
		firstTriggeredAt string
		lastTriggeredAt  string
		acknowledgedAt   sql.NullString
		mutedUntil       sql.NullString
		createdAt        string
		updatedAt        string
	)
	if err := scanner.Scan(
		&item.ID,
		&item.RuleID,
		&item.ServerID,
		&item.Metric,
		&item.Operator,
		&item.Threshold,
		&item.CurrentValue,
		&item.Severity,
		&item.Message,
		&item.Status,
		&item.DurationSeconds,
		&firstTriggeredAt,
		&lastTriggeredAt,
		&acknowledgedAt,
		&item.AcknowledgedBy,
		&mutedUntil,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domain.AlertState{}, err
	}
	var err error
	if item.FirstTriggeredAt, err = time.Parse(time.RFC3339Nano, firstTriggeredAt); err != nil {
		return domain.AlertState{}, err
	}
	if item.LastTriggeredAt, err = time.Parse(time.RFC3339Nano, lastTriggeredAt); err != nil {
		return domain.AlertState{}, err
	}
	if acknowledgedAt.Valid {
		parsed, err := time.Parse(time.RFC3339Nano, acknowledgedAt.String)
		if err != nil {
			return domain.AlertState{}, err
		}
		item.AcknowledgedAt = &parsed
	}
	if mutedUntil.Valid {
		parsed, err := time.Parse(time.RFC3339Nano, mutedUntil.String)
		if err != nil {
			return domain.AlertState{}, err
		}
		item.MutedUntil = &parsed
	}
	if item.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return domain.AlertState{}, err
	}
	if item.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
		return domain.AlertState{}, err
	}
	return item, nil
}

func scanAlertHistoryEvent(scanner interface {
	Scan(dest ...any) error
}) (domain.AlertHistoryEvent, error) {
	var (
		item        domain.AlertHistoryEvent
		triggeredAt string
		createdAt   string
	)
	if err := scanner.Scan(
		&item.ID,
		&item.AlertID,
		&item.RuleID,
		&item.ServerID,
		&item.ServerName,
		&item.EventType,
		&item.Metric,
		&item.Operator,
		&item.Threshold,
		&item.CurrentValue,
		&item.Severity,
		&item.Message,
		&item.Status,
		&triggeredAt,
		&createdAt,
		&item.ActorUsername,
		&item.Detail,
	); err != nil {
		return domain.AlertHistoryEvent{}, err
	}
	var err error
	if item.TriggeredAt, err = time.Parse(time.RFC3339Nano, triggeredAt); err != nil {
		return domain.AlertHistoryEvent{}, err
	}
	if item.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return domain.AlertHistoryEvent{}, err
	}
	return item, nil
}

func nullableRFC3339(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}
