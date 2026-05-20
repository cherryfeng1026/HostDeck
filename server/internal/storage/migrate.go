package storage

import (
	"context"
	"database/sql"
	"errors"
)

const createSchemaMigrationsTableSQL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version BIGINT PRIMARY KEY,
    applied_at TEXT NOT NULL
);
`

const createServersTableSQL = `
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
    expires_at TEXT,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
`

const createServerStatusLatestTableSQL = `
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
    source TEXT NOT NULL,
    collect_status TEXT NOT NULL DEFAULT 'unknown',
    last_collect_started_at TEXT NOT NULL DEFAULT '',
    last_collect_finished_at TEXT NOT NULL DEFAULT '',
    last_success_at TEXT NOT NULL DEFAULT '',
    last_collect_error TEXT NOT NULL DEFAULT '',
    collect_failure_count INTEGER NOT NULL DEFAULT 0,
    collect_duration_ms BIGINT NOT NULL DEFAULT 0,
    stale INTEGER NOT NULL DEFAULT 0
);
`

const createServerStatusHistoryTableSQL = `
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

const createCommandLogsTableSQL = `
CREATE TABLE IF NOT EXISTS command_logs (
    id BIGSERIAL PRIMARY KEY,
    server_id BIGINT NOT NULL,
    server_name TEXT NOT NULL DEFAULT '',
    server_ip TEXT NOT NULL DEFAULT '',
    executor_username TEXT NOT NULL DEFAULT '',
    command TEXT NOT NULL,
    stdout TEXT NOT NULL,
    stderr TEXT NOT NULL,
    exit_code INTEGER NOT NULL,
    duration_ms BIGINT NOT NULL,
    executed_at TIMESTAMPTZ NOT NULL,
    source TEXT NOT NULL DEFAULT 'custom',
    template_id TEXT NOT NULL DEFAULT '',
    risk_level TEXT NOT NULL DEFAULT 'normal',
    risk_confirmed INTEGER NOT NULL DEFAULT 0,
    executor_auth_method TEXT NOT NULL DEFAULT '',
    request_id TEXT NOT NULL DEFAULT ''
);
`

const createAlertRulesTableSQL = `
CREATE TABLE IF NOT EXISTS alert_rules (
    id BIGSERIAL PRIMARY KEY,
    metric TEXT NOT NULL,
    operator TEXT NOT NULL,
    threshold REAL NOT NULL,
    duration_seconds INTEGER NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    scope_type TEXT NOT NULL DEFAULT 'all',
    scope_value TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
`

const createAlertStatesTableSQL = `
CREATE TABLE IF NOT EXISTS alert_states (
    id BIGSERIAL PRIMARY KEY,
    rule_id BIGINT NOT NULL,
    server_id BIGINT NOT NULL,
    metric TEXT NOT NULL,
    operator TEXT NOT NULL,
    threshold REAL NOT NULL,
    current_value REAL NOT NULL,
    severity TEXT NOT NULL,
    message TEXT NOT NULL,
    status TEXT NOT NULL,
    duration_seconds INTEGER NOT NULL,
    first_triggered_at TEXT NOT NULL,
    last_triggered_at TEXT NOT NULL,
    acknowledged_at TEXT,
    acknowledged_by TEXT NOT NULL DEFAULT '',
    muted_until TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(rule_id, server_id)
);
`

const createAlertHistoryTableSQL = `
CREATE TABLE IF NOT EXISTS alert_history (
    id BIGSERIAL PRIMARY KEY,
    alert_id BIGINT NOT NULL,
    rule_id BIGINT NOT NULL,
    server_id BIGINT NOT NULL,
    event_type TEXT NOT NULL,
    metric TEXT NOT NULL,
    operator TEXT NOT NULL,
    threshold REAL NOT NULL,
    current_value REAL NOT NULL,
    severity TEXT NOT NULL,
    message TEXT NOT NULL,
    status TEXT NOT NULL,
    triggered_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    actor_username TEXT NOT NULL DEFAULT '',
    detail TEXT NOT NULL DEFAULT ''
);
`

