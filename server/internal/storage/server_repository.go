package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"hostdeck/server/internal/credential"
	"hostdeck/server/internal/domain"
)

type ServerFilter struct {
	ID            int64
	Keyword       string
	Tag           string
	CollectorMode string
}

type ServerRepository struct {
	db        *sql.DB
	masterKey string
}

var ErrServerIPConflict = errors.New("服务器 IP 已存在")
var ErrServerNotFound = errors.New("服务器不存在")
var ErrServerCredentialRequired = errors.New("服务器 SSH 凭据不能为空")

type ServerIPConflictError struct {
	IP string
}

func (e ServerIPConflictError) Error() string {
	return fmt.Sprintf("%s: %s", ErrServerIPConflict.Error(), e.IP)
}

func (e ServerIPConflictError) Is(target error) bool {
	return target == ErrServerIPConflict
}

func NewServerRepository(db *sql.DB, masterKey ...string) *ServerRepository {
	repo := &ServerRepository{db: db}
	if len(masterKey) > 0 {
		repo.masterKey = strings.TrimSpace(masterKey[0])
	}
	return repo
}

func (r *ServerRepository) Create(ctx context.Context, item domain.Server) error {
	tags, err := marshalTags(item.Tags)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	if item.SSHPort == 0 {
		item.SSHPort = 22
	}
	if item.AuthType == "" {
		item.AuthType = "password"
	}
	item.CollectorMode = domain.NormalizeCollectorMode(item.CollectorMode)
	item.TrustedHostKeyFingerprint = strings.TrimSpace(item.TrustedHostKeyFingerprint)

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var serverID int64
	err = tx.QueryRowContext(
		ctx,
		`INSERT INTO servers (
			name, hostname, ip, ssh_port, username, auth_type, trusted_host_key_fingerprint,
			collector_mode, tags, purpose, remark, expires_at, maintenance_start_at, maintenance_end_at,
			enabled, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		RETURNING id`,
		item.Name,
		item.Hostname,
		item.IP,
		item.SSHPort,
		item.Username,
		item.AuthType,
		item.TrustedHostKeyFingerprint,
		item.CollectorMode,
		tags,
		item.Purpose,
		item.Remark,
		nullableRFC3339(item.ExpiresAt),
		nullableRFC3339(item.MaintenanceStartAt),
		nullableRFC3339(item.MaintenanceEndAt),
		boolToInt(item.Enabled),
		now.Format(time.RFC3339Nano),
		now.Format(time.RFC3339Nano),
	).Scan(&serverID)
	if err != nil {
		return wrapServerMutationError(err, item.IP)
	}

	if err := r.syncCredentialTx(ctx, tx, serverID, item); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (r *ServerRepository) List(ctx context.Context, filter ServerFilter) ([]domain.Server, error) {
	query := strings.Builder{}
	query.WriteString(`SELECT
		s.id,
		s.name,
		s.hostname,
		s.ip,
		s.ssh_port,
		s.username,
		s.auth_type,
		s.trusted_host_key_fingerprint,
		s.collector_mode,
		s.tags,
		s.purpose,
		s.remark,
		s.expires_at,
		s.maintenance_start_at,
		s.maintenance_end_at,
		s.enabled,
		s.created_at,
		s.updated_at,
		CASE
			WHEN sc.server_id IS NOT NULL AND sc.password_ciphertext <> '' THEN 1
			ELSE 0
		END AS password_configured,
		CASE
			WHEN sc.server_id IS NOT NULL AND sc.private_key_ciphertext <> '' THEN 1
			ELSE 0
		END AS private_key_configured
	FROM servers s
	LEFT JOIN server_credentials sc ON sc.server_id = s.id`)

	args := make([]any, 0, 4)
	clauses := make([]string, 0, 4)
	nextIndex := 1
	if filter.ID > 0 {
		clauses = append(clauses, fmt.Sprintf("s.id = $%d", nextIndex))
		args = append(args, filter.ID)
		nextIndex++
	}
	if filter.Keyword != "" {
		clauses = append(clauses, fmt.Sprintf("(s.name LIKE $%d OR s.hostname LIKE $%d OR s.ip LIKE $%d)", nextIndex, nextIndex+1, nextIndex+2))
		keyword := "%" + filter.Keyword + "%"
		args = append(args, keyword, keyword, keyword)
		nextIndex += 3
	}
	if filter.Tag != "" {
		clauses = append(clauses, fmt.Sprintf("s.tags LIKE $%d", nextIndex))
		args = append(args, "%\""+filter.Tag+"\"%")
		nextIndex++
	}
	if filter.CollectorMode != "" {
		clauses = append(clauses, fmt.Sprintf("s.collector_mode = $%d", nextIndex))
		args = append(args, filter.CollectorMode)
		nextIndex++
	}
	if len(clauses) > 0 {
		query.WriteString(" WHERE ")
		query.WriteString(strings.Join(clauses, " AND "))
	}
	query.WriteString(" ORDER BY id ASC")

	rows, err := r.db.QueryContext(ctx, query.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.Server, 0)
	for rows.Next() {
		item, err := scanServer(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

func (r *ServerRepository) Update(ctx context.Context, item domain.Server) error {
	tags, err := marshalTags(item.Tags)
	if err != nil {
		return err
	}

	if item.SSHPort == 0 {
		item.SSHPort = 22
	}
	if item.AuthType == "" {
		item.AuthType = "password"
	}
	item.CollectorMode = domain.NormalizeCollectorMode(item.CollectorMode)
	item.TrustedHostKeyFingerprint = strings.TrimSpace(item.TrustedHostKeyFingerprint)

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	result, err := tx.ExecContext(
		ctx,
		`UPDATE servers
		SET name = $1, hostname = $2, ip = $3, ssh_port = $4, username = $5, auth_type = $6,
		    trusted_host_key_fingerprint = $7, collector_mode = $8, tags = $9, purpose = $10, remark = $11,
		    expires_at = $12, maintenance_start_at = $13, maintenance_end_at = $14, enabled = $15, updated_at = $16
		WHERE id = $17`,
		item.Name,
		item.Hostname,
		item.IP,
		item.SSHPort,
		item.Username,
		item.AuthType,
		item.TrustedHostKeyFingerprint,
		item.CollectorMode,
		tags,
		item.Purpose,
		item.Remark,
		nullableRFC3339(item.ExpiresAt),
		nullableRFC3339(item.MaintenanceStartAt),
		nullableRFC3339(item.MaintenanceEndAt),
		boolToInt(item.Enabled),
		time.Now().UTC().Format(time.RFC3339Nano),
		item.ID,
	)
	if err != nil {
		return wrapServerMutationError(err, item.IP)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrServerNotFound
	}

	if err := r.syncCredentialTx(ctx, tx, item.ID, item); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (r *ServerRepository) UpdateTrustedHostKeyFingerprint(ctx context.Context, id int64, fingerprint string) error {
	result, err := r.db.ExecContext(
		ctx,
		`UPDATE servers SET trusted_host_key_fingerprint = $1, updated_at = $2 WHERE id = $3`,
		strings.TrimSpace(fingerprint),
		time.Now().UTC().Format(time.RFC3339Nano),
		id,
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *ServerRepository) Delete(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	statements := []string{
		`DELETE FROM server_credentials WHERE server_id = $1`,
		`DELETE FROM server_status_latest WHERE server_id = $1`,
		`DELETE FROM alert_states WHERE server_id = $1`,
		`DELETE FROM servers WHERE id = $1`,
	}
	for _, statement := range statements[:len(statements)-1] {
		if _, err := tx.ExecContext(ctx, statement, id); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, statements[len(statements)-1], id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrServerNotFound
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func scanServer(scanner interface {
	Scan(dest ...any) error
}) (domain.Server, error) {
	var (
		item                 domain.Server
		rawTags              string
		rawCreate            string
		rawUpdate            string
		rawExpires           sql.NullString
		rawMaintenanceStart  sql.NullString
		rawMaintenanceEnd    sql.NullString
		enabled              int
		passwordConfigured   int
		privateKeyConfigured int
	)

	err := scanner.Scan(
		&item.ID,
		&item.Name,
		&item.Hostname,
		&item.IP,
		&item.SSHPort,
		&item.Username,
		&item.AuthType,
		&item.TrustedHostKeyFingerprint,
		&item.CollectorMode,
		&rawTags,
		&item.Purpose,
		&item.Remark,
		&rawExpires,
		&rawMaintenanceStart,
		&rawMaintenanceEnd,
		&enabled,
		&rawCreate,
		&rawUpdate,
		&passwordConfigured,
		&privateKeyConfigured,
	)
	if err != nil {
		return domain.Server{}, err
	}

	item.Tags, err = unmarshalTags(rawTags)
	if err != nil {
		return domain.Server{}, err
	}
	item.CollectorMode = domain.NormalizeCollectorMode(item.CollectorMode)
	item.Enabled = enabled == 1
	item.PasswordConfigured = passwordConfigured == 1
	item.PrivateKeyConfigured = privateKeyConfigured == 1
	item.ExpiresAt, err = parseNullableRFC3339(rawExpires)
	if err != nil {
		return domain.Server{}, fmt.Errorf("parse expires_at: %w", err)
	}
	item.MaintenanceStartAt, err = parseNullableRFC3339(rawMaintenanceStart)
	if err != nil {
		return domain.Server{}, fmt.Errorf("parse maintenance_start_at: %w", err)
	}
	item.MaintenanceEndAt, err = parseNullableRFC3339(rawMaintenanceEnd)
	if err != nil {
		return domain.Server{}, fmt.Errorf("parse maintenance_end_at: %w", err)
	}
	item.CreatedAt, err = time.Parse(time.RFC3339Nano, rawCreate)
	if err != nil {
		return domain.Server{}, fmt.Errorf("parse created_at: %w", err)
	}
	item.UpdatedAt, err = time.Parse(time.RFC3339Nano, rawUpdate)
	if err != nil {
		return domain.Server{}, fmt.Errorf("parse updated_at: %w", err)
	}

	return item, nil
}

func marshalTags(tags []string) (string, error) {
	if len(tags) == 0 {
		tags = []string{}
	}

	raw, err := json.Marshal(tags)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func unmarshalTags(raw string) ([]string, error) {
	if raw == "" {
		return []string{}, nil
	}

	var tags []string
	if err := json.Unmarshal([]byte(raw), &tags); err != nil {
		return nil, err
	}
	return tags, nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func parseNullableRFC3339(value sql.NullString) (*time.Time, error) {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value.String)
	if err != nil {
		return nil, err
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func wrapServerMutationError(err error, ip string) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ServerIPConflictError{IP: ip}
	}
	if strings.Contains(err.Error(), "UNIQUE constraint failed: servers.ip") {
		return ServerIPConflictError{IP: ip}
	}
	return err
}

func (r *ServerRepository) syncCredentialTx(ctx context.Context, tx *sql.Tx, serverID int64, item domain.Server) error {
	switch item.AuthType {
	case "password":
		secret := strings.TrimSpace(item.Password)
		if secret == "" {
			return requireExistingCredentialTx(ctx, tx, serverID, item.AuthType)
		}
		cipher, err := credential.NewCipher(r.masterKey)
		if err != nil {
			return err
		}
		ciphertext, err := cipher.Encrypt(secret)
		if err != nil {
			return err
		}
		return upsertCredentialTx(ctx, tx, serverID, item.AuthType, ciphertext, "")
	case "private_key":
		secret := strings.TrimSpace(item.PrivateKey)
		if secret == "" {
			return requireExistingCredentialTx(ctx, tx, serverID, item.AuthType)
		}
		cipher, err := credential.NewCipher(r.masterKey)
		if err != nil {
			return err
		}
		ciphertext, err := cipher.Encrypt(secret)
		if err != nil {
			return err
		}
		return upsertCredentialTx(ctx, tx, serverID, item.AuthType, "", ciphertext)
	default:
		return deleteCredentialTx(ctx, tx, serverID)
	}
}

func requireExistingCredentialTx(ctx context.Context, tx *sql.Tx, serverID int64, authType string) error {
	var (
		existingAuthType     string
		passwordCiphertext   string
		privateKeyCiphertext string
	)
	err := tx.QueryRowContext(
		ctx,
		`SELECT auth_type, password_ciphertext, private_key_ciphertext FROM server_credentials WHERE server_id = $1`,
		serverID,
	).Scan(&existingAuthType, &passwordCiphertext, &privateKeyCiphertext)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrServerCredentialRequired
	}
	if err != nil {
		return err
	}
	if existingAuthType != authType {
		return ErrServerCredentialRequired
	}
	switch authType {
	case "password":
		if strings.TrimSpace(passwordCiphertext) != "" {
			return nil
		}
	case "private_key":
		if strings.TrimSpace(privateKeyCiphertext) != "" {
			return nil
		}
	}
	return ErrServerCredentialRequired
}

func upsertCredentialTx(ctx context.Context, tx *sql.Tx, serverID int64, authType string, passwordCiphertext string, privateKeyCiphertext string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO server_credentials (
			server_id, auth_type, password_ciphertext, private_key_ciphertext, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (server_id) DO UPDATE SET
			auth_type = excluded.auth_type,
			password_ciphertext = excluded.password_ciphertext,
			private_key_ciphertext = excluded.private_key_ciphertext,
			updated_at = excluded.updated_at`,
		serverID,
		authType,
		passwordCiphertext,
		privateKeyCiphertext,
		now,
		now,
	)
	return err
}

func deleteCredentialTx(ctx context.Context, tx *sql.Tx, serverID int64) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM server_credentials WHERE server_id = $1`, serverID)
	return err
}
