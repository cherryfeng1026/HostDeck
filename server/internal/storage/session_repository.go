package storage

import (
	"context"
	"database/sql"
	"time"
)

type SessionRecord struct {
	ID         int64
	UserID     int64
	TokenHash  string
	ExpiresAt  time.Time
	CreatedAt  time.Time
	LastSeenAt time.Time
	IP         string
	UserAgent  string
}

type SessionRepository struct {
	db *sql.DB
}

func NewSessionRepository(db *sql.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

func (r *SessionRepository) Create(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time, ip string, userAgent string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO user_sessions (user_id, token_hash, expires_at, created_at, last_seen_at, ip, user_agent)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		userID,
		tokenHash,
		expiresAt.UTC().Format(time.RFC3339Nano),
		now,
		now,
		ip,
		userAgent,
	)
	return err
}

func (r *SessionRepository) GetByTokenHash(ctx context.Context, tokenHash string) (SessionRecord, error) {
	var (
		item       SessionRecord
		expiresAt  string
		createdAt  string
		lastSeenAt string
	)

	err := r.db.QueryRowContext(
		ctx,
		`SELECT id, user_id, token_hash, expires_at, created_at, last_seen_at, ip, user_agent
		   FROM user_sessions
		  WHERE token_hash = $1`,
		tokenHash,
	).Scan(
		&item.ID,
		&item.UserID,
		&item.TokenHash,
		&expiresAt,
		&createdAt,
		&lastSeenAt,
		&item.IP,
		&item.UserAgent,
	)
	if err != nil {
		return SessionRecord{}, err
	}

	var parseErr error
	item.ExpiresAt, parseErr = time.Parse(time.RFC3339Nano, expiresAt)
	if parseErr != nil {
		return SessionRecord{}, parseErr
	}
	item.CreatedAt, parseErr = time.Parse(time.RFC3339Nano, createdAt)
	if parseErr != nil {
		return SessionRecord{}, parseErr
	}
	item.LastSeenAt, parseErr = time.Parse(time.RFC3339Nano, lastSeenAt)
	if parseErr != nil {
		return SessionRecord{}, parseErr
	}
	return item, nil
}

func (r *SessionRepository) Touch(ctx context.Context, sessionID int64, lastSeenAt time.Time) error {
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE user_sessions SET last_seen_at = $1 WHERE id = $2`,
		lastSeenAt.UTC().Format(time.RFC3339Nano),
		sessionID,
	)
	return err
}

func (r *SessionRepository) DeleteByTokenHash(ctx context.Context, tokenHash string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM user_sessions WHERE token_hash = $1`, tokenHash)
	return err
}

func (r *SessionRepository) DeleteByUserID(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM user_sessions WHERE user_id = $1`, userID)
	return err
}

func (r *SessionRepository) DeleteExpired(ctx context.Context, now time.Time) error {
	_, err := r.db.ExecContext(
		ctx,
		`DELETE FROM user_sessions WHERE expires_at <= $1`,
		now.UTC().Format(time.RFC3339Nano),
	)
	return err
}