const createServerCredentialsTableSQL = `
CREATE TABLE IF NOT EXISTS server_credentials (
    server_id BIGINT PRIMARY KEY,
    auth_type TEXT NOT NULL DEFAULT 'password',
    password_ciphertext TEXT NOT NULL DEFAULT '',
    private_key_ciphertext TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
`

const createUsersTableSQL = `
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'admin',
    enabled INTEGER NOT NULL DEFAULT 1,
    last_login_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
`

const createUserSessionsTableSQL = `
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

const createAuthEventsTableSQL = `
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

const createAPITokensTableSQL = `
CREATE TABLE IF NOT EXISTS api_tokens (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    token_prefix TEXT NOT NULL,
    scopes TEXT NOT NULL DEFAULT '[]',
    last_used_at TEXT NOT NULL DEFAULT '',
    expires_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    revoked_at TEXT NOT NULL DEFAULT ''
);
`

const createAuditEventsTableSQL = `
CREATE TABLE IF NOT EXISTS audit_events (
    id BIGSERIAL PRIMARY KEY,
    kind TEXT NOT NULL,
    severity TEXT NOT NULL DEFAULT 'info',
    title TEXT NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    server_id BIGINT NOT NULL DEFAULT 0,
    server_name TEXT NOT NULL DEFAULT '',
    username TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);
`

const createAlertNotificationSettingsTableSQL = `
CREATE TABLE IF NOT EXISTS alert_notification_settings (
    id BIGSERIAL PRIMARY KEY,
    singleton SMALLINT NOT NULL DEFAULT 1 CHECK (singleton = 1),
    enabled INTEGER NOT NULL DEFAULT 0,
    webhook_url TEXT NOT NULL DEFAULT '',
    webhook_timeout_seconds INTEGER NOT NULL DEFAULT 5,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
`

const createCommandTemplatesTableSQL = `
CREATE TABLE IF NOT EXISTS command_templates (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    command_text TEXT NOT NULL,
    scope TEXT NOT NULL,
    risk_level TEXT NOT NULL DEFAULT 'normal',
    created_by TEXT NOT NULL DEFAULT '',
    variables_json TEXT NOT NULL DEFAULT '[]',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
`

const createCommandTemplateFavoritesTableSQL = `
CREATE TABLE IF NOT EXISTS command_template_favorites (
    template_id TEXT NOT NULL,
    username TEXT NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY (template_id, username)
);
`

const createCommandTemplatesScopeIndexSQL = `
CREATE INDEX IF NOT EXISTS idx_command_templates_scope_created_by ON command_templates (scope, created_by, updated_at DESC);
`

const createCommandTemplateFavoritesUsernameIndexSQL = `
CREATE INDEX IF NOT EXISTS idx_command_template_favorites_username ON command_template_favorites (username, created_at DESC);
`

const createServerStatusHistorySampledAtIndexSQL = `
CREATE INDEX IF NOT EXISTS idx_server_status_history_sampled_at ON server_status_history (sampled_at);
`

const createServerStatusHistoryServerSampledAtIndexSQL = `
CREATE INDEX IF NOT EXISTS idx_server_status_history_server_sampled_at ON server_status_history (server_id, sampled_at);
`

const normalizeAlertNotificationSettingsSingletonSQL = `
DELETE FROM alert_notification_settings
 WHERE id NOT IN (
    SELECT id FROM alert_notification_settings ORDER BY id ASC LIMIT 1
 );
ALTER TABLE alert_notification_settings ADD COLUMN IF NOT EXISTS singleton SMALLINT NOT NULL DEFAULT 1;
UPDATE alert_notification_settings SET singleton = 1;
CREATE UNIQUE INDEX IF NOT EXISTS idx_alert_notification_settings_singleton ON alert_notification_settings (singleton);
`

const addServersTrustedHostKeyFingerprintSQL = `
ALTER TABLE servers ADD COLUMN IF NOT EXISTS trusted_host_key_fingerprint TEXT NOT NULL DEFAULT '';
`

