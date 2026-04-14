package storage

import (
	"context"
	"database/sql"
	"time"

	"hostdeck/server/internal/domain"
)

type UserRecord struct {
	domain.User
	PasswordHash string
}

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Count(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}

func (r *UserRepository) Create(ctx context.Context, username string, passwordHash string, role string) (domain.User, error) {
	if role == "" {
		role = domain.RoleAdmin
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	var (
		user        domain.User
		lastLoginAt string
		createdAt   string
		updatedAt   string
	)

	err := r.db.QueryRowContext(
		ctx,
		`INSERT INTO users (username, password_hash, role, last_login_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, username, role, last_login_at, created_at, updated_at`,
		username,
		passwordHash,
		role,
		"",
		now,
		now,
	).Scan(
		&user.ID,
		&user.Username,
		&user.Role,
		&lastLoginAt,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return domain.User{}, err
	}

	if err := fillUserTimestamps(&user, lastLoginAt, createdAt, updatedAt); err != nil {
		return domain.User{}, err
	}
	return user, nil
}

func (r *UserRepository) GetByUsername(ctx context.Context, username string) (UserRecord, error) {
	row := r.db.QueryRowContext(
		ctx,
		`SELECT id, username, password_hash, role, last_login_at, created_at, updated_at
		   FROM users
		  WHERE username = $1`,
		username,
	)
	return scanUserRecord(row)
}

func (r *UserRepository) GetByID(ctx context.Context, id int64) (UserRecord, error) {
	row := r.db.QueryRowContext(
		ctx,
		`SELECT id, username, password_hash, role, last_login_at, created_at, updated_at
		   FROM users
		  WHERE id = $1`,
		id,
	)
	return scanUserRecord(row)
}

func (r *UserRepository) List(ctx context.Context) ([]domain.User, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, username, role, last_login_at, created_at, updated_at
		   FROM users
		  ORDER BY id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.User, 0)
	for rows.Next() {
		var (
			user        domain.User
			lastLoginAt string
			createdAt   string
			updatedAt   string
		)
		if err := rows.Scan(&user.ID, &user.Username, &user.Role, &lastLoginAt, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if err := fillUserTimestamps(&user, lastLoginAt, createdAt, updatedAt); err != nil {
			return nil, err
		}
		items = append(items, user)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *UserRepository) UpdateLastLoginAt(ctx context.Context, id int64, at time.Time) error {
	value := at.UTC().Format(time.RFC3339Nano)
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE users
		    SET last_login_at = $1, updated_at = $1
		  WHERE id = $2`,
		value,
		id,
	)
	return err
}

func (r *UserRepository) UpdatePassword(ctx context.Context, id int64, passwordHash string, updatedAt time.Time) error {
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE users
		    SET password_hash = $1, updated_at = $2
		  WHERE id = $3`,
		passwordHash,
		updatedAt.UTC().Format(time.RFC3339Nano),
		id,
	)
	return err
}

func scanUserRecord(scanner interface {
	Scan(dest ...any) error
}) (UserRecord, error) {
	var (
		record      UserRecord
		lastLoginAt string
		createdAt   string
		updatedAt   string
	)

	if err := scanner.Scan(
		&record.ID,
		&record.Username,
		&record.PasswordHash,
		&record.Role,
		&lastLoginAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return UserRecord{}, err
	}

	if err := fillUserTimestamps(&record.User, lastLoginAt, createdAt, updatedAt); err != nil {
		return UserRecord{}, err
	}
	return record, nil
}

func fillUserTimestamps(user *domain.User, lastLoginAt string, createdAt string, updatedAt string) error {
	created, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return err
	}
	updated, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return err
	}

	user.CreatedAt = created
	user.UpdatedAt = updated
	if lastLoginAt == "" {
		user.LastLoginAt = nil
		return nil
	}

	lastLogin, err := time.Parse(time.RFC3339Nano, lastLoginAt)
	if err != nil {
		return err
	}
	user.LastLoginAt = &lastLogin
	return nil
}
