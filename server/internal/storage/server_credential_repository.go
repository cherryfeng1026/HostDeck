package storage

import (
	"context"
	"database/sql"
	"time"
)

type ServerCredential struct {
	ServerID           int64
	AuthType           string
	PasswordCiphertext string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type ServerCredentialRepository struct {
	db *sql.DB
}

func NewServerCredentialRepository(db *sql.DB) *ServerCredentialRepository {
	return &ServerCredentialRepository{db: db}
}

func (r *ServerCredentialRepository) UpsertPassword(ctx context.Context, serverID int64, authType string, ciphertext string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO server_credentials (
			server_id, auth_type, password_ciphertext, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (server_id) DO UPDATE SET
			auth_type = excluded.auth_type,
			password_ciphertext = excluded.password_ciphertext,
			updated_at = excluded.updated_at`,
		serverID,
		authType,
		ciphertext,
		now,
		now,
	)
	return err
}

func (r *ServerCredentialRepository) GetByServerID(ctx context.Context, serverID int64) (ServerCredential, error) {
	var (
		item      ServerCredential
		createdAt string
		updatedAt string
	)

	err := r.db.QueryRowContext(
		ctx,
		`SELECT server_id, auth_type, password_ciphertext, created_at, updated_at
		FROM server_credentials
		WHERE server_id = $1`,
		serverID,
	).Scan(&item.ServerID, &item.AuthType, &item.PasswordCiphertext, &createdAt, &updatedAt)
	if err != nil {
		return ServerCredential{}, err
	}

	item.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return ServerCredential{}, err
	}
	item.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return ServerCredential{}, err
	}

	return item, nil
}