const addServersMaintenanceWindowSQL = `
ALTER TABLE servers
    ADD COLUMN IF NOT EXISTS maintenance_start_at TEXT,
    ADD COLUMN IF NOT EXISTS maintenance_end_at TEXT;
`

const addServerPrivateKeyAndExpiresAtSQL = `
ALTER TABLE server_credentials ADD COLUMN IF NOT EXISTS private_key_ciphertext TEXT NOT NULL DEFAULT '';
ALTER TABLE servers ADD COLUMN IF NOT EXISTS expires_at TEXT;
`

const createUserSessionsExpiresIndexSQL = `
CREATE INDEX IF NOT EXISTS idx_user_sessions_expires_at ON user_sessions (expires_at);
`

const createAuthEventsCreatedIndexSQL = `
CREATE INDEX IF NOT EXISTS idx_auth_events_created_at ON auth_events (created_at);
`

const createCommandLogsExecutedIndexSQL = `
CREATE INDEX IF NOT EXISTS idx_command_logs_executed_at ON command_logs (executed_at);
`

const addCommandLogsExecutorUsernameSQL = `
ALTER TABLE command_logs ADD COLUMN IF NOT EXISTS executor_username TEXT NOT NULL DEFAULT '';
`

const createCommandLogsServerExecutedIndexSQL = `
CREATE INDEX IF NOT EXISTS idx_command_logs_server_executed_at ON command_logs (server_id, executed_at);
`

const createCommandLogsExecutorExecutedIndexSQL = `
CREATE INDEX IF NOT EXISTS idx_command_logs_executor_executed_at ON command_logs (executor_username, executed_at);
`

const alterCommandLogsExecutedAtToTimestamptzSQL = `
ALTER TABLE command_logs
    ALTER COLUMN executed_at TYPE TIMESTAMPTZ
    USING executed_at::timestamptz;
`

const createAuditEventsCreatedIndexSQL = `
CREATE INDEX IF NOT EXISTS idx_audit_events_created_at ON audit_events (created_at);
`

const addUsersEnabledSQL = `
ALTER TABLE users ADD COLUMN IF NOT EXISTS enabled INTEGER NOT NULL DEFAULT 1;
`

const addUsersNotificationReadAtSQL = `
ALTER TABLE users ADD COLUMN IF NOT EXISTS notification_read_at TEXT NOT NULL DEFAULT '';
`

const createAPITokensUserActiveIndexSQL = `
CREATE INDEX IF NOT EXISTS idx_api_tokens_user_active ON api_tokens (user_id, revoked_at, created_at DESC);
`

const createAPITokensHashIndexSQL = `
CREATE UNIQUE INDEX IF NOT EXISTS idx_api_tokens_token_hash ON api_tokens (token_hash);
`

const addCommandLogsServerSnapshotSQL = `
ALTER TABLE command_logs ADD COLUMN IF NOT EXISTS server_name TEXT NOT NULL DEFAULT '';
ALTER TABLE command_logs ADD COLUMN IF NOT EXISTS server_ip TEXT NOT NULL DEFAULT '';
ALTER TABLE alert_history ADD COLUMN IF NOT EXISTS server_name TEXT NOT NULL DEFAULT '';
UPDATE command_logs c SET server_name = COALESCE(s.name, ''), server_ip = COALESCE(s.ip, '') FROM servers s WHERE c.server_id = s.id AND (c.server_name = '' OR c.server_ip = '');
UPDATE alert_history h SET server_name = COALESCE(s.name, '') FROM servers s WHERE h.server_id = s.id AND h.server_name = '';
CREATE INDEX IF NOT EXISTS idx_command_logs_server_name_executed_at ON command_logs (server_name, executed_at DESC);
`

const addAPITokensScopesSQL = `
ALTER TABLE api_tokens ADD COLUMN IF NOT EXISTS scopes TEXT NOT NULL DEFAULT '[]';
UPDATE api_tokens SET scopes = '["*"]' WHERE scopes = '' OR scopes = '[]';
`

