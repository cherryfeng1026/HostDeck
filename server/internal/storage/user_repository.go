package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"hostdeck/server/internal/domain"
)

type UserRecord struct {
	domain.User
	PasswordHash string
}

type UserRepository struct {
	db *sql.DB
}

var ErrUserUsernameConflict = errors.New("用户名已存在")

type UserUsernameConflictError struct {
	Username string
}

func (e UserUsernameConflictError) Error() string {
	return fmt.Sprintf("%s: %s", ErrUserUsernameConflict.Error(), e.Username)
}

func (e UserUsernameConflictError) Is(target error) bool {
	return target == ErrUserUsernameConflict
}

type UserUpdateInput struct {
	Role    string
	Enabled *bool
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
		`INSERT INTO users (username, password_hash, role, enabled, last_login_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, username, role, enabled, last_login_at, created_at, updated_at`,
		username,
		passwordHash,
		role,
		boolToInt(true),
		"",
		now,
		now,
	).Scan(
		&user.ID,
		&user.Username,
		&user.Role,
		&user.Enabled,
		&lastLoginAt,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return domain.User{}, wrapUserMutationError(err, username)
	}

	if err := fillUserTimestamps(&user, lastLoginAt, createdAt, updatedAt); err != nil {
		return domain.User{}, err
	}
	return user, nil
}

func (r *UserRepository) GetByUsername(ctx context.Context, username string) (UserRecord, error) {
	row := r.db.QueryRowContext(
		ctx,
		`SELECT id, username, password_hash, role, enabled, last_login_at, created_at, updated_at
		   FROM users
		  WHERE username = $1`,
		username,
	)
	return scanUserRecord(row)
}

func (r *UserRepository) GetByID(ctx context.Context, id int64) (UserRecord, error) {
	row := r.db.QueryRowContext(
		ctx,
		`SELECT id, username, password_hash, role, enabled, last_login_at, created_at, updated_at
		   FROM users
		  WHERE id = $1`,
		id,
	)
	return scanUserRecord(row)
}

func (r *UserRepository) List(ctx context.Context) ([]domain.User, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, username, role, enabled, last_login_at, created_at, updated_at
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
		if err := rows.Scan(&user.ID, &user.Username, &user.Role, &user.Enabled, &lastLoginAt, &createdAt, &updatedAt); err != nil {
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

func (r *UserRepository) GetNotificationReadAt(ctx context.Context, id int64) (*time.Time, error) {
	var value string
	if err := r.db.QueryRowContext(ctx, `SELECT notification_read_at FROM users WHERE id = $1`, id).Scan(&value); err != nil {
		return nil, err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func (r *UserRepository) UpdateNotificationReadAt(ctx context.Context, id int64, readAt time.Time) error {
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE users
		    SET notification_read_at = $1, updated_at = $1
		  WHERE id = $2`,
		readAt.UTC().Format(time.RFC3339Nano),
		id,
	)
	return err
}

func wrapUserMutationError(err error, username string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "users_username") {
		return UserUsernameConflictError{Username: username}
	}
	if strings.Contains(err.Error(), "UNIQUE constraint failed: users.username") {
		return UserUsernameConflictError{Username: username}
	}
	return err
}

func (r *UserRepository) Update(ctx context.Context, id int64, input UserUpdateInput, updatedAt time.Time) error {
	fields := make([]string, 0, 3)
	args := make([]any, 0, 4)
	index := 1
	if input.Role != "" {
		fields = append(fields, "role = $1")
		args = append(args, input.Role)
		index++
	}
	if input.Enabled != nil {
		fields = append(fields, "enabled = $"+strconv.Itoa(index))
		args = append(args, boolToInt(*input.Enabled))
		index++
	}
	if len(fields) == 0 {
		return nil
	}
	fields = append(fields, "updated_at = $"+strconv.Itoa(index))
	args = append(args, updatedAt.UTC().Format(time.RFC3339Nano))
	index++
	args = append(args, id)
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE users SET `+strings.Join(fields, ", ")+` WHERE id = $`+strconv.Itoa(index),
		args...,
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
		&record.Enabled,
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
