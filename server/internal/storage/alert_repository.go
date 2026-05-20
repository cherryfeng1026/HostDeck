package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"hostdeck/server/internal/credential"
	"hostdeck/server/internal/domain"
)

type AlertRepository struct {
	db        *sql.DB
	masterKey string
}

var (
	ErrAlertActionNotAllowed = errors.New("当前告警状态不支持该操作")
	ErrAlertRuleNotFound     = errors.New("告警规则不存在")
)

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

type AlertNotificationDeliveryBuilder func(domain.AlertState) (domain.AlertNotificationDelivery, string, bool, error)

type AlertNotificationDeliveryCreated func(domain.AlertNotificationDelivery)

type AlertEvaluationRecord struct {
	RuleID                      int64
	ServerID                    int64
	Metric                      string
	Operator                    string
	Threshold                   float64
	CurrentValue                float64
	Severity                    string
	Message                     string
	DurationSeconds             int
	TriggeredAt                 time.Time
	LastTriggeredAt             time.Time
	Status                      string
	AcknowledgedAt              *time.Time
	AcknowledgedBy              string
	MutedUntil                  *time.Time
	NotificationDeliveryBuilder AlertNotificationDeliveryBuilder
	NotificationDeliveryCreated AlertNotificationDeliveryCreated
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

func NewAlertRepository(db *sql.DB, masterKey ...string) *AlertRepository {
	repo := &AlertRepository{db: db}
	if len(masterKey) > 0 {
		repo.masterKey = strings.TrimSpace(masterKey[0])
	}
	return repo
}

func (r *AlertRepository) List(ctx context.Context) ([]domain.AlertRule, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, metric, operator, threshold, duration_seconds, enabled, scope_type, scope_value, created_at, updated_at
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
		`INSERT INTO alert_rules (metric, operator, threshold, duration_seconds, enabled, scope_type, scope_value, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		rule.Metric,
		rule.Operator,
		rule.Threshold,
		rule.DurationSeconds,
		boolToInt(rule.Enabled),
		normalizeAlertRuleScopeType(rule.ScopeType),
		normalizeAlertRuleScopeValue(rule.ScopeType, rule.ScopeValue),
		now,
		now,
	)
	return err
}

func (r *AlertRepository) Update(ctx context.Context, rule domain.AlertRule) error {
	result, err := r.db.ExecContext(
		ctx,
		`UPDATE alert_rules
		    SET metric = $1, operator = $2, threshold = $3, duration_seconds = $4, enabled = $5, scope_type = $6, scope_value = $7, updated_at = $8
		  WHERE id = $9`,
		rule.Metric,
		rule.Operator,
		rule.Threshold,
		rule.DurationSeconds,
		boolToInt(rule.Enabled),
		normalizeAlertRuleScopeType(rule.ScopeType),
		normalizeAlertRuleScopeValue(rule.ScopeType, rule.ScopeValue),
		time.Now().UTC().Format(time.RFC3339Nano),
		rule.ID,
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrAlertRuleNotFound
	}
	return nil
}

func (r *AlertRepository) Delete(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM alert_rules WHERE id = $1)`, id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrAlertRuleNotFound
	}

	rows, err := tx.QueryContext(
		ctx,
		`SELECT id, rule_id, server_id, metric, operator, threshold, current_value, severity, message,
		        status, duration_seconds, first_triggered_at, last_triggered_at, acknowledged_at,
		        acknowledged_by, muted_until, created_at, updated_at
		   FROM alert_states
		  WHERE rule_id = $1`,
		id,
	)
	if err != nil {
		return err
	}
	states := make([]domain.AlertState, 0)
	for rows.Next() {
		state, err := scanAlertState(rows)
		if err != nil {
			_ = rows.Close()
			return err
		}
		states = append(states, state)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	now := time.Now().UTC()
	for _, state := range states {
		if state.Status == domain.AlertStatusPending {
			continue
		}
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
			CreatedAt:    now,
			Detail:       "规则已删除",
		}); err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM alert_states WHERE rule_id = $1`, id); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM alert_rules WHERE id = $1`, id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrAlertRuleNotFound
	}
	return tx.Commit()
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

	var createdDelivery domain.AlertNotificationDelivery
	if record.NotificationDeliveryBuilder != nil {
		delivery, payload, ok, err := record.NotificationDeliveryBuilder(state)
		if err != nil {
			return domain.AlertState{}, false, err
		}
		if ok {
			createdDelivery, err = insertAlertNotificationDeliveryTx(ctx, tx, delivery, payload)
			if err != nil {
				return domain.AlertState{}, false, err
			}
		}
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
	if record.NotificationDeliveryCreated != nil && createdDelivery.ID != 0 {
		record.NotificationDeliveryCreated(createdDelivery)
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
	return r.listHistory(ctx, normalizeAlertHistoryLimit(limit), "", nil)
}

func (r *AlertRepository) ListHistoryByTypes(ctx context.Context, limit int, eventTypes ...string) ([]domain.AlertHistoryEvent, error) {
	return r.listHistory(ctx, normalizeAlertHistoryLimit(limit), "", eventTypes)
}

func (r *AlertRepository) SearchHistory(ctx context.Context, query string, limit int, eventTypes ...string) ([]domain.AlertHistoryEvent, error) {
	return r.listHistory(ctx, normalizeAlertHistoryLimit(limit), strings.TrimSpace(query), eventTypes)
}

func (r *AlertRepository) listHistory(ctx context.Context, limit int, query string, eventTypes []string) ([]domain.AlertHistoryEvent, error) {
	args := []any{}
	clauses := make([]string, 0, 2)
	serverNameExpression := "COALESCE(NULLIF(h.server_name, ''), s.name, '')"

	eventTypes = normalizeAlertHistoryEventTypes(eventTypes)
	if len(eventTypes) > 0 {
		placeholders := make([]string, 0, len(eventTypes))
		for _, eventType := range eventTypes {
			args = append(args, eventType)
			placeholders = append(placeholders, fmt.Sprintf("$%d", len(args)))
		}
		clauses = append(clauses, fmt.Sprintf("h.event_type IN (%s)", strings.Join(placeholders, ", ")))
	}

	if query != "" {
		args = append(args, "%"+strings.ToLower(query)+"%")
		placeholder := fmt.Sprintf("$%d", len(args))
		clauses = append(clauses, fmt.Sprintf(`(
			LOWER(%s) LIKE %s OR
			LOWER(h.message) LIKE %s OR
			LOWER(h.metric) LIKE %s OR
			LOWER(h.detail) LIKE %s OR
			LOWER(h.actor_username) LIKE %s
		)`, serverNameExpression, placeholder, placeholder, placeholder, placeholder, placeholder))
	}

	statement := fmt.Sprintf(`SELECT h.id, h.alert_id, h.rule_id, h.server_id, %s, h.event_type, h.metric,
	        h.operator, h.threshold, h.current_value, h.severity, h.message, h.status,
	        h.triggered_at, h.created_at, h.actor_username, h.detail
	   FROM alert_history h
	   LEFT JOIN servers s ON s.id = h.server_id`, serverNameExpression)
	if len(clauses) > 0 {
		statement += " WHERE " + strings.Join(clauses, " AND ")
	}
	args = append(args, limit)
	statement += fmt.Sprintf(" ORDER BY h.created_at DESC, h.id DESC LIMIT $%d", len(args))

	rows, err := r.db.QueryContext(ctx, statement, args...)
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

func normalizeAlertHistoryLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 200 {
		return 200
	}
	return limit
}

func normalizeAlertHistoryEventTypes(eventTypes []string) []string {
	if len(eventTypes) == 0 {
		return nil
	}
	filtered := make([]string, 0, len(eventTypes))
	seen := make(map[string]struct{}, len(eventTypes))
	for _, eventType := range eventTypes {
		value := strings.TrimSpace(eventType)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		filtered = append(filtered, value)
	}
	return filtered
}

func (r *AlertRepository) DeleteHistoryBefore(ctx context.Context, cutoff time.Time) error {
	_, err := r.db.ExecContext(
		ctx,
		`DELETE FROM alert_history WHERE created_at < $1`,
		cutoff.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (r *AlertRepository) CreateNotificationDelivery(ctx context.Context, delivery domain.AlertNotificationDelivery, payload string) (domain.AlertNotificationDelivery, error) {
	return insertAlertNotificationDelivery(ctx, r.db, delivery, payload)
}

type alertNotificationDeliveryInserter interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func insertAlertNotificationDelivery(ctx context.Context, inserter alertNotificationDeliveryInserter, delivery domain.AlertNotificationDelivery, payload string) (domain.AlertNotificationDelivery, error) {
	now := time.Now().UTC()
	occurredAt := delivery.OccurredAt.UTC()
	if occurredAt.IsZero() {
		occurredAt = now
	}
	row := inserter.QueryRowContext(
		ctx,
		`INSERT INTO alert_notification_deliveries (
			event_type, alert_id, rule_id, server_id, server_name, status, attempt_count,
			next_attempt_at, last_attempt_at, last_error, payload, occurred_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, 0, NULL, NULL, '', $7, $8, $9, $9)
		RETURNING id, event_type, alert_id, rule_id, server_id, server_name, status, attempt_count,
		          next_attempt_at, last_attempt_at, last_error, payload, occurred_at, created_at, updated_at`,
		delivery.EventType,
		delivery.AlertID,
		delivery.RuleID,
		delivery.ServerID,
		delivery.ServerName,
		normalizeNotificationDeliveryStatus(delivery.Status),
		strings.TrimSpace(payload),
		occurredAt.Format(time.RFC3339Nano),
		now.Format(time.RFC3339Nano),
	)
	return scanAlertNotificationDelivery(row)
}

func insertAlertNotificationDeliveryTx(ctx context.Context, tx *sql.Tx, delivery domain.AlertNotificationDelivery, payload string) (domain.AlertNotificationDelivery, error) {
	return insertAlertNotificationDelivery(ctx, tx, delivery, payload)
}

func (r *AlertRepository) RecordNotificationDeliveryAttempt(ctx context.Context, deliveryID int64, status string, lastError string, nextAttemptAt *time.Time, attemptedAt time.Time) error {
	attemptedAt = attemptedAt.UTC()
	if attemptedAt.IsZero() {
		attemptedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE alert_notification_deliveries
		    SET status = $1,
		        attempt_count = attempt_count + 1,
		        next_attempt_at = $2,
		        last_attempt_at = $3,
		        last_error = $4,
		        updated_at = $3
		  WHERE id = $5`,
		normalizeNotificationDeliveryStatus(status),
		nullableRFC3339(nextAttemptAt),
		attemptedAt.Format(time.RFC3339Nano),
		truncateNotificationDeliveryError(lastError),
		deliveryID,
	)
	return err
}

func (r *AlertRepository) ListNotificationDeliveries(ctx context.Context, filter domain.AlertNotificationDeliveryFilter) ([]domain.AlertNotificationDelivery, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	args := make([]any, 0, 2)
	clauses := make([]string, 0, 2)
	if status := strings.TrimSpace(filter.Status); status != "" {
		args = append(args, normalizeNotificationDeliveryStatus(status))
		clauses = append(clauses, fmt.Sprintf("status = $%d", len(args)))
	}
	if filter.DueOnly {
		now := filter.Now.UTC()
		if now.IsZero() {
			now = time.Now().UTC()
		}
		args = append(args, now.Format(time.RFC3339Nano))
		clauses = append(clauses, fmt.Sprintf("status IN ('%s', '%s') AND (next_attempt_at IS NULL OR next_attempt_at = '' OR next_attempt_at <= $%d)", domain.AlertNotificationDeliveryPending, domain.AlertNotificationDeliveryFailed, len(args)))
	}
	statement := `SELECT id, event_type, alert_id, rule_id, server_id, server_name, status, attempt_count,
	       next_attempt_at, last_attempt_at, last_error, payload, occurred_at, created_at, updated_at
	  FROM alert_notification_deliveries`
	if len(clauses) > 0 {
		statement += " WHERE " + strings.Join(clauses, " AND ")
	}
	args = append(args, limit)
	statement += fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d", len(args))

	rows, err := r.db.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.AlertNotificationDelivery, 0)
	for rows.Next() {
		item, err := scanAlertNotificationDelivery(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *AlertRepository) GetNotificationSettings(ctx context.Context) (domain.AlertNotificationSettings, error) {
	row := r.db.QueryRowContext(
		ctx,
		`SELECT enabled, webhook_url, webhook_timeout_seconds, created_at, updated_at
		   FROM alert_notification_settings
		  WHERE singleton = 1`,
	)
	settings, err := scanOptionalAlertNotificationSettings(row)
	if err != nil {
		return domain.AlertNotificationSettings{}, err
	}
	if settings.WebhookTimeoutSeconds <= 0 {
		settings.WebhookTimeoutSeconds = 5
	}
	settings.WebhookURL = strings.TrimSpace(settings.WebhookURL)
	if settings.WebhookURL != "" {
		decrypted, err := r.decryptNotificationWebhookURL(settings.WebhookURL)
		if err != nil {
			return domain.AlertNotificationSettings{}, err
		}
		settings.WebhookURL = decrypted
		settings.WebhookConfigured = true
		return settings, nil
	}
	settings.WebhookConfigured = false
	return settings, nil
}

func (r *AlertRepository) SaveNotificationSettings(ctx context.Context, settings domain.AlertNotificationSettings) (domain.AlertNotificationSettings, error) {
	settings.WebhookURL = strings.TrimSpace(settings.WebhookURL)
	if settings.WebhookTimeoutSeconds <= 0 {
		settings.WebhookTimeoutSeconds = 5
	}

	storedWebhookURL, err := r.encryptNotificationWebhookURL(settings.WebhookURL)
	if err != nil {
		return domain.AlertNotificationSettings{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = r.db.ExecContext(
		ctx,
		`INSERT INTO alert_notification_settings (singleton, enabled, webhook_url, webhook_timeout_seconds, created_at, updated_at)
		 VALUES (1, $1, $2, $3, $4, $4)
		 ON CONFLICT (singleton) DO UPDATE
		 SET enabled = EXCLUDED.enabled,
		     webhook_url = EXCLUDED.webhook_url,
		     webhook_timeout_seconds = EXCLUDED.webhook_timeout_seconds,
		     updated_at = EXCLUDED.updated_at`,
		boolToInt(settings.Enabled),
		storedWebhookURL,
		settings.WebhookTimeoutSeconds,
		now,
	)
	if err != nil {
		return domain.AlertNotificationSettings{}, err
	}
	return r.GetNotificationSettings(ctx)
}

func (r *AlertRepository) encryptNotificationWebhookURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if strings.TrimSpace(r.masterKey) == "" {
		return "", errors.New("master_key 未配置，无法加密通知 Webhook")
	}
	cipher, err := credential.NewCipher(r.masterKey)
	if err != nil {
		return "", err
	}
	return cipher.Encrypt(value)
}

func (r *AlertRepository) decryptNotificationWebhookURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value, nil
	}
	if strings.TrimSpace(r.masterKey) == "" {
		return "", errors.New("master_key 未配置，无法解密通知 Webhook")
	}
	cipher, err := credential.NewCipher(r.masterKey)
	if err != nil {
		return "", err
	}
	return cipher.Decrypt(value)
}

func insertAlertHistoryTx(ctx context.Context, tx *sql.Tx, event AlertMutationRecord) error {
	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO alert_history (
			alert_id, rule_id, server_id, server_name, event_type, metric, operator, threshold, current_value,
			severity, message, status, triggered_at, created_at, actor_username, detail
		) VALUES ($1, $2, $3, COALESCE((SELECT name FROM servers WHERE id = $3), ''), $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
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
		item       domain.AlertRule
		enabled    int
		scopeType  string
		scopeValue string
		createdAt  string
		updatedAt  string
		err        error
	)

	if err = scanner.Scan(
		&item.ID,
		&item.Metric,
		&item.Operator,
		&item.Threshold,
		&item.DurationSeconds,
		&enabled,
		&scopeType,
		&scopeValue,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domain.AlertRule{}, err
	}

	item.Enabled = enabled == 1
	item.ScopeType = normalizeAlertRuleScopeType(scopeType)
	item.ScopeValue = normalizeAlertRuleScopeValue(item.ScopeType, scopeValue)
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

func scanAlertNotificationDelivery(scanner interface {
	Scan(dest ...any) error
}) (domain.AlertNotificationDelivery, error) {
	var (
		item          domain.AlertNotificationDelivery
		nextAttemptAt sql.NullString
		lastAttemptAt sql.NullString
		occurredAt    string
		createdAt     string
		updatedAt     string
	)
	if err := scanner.Scan(
		&item.ID,
		&item.EventType,
		&item.AlertID,
		&item.RuleID,
		&item.ServerID,
		&item.ServerName,
		&item.Status,
		&item.AttemptCount,
		&nextAttemptAt,
		&lastAttemptAt,
		&item.LastError,
		&item.Payload,
		&occurredAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domain.AlertNotificationDelivery{}, err
	}
	var err error
	item.Status = normalizeNotificationDeliveryStatus(item.Status)
	item.NextAttemptAt, err = parseNullableRFC3339(nextAttemptAt)
	if err != nil {
		return domain.AlertNotificationDelivery{}, err
	}
	item.LastAttemptAt, err = parseNullableRFC3339(lastAttemptAt)
	if err != nil {
		return domain.AlertNotificationDelivery{}, err
	}
	if item.OccurredAt, err = time.Parse(time.RFC3339Nano, occurredAt); err != nil {
		return domain.AlertNotificationDelivery{}, err
	}
	if item.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return domain.AlertNotificationDelivery{}, err
	}
	if item.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
		return domain.AlertNotificationDelivery{}, err
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

func scanOptionalAlertNotificationSettings(scanner interface {
	Scan(dest ...any) error
}) (domain.AlertNotificationSettings, error) {
	item, err := scanAlertNotificationSettings(scanner)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.AlertNotificationSettings{}, nil
		}
		return domain.AlertNotificationSettings{}, err
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

func scanAlertNotificationSettings(scanner interface {
	Scan(dest ...any) error
}) (domain.AlertNotificationSettings, error) {
	var (
		item      domain.AlertNotificationSettings
		enabled   int
		createdAt string
		updatedAt string
		err       error
	)
	if err = scanner.Scan(
		&enabled,
		&item.WebhookURL,
		&item.WebhookTimeoutSeconds,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domain.AlertNotificationSettings{}, err
	}
	item.Enabled = enabled == 1
	if item.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return domain.AlertNotificationSettings{}, err
	}
	if item.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
		return domain.AlertNotificationSettings{}, err
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

func normalizeAlertRuleScopeType(value string) string {
	switch strings.TrimSpace(value) {
	case domain.AlertRuleScopeServer:
		return domain.AlertRuleScopeServer
	case domain.AlertRuleScopeTag:
		return domain.AlertRuleScopeTag
	case domain.AlertRuleScopePurpose:
		return domain.AlertRuleScopePurpose
	default:
		return domain.AlertRuleScopeAll
	}
}

func normalizeAlertRuleScopeValue(scopeType string, value string) string {
	if normalizeAlertRuleScopeType(scopeType) == domain.AlertRuleScopeAll {
		return ""
	}
	return strings.TrimSpace(value)
}

func normalizeNotificationDeliveryStatus(value string) string {
	switch strings.TrimSpace(value) {
	case domain.AlertNotificationDeliverySent:
		return domain.AlertNotificationDeliverySent
	case domain.AlertNotificationDeliveryFailed:
		return domain.AlertNotificationDeliveryFailed
	default:
		return domain.AlertNotificationDeliveryPending
	}
}

func truncateNotificationDeliveryError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 1024 {
		return value
	}
	return value[:1024]
}