const addCommandLogsAuditFieldsSQL = `
ALTER TABLE command_logs ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'custom';
ALTER TABLE command_logs ADD COLUMN IF NOT EXISTS template_id TEXT NOT NULL DEFAULT '';
ALTER TABLE command_logs ADD COLUMN IF NOT EXISTS risk_level TEXT NOT NULL DEFAULT 'normal';
ALTER TABLE command_logs ADD COLUMN IF NOT EXISTS risk_confirmed INTEGER NOT NULL DEFAULT 0;
ALTER TABLE command_logs ADD COLUMN IF NOT EXISTS executor_auth_method TEXT NOT NULL DEFAULT '';
ALTER TABLE command_logs ADD COLUMN IF NOT EXISTS request_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_command_logs_risk_executed_at ON command_logs (risk_level, executed_at DESC);
`

const addServerStatusCollectStateSQL = `
ALTER TABLE server_status_latest ADD COLUMN IF NOT EXISTS collect_status TEXT NOT NULL DEFAULT 'unknown';
ALTER TABLE server_status_latest ADD COLUMN IF NOT EXISTS last_collect_started_at TEXT NOT NULL DEFAULT '';
ALTER TABLE server_status_latest ADD COLUMN IF NOT EXISTS last_collect_finished_at TEXT NOT NULL DEFAULT '';
ALTER TABLE server_status_latest ADD COLUMN IF NOT EXISTS last_success_at TEXT NOT NULL DEFAULT '';
ALTER TABLE server_status_latest ADD COLUMN IF NOT EXISTS last_collect_error TEXT NOT NULL DEFAULT '';
ALTER TABLE server_status_latest ADD COLUMN IF NOT EXISTS collect_failure_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE server_status_latest ADD COLUMN IF NOT EXISTS collect_duration_ms BIGINT NOT NULL DEFAULT 0;
ALTER TABLE server_status_latest ADD COLUMN IF NOT EXISTS stale INTEGER NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_server_status_latest_collect_status ON server_status_latest (collect_status);
`

const addAlertRuleScopeSQL = `
ALTER TABLE alert_rules ADD COLUMN IF NOT EXISTS scope_type TEXT NOT NULL DEFAULT 'all';
ALTER TABLE alert_rules ADD COLUMN IF NOT EXISTS scope_value TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_alert_rules_scope ON alert_rules (scope_type, scope_value);
`

const createAlertNotificationDeliveriesTableSQL = `
CREATE TABLE IF NOT EXISTS alert_notification_deliveries (
    id BIGSERIAL PRIMARY KEY,
    event_type TEXT NOT NULL,
    alert_id BIGINT NOT NULL DEFAULT 0,
    rule_id BIGINT NOT NULL DEFAULT 0,
    server_id BIGINT NOT NULL DEFAULT 0,
    server_name TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TEXT,
    last_attempt_at TEXT,
    last_error TEXT NOT NULL DEFAULT '',
    payload TEXT NOT NULL,
    occurred_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_alert_notification_deliveries_status_next ON alert_notification_deliveries (status, next_attempt_at, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_alert_notification_deliveries_created_at ON alert_notification_deliveries (created_at DESC);
`

type migrationStep struct {
	version    int64
	statements []string
}

