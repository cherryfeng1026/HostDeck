package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"hostdeck/server/internal/domain"
)

type APITokenRecord struct {
	domain.APIToken
	TokenHash string
}

type APITokenRepository struct {
	db *sql.DB
}

func NewAPITokenRepository(db *sql.DB) *APITokenRepository {
	return &APITokenRepository{db: db}
}

func (r *APITokenRepository) Create(ctx context.Context, userID int64, name string, tokenHash string, prefix string, scopes []string, expiresAt *time.Time, now time.Time) (domain.APIToken, error) {
	scopesJSON, err := json.Marshal(normalizeAPITokenScopes(scopes))
	if err != nil {
		return domain.APIToken{}, err
	}
	timestamp := now.UTC().Format(time.RFC3339Nano)
	expiresAtValue := ""
	if expiresAt != nil && !expiresAt.IsZero() {
		expiresAtValue = expiresAt.UTC().Format(time.RFC3339Nano)
	}

	row := r.db.QueryRowContext(
		ctx,
		`INSERT INTO api_tokens (user_id, name, token_hash, token_prefix, scopes, last_used_at, expires_at, created_at, updated_at, revoked_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING id, user_id, name, token_prefix, scopes, last_used_at, expires_at, created_at, updated_at, revoked_at`,
		userID,
		name,
		tokenHash,
		prefix,
		string(scopesJSON),
		"",
		expiresAtValue,
		timestamp,
		timestamp,
		"",
	)
	return scanAPIToken(row)
}

func (r *APITokenRepository) ListActiveByUserID(ctx context.Context, userID int64) ([]domain.APIToken, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, user_id, name, token_prefix, scopes, last_used_at, expires_at, created_at, updated_at, revoked_at
		   FROM api_tokens
		  WHERE user_id = $1 AND revoked_at = '' AND (expires_at = '' OR expires_at > $2)
		  ORDER BY created_at DESC`,
		userID,
		now,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.APIToken, 0)
	for rows.Next() {
		item, err := scanAPIToken(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *APITokenRepository) GetByID(ctx context.Context, id int64) (APITokenRecord, error) {
	row := r.db.QueryRowContext(
		ctx,
		`SELECT id, user_id, name, token_hash, token_prefix, scopes, last_used_at, expires_at, created_at, updated_at, revoked_at
		   FROM api_tokens
		  WHERE id = $1`,
		id,
	)
	return scanAPITokenRecord(row)
}

func (r *APITokenRepository) GetByTokenHash(ctx context.Context, tokenHash string) (APITokenRecord, error) {
	row := r.db.QueryRowContext(
		ctx,
		`SELECT id, user_id, name, token_hash, token_prefix, scopes, last_used_at, expires_at, created_at, updated_at, revoked_at
		   FROM api_tokens
		  WHERE token_hash = $1`,
		tokenHash,
	)
	return scanAPITokenRecord(row)
}

func (r *APITokenRepository) Touch(ctx context.Context, id int64, usedAt time.Time) error {
	value := usedAt.UTC().Format(time.RFC3339Nano)
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE api_tokens
		    SET last_used_at = $1, updated_at = $1
		  WHERE id = $2`,
		value,
		id,
	)
	return err
}

func (r *APITokenRepository) Revoke(ctx context.Context, id int64, revokedAt time.Time) error {
	value := revokedAt.UTC().Format(time.RFC3339Nano)
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE api_tokens
		    SET revoked_at = $1, updated_at = $1
		  WHERE id = $2`,
		value,
		id,
	)
	return err
}

func (r *APITokenRepository) DeleteExpiredOrRevokedBefore(ctx context.Context, cutoff time.Time) error {
	value := cutoff.UTC().Format(time.RFC3339Nano)
	_, err := r.db.ExecContext(
		ctx,
		`DELETE FROM api_tokens
		  WHERE (expires_at <> '' AND expires_at <= $1) OR (revoked_at <> '' AND revoked_at <= $1)`,
		value,
	)
	return err
}

func scanAPITokenRecord(scanner interface{ Scan(dest ...any) error }) (APITokenRecord, error) {
	var (
		record     APITokenRecord
		scopes     string
		lastUsedAt string
		expiresAt  string
		createdAt  string
		updatedAt  string
		revokedAt  string
	)

	if err := scanner.Scan(
		&record.ID,
		&record.UserID,
		&record.Name,
		&record.TokenHash,
		&record.Prefix,
		&scopes,
		&lastUsedAt,
		&expiresAt,
		&createdAt,
		&updatedAt,
		&revokedAt,
	); err != nil {
		return APITokenRecord{}, err
	}
	if err := fillAPIToken(&record.APIToken, scopes, lastUsedAt, expiresAt, createdAt, updatedAt, revokedAt); err != nil {
		return APITokenRecord{}, err
	}
	return record, nil
}

func scanAPIToken(scanner interface{ Scan(dest ...any) error }) (domain.APIToken, error) {
	var (
		item       domain.APIToken
		scopes     string
		lastUsedAt string
		expiresAt  string
		createdAt  string
		updatedAt  string
		revokedAt  string
	)

	if err := scanner.Scan(
		&item.ID,
		&item.UserID,
		&item.Name,
		&item.Prefix,
		&scopes,
		&lastUsedAt,
		&expiresAt,
		&createdAt,
		&updatedAt,
		&revokedAt,
	); err != nil {
		return domain.APIToken{}, err
	}
	if err := fillAPIToken(&item, scopes, lastUsedAt, expiresAt, createdAt, updatedAt, revokedAt); err != nil {
		return domain.APIToken{}, err
	}
	return item, nil
}

func normalizeAPITokenScopes(scopes []string) []string {
	seen := map[string]struct{}{}
	items := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		items = append(items, scope)
	}
	if len(items) == 0 {
		return []string{domain.ScopeAll}
	}
	return items
}

func normalizeAPITokenScopesFromJSON(raw string) []string {
	var scopes []string
	if strings.TrimSpace(raw) == "" || json.Unmarshal([]byte(raw), &scopes) != nil {
		return []string{domain.ScopeAll}
	}
	return normalizeAPITokenScopes(scopes)
}

func fillAPIToken(item *domain.APIToken, scopes string, lastUsedAt string, expiresAt string, createdAt string, updatedAt string, revokedAt string) error {
	item.Scopes = normalizeAPITokenScopesFromJSON(scopes)
	created, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return err
	}
	updated, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return err
	}
	item.CreatedAt = created
	item.UpdatedAt = updated
	item.LastUsedAt = nil
	item.ExpiresAt = nil
	item.RevokedAt = nil
	item.IsActive = revokedAt == ""
	if lastUsedAt != "" {
		parsed, err := time.Parse(time.RFC3339Nano, lastUsedAt)
		if err != nil {
			return err
		}
		item.LastUsedAt = &parsed
	}
	if expiresAt != "" {
		parsed, err := time.Parse(time.RFC3339Nano, expiresAt)
		if err != nil {
			return err
		}
		item.ExpiresAt = &parsed
		if !parsed.After(time.Now().UTC()) {
			item.IsActive = false
		}
	}
	if revokedAt != "" {
		parsed, err := time.Parse(time.RFC3339Nano, revokedAt)
		if err != nil {
			return err
		}
		item.RevokedAt = &parsed
		item.IsActive = false
	}
	return nil
}
