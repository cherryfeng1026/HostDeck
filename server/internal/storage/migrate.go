package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

const createServersTableSQLiteSQL = `
CREATE TABLE IF NOT EXISTS servers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    hostname TEXT NOT NULL,
    ip TEXT NOT NULL UNIQUE,
    ssh_port INTEGER NOT NULL,
    username TEXT NOT NULL,
    auth_type TEXT NOT NULL DEFAULT 'password',
    credential_ref TEXT NOT NULL DEFAULT '',
    collector_mode TEXT NOT NULL DEFAULT 'ssh_only',
    tags TEXT NOT NULL DEFAULT '[]',
    purpose TEXT NOT NULL DEFAULT '',
    remark TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
`

const createServerStatusLatestTableSQLiteSQL = `
CREATE TABLE IF NOT EXISTS server_status_latest (
    server_id INTEGER PRIMARY KEY,
    online INTEGER NOT NULL,
    ssh_ok INTEGER NOT NULL,
    cpu_usage REAL NOT NULL,
    memory_usage REAL NOT NULL,
    disk_usage REAL NOT NULL,
    os_version TEXT NOT NULL,
    kernel_version TEXT NOT NULL,
    uptime_seconds INTEGER NOT NULL,
    load_1 REAL NOT NULL,
    load_5 REAL NOT NULL,
    load_15 REAL NOT NULL,
    last_report_at TEXT NOT NULL,
    source TEXT NOT NULL
);
`

const createServerStatusHistoryTableSQLiteSQL = `
CREATE TABLE IF NOT EXISTS server_status_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    server_id INTEGER NOT NULL,
    sampled_at TEXT NOT NULL,
    cpu_usage REAL NOT NULL,
    memory_usage REAL NOT NULL,
    disk_usage REAL NOT NULL,
    load_1 REAL NOT NULL,
    load_5 REAL NOT NULL,
    load_15 REAL NOT NULL
);
`

const createCommandLogsTableSQLiteSQL = `
CREATE TABLE IF NOT EXISTS command_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    server_id INTEGER NOT NULL,
    command TEXT NOT NULL,
    stdout TEXT NOT NULL,
    stderr TEXT NOT NULL,
    exit_code INTEGER NOT NULL,
    duration_ms INTEGER NOT NULL,
    executed_at TEXT NOT NULL
);
`

const createAlertRulesTableSQLiteSQL = `
CREATE TABLE IF NOT EXISTS alert_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    metric TEXT NOT NULL,
    operator TEXT NOT NULL,
    threshold REAL NOT NULL,
    duration_seconds INTEGER NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
`

const createServerCredentialsTableSQLiteSQL = `
CREATE TABLE IF NOT EXISTS server_credentials (
    server_id INTEGER PRIMARY KEY,
    auth_type TEXT NOT NULL DEFAULT 'password',
    password_ciphertext TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
`

const createUsersTableSQLiteSQL = `
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'admin',
    last_login_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
`

const createUserSessionsTableSQLiteSQL = `
CREATE TABLE IF NOT EXISTS user_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    ip TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT ''
);
`

const createAuthEventsTableSQLiteSQL = `
CREATE TABLE IF NOT EXISTS auth_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL DEFAULT 0,
    username TEXT NOT NULL DEFAULT '',
    event_type TEXT NOT NULL,
    detail TEXT NOT NULL DEFAULT '',
    ip TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);
`

const createServersTablePostgresSQL = `
CREATE TABLE IF NOT EXISTS servers (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    hostname TEXT NOT NULL,
    ip TEXT NOT NULL UNIQUE,
    ssh_port INTEGER NOT NULL,
    username TEXT NOT NULL,
    auth_type TEXT NOT NULL DEFAULT 'password',
    credential_ref TEXT NOT NULL DEFAULT '',
    collector_mode TEXT NOT NULL DEFAULT 'ssh_only',
    tags TEXT NOT NULL DEFAULT '[]',
    purpose TEXT NOT NULL DEFAULT '',
    remark TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
`

const createServerStatusLatestTablePostgresSQL = `
CREATE TABLE IF NOT EXISTS server_status_latest (
    server_id BIGINT PRIMARY KEY,
    online INTEGER NOT NULL,
    ssh_ok INTEGER NOT NULL,
    cpu_usage REAL NOT NULL,
    memory_usage REAL NOT NULL,
    disk_usage REAL NOT NULL,
    os_version TEXT NOT NULL,
    kernel_version TEXT NOT NULL,
    uptime_seconds BIGINT NOT NULL,
    load_1 REAL NOT NULL,
    load_5 REAL NOT NULL,
    load_15 REAL NOT NULL,
    last_report_at TEXT NOT NULL,
    source TEXT NOT NULL
);
`