var schemaMigrations = []migrationStep{
	{
		version: 1,
		statements: []string{
			createServersTableSQL,
			createServerCredentialsTableSQL,
			createServerStatusLatestTableSQL,
			createServerStatusHistoryTableSQL,
			createCommandLogsTableSQL,
			createUsersTableSQL,
			createUserSessionsTableSQL,
			createAuthEventsTableSQL,
			createAlertRulesTableSQL,
			createUserSessionsExpiresIndexSQL,
			createAuthEventsCreatedIndexSQL,
			createCommandLogsExecutedIndexSQL,
		},
	},
	{
		version: 2,
		statements: []string{
			addServersTrustedHostKeyFingerprintSQL,
		},
	},
	{
		version: 3,
		statements: []string{
			createAlertStatesTableSQL,
			createAlertHistoryTableSQL,
		},
	},
	{
		version: 4,
		statements: []string{
			createAuditEventsTableSQL,
			createAuditEventsCreatedIndexSQL,
		},
	},
	{
		version: 5,
		statements: []string{
			addCommandLogsExecutorUsernameSQL,
			createCommandLogsServerExecutedIndexSQL,
			createCommandLogsExecutorExecutedIndexSQL,
		},
	},
	{
		version: 6,
		statements: []string{
			addUsersEnabledSQL,
		},
	},
	{
		version: 7,
		statements: []string{
			addUsersNotificationReadAtSQL,
		},
	},
	{
		version: 8,
		statements: []string{
			createAPITokensTableSQL,
			createAPITokensUserActiveIndexSQL,
			createAPITokensHashIndexSQL,
		},
	},
	{
		version: 9,
		statements: []string{
			addServersMaintenanceWindowSQL,
		},
	},
	{
		version: 10,
		statements: []string{
			alterCommandLogsExecutedAtToTimestamptzSQL,
		},
	},
	{
		version: 11,
		statements: []string{
			createAlertNotificationSettingsTableSQL,
		},
	},
	{
		version: 12,
		statements: []string{
			normalizeAlertNotificationSettingsSingletonSQL,
		},
	},
	{
		version: 13,
		statements: []string{
			createCommandTemplatesTableSQL,
			createCommandTemplateFavoritesTableSQL,
			createCommandTemplatesScopeIndexSQL,
			createCommandTemplateFavoritesUsernameIndexSQL,
		},
	},
	{
		version: 14,
		statements: []string{
			createServerStatusHistorySampledAtIndexSQL,
			createServerStatusHistoryServerSampledAtIndexSQL,
		},
	},
	{
		version: 15,
		statements: []string{
			addCommandLogsServerSnapshotSQL,
		},
	},
	{
		version: 16,
		statements: []string{
			addAPITokensScopesSQL,
		},
	},
	{
		version: 17,
		statements: []string{
			addCommandLogsAuditFieldsSQL,
		},
	},
	{
		version: 18,
		statements: []string{
			addServerStatusCollectStateSQL,
		},
	},
	{
		version: 19,
		statements: []string{
			addAlertRuleScopeSQL,
			createAlertNotificationDeliveriesTableSQL,
		},
	},
	{
		version: 20,
		statements: []string{
			`ALTER TABLE server_status_latest ADD COLUMN IF NOT EXISTS collect_duration_ms BIGINT NOT NULL DEFAULT 0;`,
		},
	},
	{
		version: 21,
		statements: []string{
			addServerPrivateKeyAndExpiresAtSQL,
		},
	},
}

func Migrate(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return errors.New("db is required")
	}

	if _, err := db.ExecContext(ctx, createSchemaMigrationsTableSQL); err != nil {
		return err
	}

	currentVersion, err := currentSchemaVersion(ctx, db)
	if err != nil {
		return err
	}

	for _, migration := range schemaMigrations {
		if migration.version <= currentVersion {
			continue
		}
		if err := applyMigration(ctx, db, migration); err != nil {
			return err
		}
		currentVersion = migration.version
	}

	return nil
}

func currentSchemaVersion(ctx context.Context, db *sql.DB) (int64, error) {
	var version sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil {
		return 0, err
	}
	if !version.Valid {
		return 0, nil
	}
	return version.Int64, nil
}

func applyMigration(ctx context.Context, db *sql.DB, migration migrationStep) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var alreadyApplied bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, migration.version).Scan(&alreadyApplied); err != nil {
		return err
	}
	if alreadyApplied {
		return nil
	}

	for _, statement := range migration.statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}

	result, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES ($1, CURRENT_TIMESTAMP::text) ON CONFLICT (version) DO NOTHING`, migration.version)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return nil
	}

	return tx.Commit()
}