const createServerStatusHistoryTablePostgresSQL = `
CREATE TABLE IF NOT EXISTS server_status_history (
    id BIGSERIAL PRIMARY KEY,
    server_id BIGINT NOT NULL,
    sampled_at TEXT NOT NULL,
    cpu_usage REAL NOT NULL,
    memory_usage REAL NOT NULL,
    disk_usage REAL NOT NULL,
    load_1 REAL NOT NULL,
    load_5 REAL NOT NULL,
    load_15 REAL NOT NULL
);
`

const createCommandLogsTablePostgresSQL = `
CREATE TABLE IF NOT EXISTS command_logs (
    id BIGSERIAL PRIMARY KEY,
    server_id BIGINT NOT NULL,
    command TEXT NOT NULL,
    stdout TEXT NOT NULL,
    stderr TEXT NOT NULL,
    exit_code INTEGER NOT NULL,
    duration_ms BIGINT NOT NULL,
    executed_at TEXT NOT NULL
);
`

const createAlertRulesTablePostgresSQL = `
CREATE TABLE IF NOT EXISTS alert_rules (
    id BIGSERIAL PRIMARY KEY,
    metric TEXT NOT NULL,
    operator TEXT NOT NULL,
    threshold REAL NOT NULL,
    duration_seconds INTEGER NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
`

const createServerCredentialsTablePostgresSQL = `
CREATE TABLE IF NOT EXISTS server_credentials (
    server_id BIGINT PRIMARY KEY,
    auth_type TEXT NOT NULL DEFAULT 'password',
    password_ciphertext TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
`

const createUsersTablePostgresSQL = `
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'admin',
    last_login_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
`

const createUserSessionsTablePostgresSQL = `
CREATE TABLE IF NOT EXISTS user_sessions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    ip TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT ''
);
`

const createAuthEventsTablePostgresSQL = `
CREATE TABLE IF NOT EXISTS auth_events (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL DEFAULT 0,
    username TEXT NOT NULL DEFAULT '',
    event_type TEXT NOT NULL,
    detail TEXT NOT NULL DEFAULT '',
    ip TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);
`

const createUserSessionsExpiresIndexSQLiteSQL = `
CREATE INDEX IF NOT EXISTS idx_user_sessions_expires_at ON user_sessions (expires_at);
`

const createAuthEventsCreatedIndexSQLiteSQL = `
CREATE INDEX IF NOT EXISTS idx_auth_events_created_at ON auth_events (created_at);
`

const createCommandLogsExecutedIndexSQLiteSQL = `
CREATE INDEX IF NOT EXISTS idx_command_logs_executed_at ON command_logs (executed_at);
`

const createUserSessionsExpiresIndexPostgresSQL = `
CREATE INDEX IF NOT EXISTS idx_user_sessions_expires_at ON user_sessions (expires_at);
`

const createAuthEventsCreatedIndexPostgresSQL = `
CREATE INDEX IF NOT EXISTS idx_auth_events_created_at ON auth_events (created_at);
`

const createCommandLogsExecutedIndexPostgresSQL = `
CREATE INDEX IF NOT EXISTS idx_command_logs_executed_at ON command_logs (executed_at);
`

func Migrate(ctx context.Context, db *sql.DB) error {
	return MigrateWithDriver(ctx, db, DriverSQLite)
}

func MigrateWithDriver(ctx context.Context, db *sql.DB, driver string) error {
	if db == nil {
		return errors.New("db is required")
	}

	statements, err := migrationStatements(driver)
	if err != nil {
		return err
	}

	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func migrationStatements(driver string) ([]string, error) {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "", DriverSQLite:
		return []string{
			createServersTableSQLiteSQL,
			createServerCredentialsTableSQLiteSQL,
			createServerStatusLatestTableSQLiteSQL,
			createUsersTableSQLiteSQL,
			createUserSessionsTableSQLiteSQL,
			createAuthEventsTableSQLiteSQL,
			createServerStatusHistoryTableSQLiteSQL,
			createCommandLogsTableSQLiteSQL,
			createAlertRulesTableSQLiteSQL,
			createUserSessionsExpiresIndexSQLiteSQL,
			createAuthEventsCreatedIndexSQLiteSQL,
			createCommandLogsExecutedIndexSQLiteSQL,
		}, nil
	case DriverPostgres:
		return []string{
			createServersTablePostgresSQL,
			createServerCredentialsTablePostgresSQL,
			createServerStatusLatestTablePostgresSQL,
			createServerStatusHistoryTablePostgresSQL,
			createCommandLogsTablePostgresSQL,
			createUsersTablePostgresSQL,
			createUserSessionsTablePostgresSQL,
			createAuthEventsTablePostgresSQL,
			createAlertRulesTablePostgresSQL,
			createUserSessionsExpiresIndexPostgresSQL,
			createAuthEventsCreatedIndexPostgresSQL,
			createCommandLogsExecutedIndexPostgresSQL,
		}, nil
	default:
		return nil, errors.New("unsupported db driver")
	}
}
