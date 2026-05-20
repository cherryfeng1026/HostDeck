package storage

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"hostdeck/server/internal/domain"
)

var (
	ErrCommandTemplateNotFound      = errors.New("命令模板不存在")
	ErrCommandTemplateAccessDenied = errors.New("当前账号无法访问该命令模板")
)

type CommandTemplateConflictError struct {
	ID string
}

func (e CommandTemplateConflictError) Error() string {
	if strings.TrimSpace(e.ID) == "" {
		return "命令模板已存在"
	}
	return "命令模板已存在: " + e.ID
}

type CommandTemplateValidationError struct {
	Message string
}

func (e CommandTemplateValidationError) Error() string {
	return e.Message
}

func newCommandTemplateValidationError(message string) error {
	return CommandTemplateValidationError{Message: message}
}

func wrapCommandTemplateMutationError(err error, id string) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return CommandTemplateConflictError{ID: id}
	}
	if strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return CommandTemplateConflictError{ID: id}
	}
	return err
}

type CommandTemplateRepository struct {
	db *sql.DB
}

func NewCommandTemplateRepository(db *sql.DB) *CommandTemplateRepository {
	return &CommandTemplateRepository{db: db}
}

func (r *CommandTemplateRepository) EnsureDefaults(ctx context.Context, items []domain.CommandTemplate) error {
	for _, item := range items {
		variablesJSON, err := json.Marshal(item.Variables)
		if err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := r.db.ExecContext(
			ctx,
			`INSERT INTO command_templates (id, name, description, command_text, scope, risk_level, created_by, variables_json, created_at, updated_at)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			 ON CONFLICT (id) DO UPDATE SET
			 	name = EXCLUDED.name,
			 	description = EXCLUDED.description,
			 	command_text = EXCLUDED.command_text,
			 	scope = EXCLUDED.scope,
			 	risk_level = EXCLUDED.risk_level,
			 	variables_json = EXCLUDED.variables_json,
			 	updated_at = EXCLUDED.updated_at`,
			item.ID,
			item.Name,
			item.Description,
			item.Command,
			item.Scope,
			item.RiskLevel,
			strings.TrimSpace(item.CreatedBy),
			string(variablesJSON),
			now,
			now,
		); err != nil {
			return err
		}
	}
	return nil
}

func (r *CommandTemplateRepository) List(ctx context.Context, filter domain.CommandTemplateFilter) ([]domain.CommandTemplate, error) {
	username := strings.TrimSpace(filter.Username)
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT
			ct.id,
			ct.name,
			ct.description,
			ct.command_text,
			ct.scope,
			ct.risk_level,
			ct.created_by,
			ct.variables_json,
			CASE WHEN ctf.template_id IS NULL THEN 0 ELSE 1 END AS is_favorite
		FROM command_templates ct
		LEFT JOIN command_template_favorites ctf
		  ON ctf.template_id = ct.id AND ctf.username = $1
		WHERE ct.scope = $2 OR (ct.scope = $3 AND ct.created_by = $1)
		ORDER BY is_favorite DESC, CASE WHEN ct.scope = $2 THEN 0 ELSE 1 END ASC, ct.name ASC`,
		username,
		domain.CommandTemplateScopeShared,
		domain.CommandTemplateScopePersonal,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.CommandTemplate, 0)
	for rows.Next() {
		item, err := scanCommandTemplate(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *CommandTemplateRepository) GetByID(ctx context.Context, templateID string, username string) (domain.CommandTemplate, error) {
	templateID = strings.TrimSpace(templateID)
	username = strings.TrimSpace(username)
	if templateID == "" {
		return domain.CommandTemplate{}, ErrCommandTemplateNotFound
	}
	row := r.db.QueryRowContext(
		ctx,
		`SELECT
			ct.id,
			ct.name,
			ct.description,
			ct.command_text,
			ct.scope,
			ct.risk_level,
			ct.created_by,
			ct.variables_json,
			0 AS is_favorite
		FROM command_templates ct
		WHERE ct.id = $1 AND (ct.scope = $2 OR (ct.scope = $3 AND ct.created_by = $4))`,
		templateID,
		domain.CommandTemplateScopeShared,
		domain.CommandTemplateScopePersonal,
		username,
	)
	item, err := scanCommandTemplate(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.CommandTemplate{}, ErrCommandTemplateNotFound
	}
	return item, err
}

func (r *CommandTemplateRepository) Create(ctx context.Context, input domain.CommandTemplateCreateInput, username string) (domain.CommandTemplate, error) {
	username = strings.TrimSpace(username)
	normalized, err := normalizeCommandTemplateInput(input, username)
	if err != nil {
		return domain.CommandTemplate{}, err
	}
	variablesJSON, err := json.Marshal(normalized.Variables)
	if err != nil {
		return domain.CommandTemplate{}, err
	}
	item := domain.CommandTemplate{
		ID:          buildCommandTemplateID(normalized.Scope, normalized.Name, username),
		Name:        normalized.Name,
		Description: normalized.Description,
		Command:     normalized.Command,
		Scope:       normalized.Scope,
		RiskLevel:   normalized.RiskLevel,
		CreatedBy:   normalized.CreatedBy,
		IsFavorite:  normalized.Scope == domain.CommandTemplateScopePersonal,
		Variables:   normalized.Variables,
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.CommandTemplate{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO command_templates (id, name, description, command_text, scope, risk_level, created_by, variables_json, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		item.ID,
		item.Name,
		item.Description,
		item.Command,
		item.Scope,
		item.RiskLevel,
		item.CreatedBy,
		string(variablesJSON),
		now,
		now,
	); err != nil {
		return domain.CommandTemplate{}, wrapCommandTemplateMutationError(err, item.ID)
	}
	if item.IsFavorite {
		if err := setFavoriteTx(ctx, tx, item.ID, username, true, now); err != nil {
			return domain.CommandTemplate{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.CommandTemplate{}, err
	}
	return item, nil
}

func (r *CommandTemplateRepository) SetFavorite(ctx context.Context, templateID string, username string, favorite bool) error {
	templateID = strings.TrimSpace(templateID)
	username = strings.TrimSpace(username)
	if templateID == "" || username == "" {
		return errors.New("template id and username are required")
	}
	if err := r.ensureTemplateAccessible(ctx, templateID, username); err != nil {
		return err
	}
	if favorite {
		return setFavoriteTx(ctx, r.db, templateID, username, true, time.Now().UTC().Format(time.RFC3339Nano))
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM command_template_favorites WHERE template_id = $1 AND username = $2`, templateID, username)
	return err
}

func (r *CommandTemplateRepository) ensureTemplateAccessible(ctx context.Context, templateID string, username string) error {
	var scope string
	var createdBy string
	err := r.db.QueryRowContext(ctx, `SELECT scope, created_by FROM command_templates WHERE id = $1`, templateID).Scan(&scope, &createdBy)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrCommandTemplateNotFound
	}
	if err != nil {
		return err
	}
	if strings.TrimSpace(scope) == domain.CommandTemplateScopePersonal && strings.TrimSpace(createdBy) != username {
		return ErrCommandTemplateAccessDenied
	}
	return nil
}

func setFavoriteTx(ctx context.Context, exec interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, templateID string, username string, favorite bool, createdAt string) error {
	if favorite {
		_, err := exec.ExecContext(
			ctx,
			`INSERT INTO command_template_favorites (template_id, username, created_at)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (template_id, username) DO NOTHING`,
			templateID,
			username,
			createdAt,
		)
		return err
	}
	_, err := exec.ExecContext(ctx, `DELETE FROM command_template_favorites WHERE template_id = $1 AND username = $2`, templateID, username)
	return err
}

type commandTemplateScanner interface {
	Scan(dest ...any) error
}

func scanCommandTemplate(scanner commandTemplateScanner) (domain.CommandTemplate, error) {
	var (
		item          domain.CommandTemplate
		variablesJSON string
		favoriteValue int
	)
	if err := scanner.Scan(
		&item.ID,
		&item.Name,
		&item.Description,
		&item.Command,
		&item.Scope,
		&item.RiskLevel,
		&item.CreatedBy,
		&variablesJSON,
		&favoriteValue,
	); err != nil {
		return domain.CommandTemplate{}, err
	}
	item.IsFavorite = favoriteValue == 1
	if strings.TrimSpace(variablesJSON) == "" {
		variablesJSON = "[]"
	}
	if err := json.Unmarshal([]byte(variablesJSON), &item.Variables); err != nil {
		return domain.CommandTemplate{}, err
	}
	return item, nil
}

type normalizedCommandTemplateInput struct {
	Name        string
	Description string
	Command     string
	Scope       string
	RiskLevel   string
	CreatedBy   string
	Variables   []domain.CommandTemplateVariable
}

func normalizeCommandTemplateInput(input domain.CommandTemplateCreateInput, username string) (normalizedCommandTemplateInput, error) {
	item := normalizedCommandTemplateInput{
		Name:        strings.TrimSpace(input.Name),
		Description: strings.TrimSpace(input.Description),
		Command:     strings.TrimSpace(input.Command),
		Scope:       strings.TrimSpace(input.Scope),
		RiskLevel:   strings.TrimSpace(input.RiskLevel),
		CreatedBy:   strings.TrimSpace(username),
		Variables:   append([]domain.CommandTemplateVariable(nil), input.Variables...),
	}
	if item.Name == "" {
		return normalizedCommandTemplateInput{}, newCommandTemplateValidationError("模板名称不能为空")
	}
	if item.Command == "" {
		return normalizedCommandTemplateInput{}, newCommandTemplateValidationError("模板命令不能为空")
	}
	if item.Scope == "" {
		item.Scope = domain.CommandTemplateScopePersonal
	}
	if item.Scope != domain.CommandTemplateScopeShared && item.Scope != domain.CommandTemplateScopePersonal {
		return normalizedCommandTemplateInput{}, newCommandTemplateValidationError("模板范围无效")
	}
	if item.Scope == domain.CommandTemplateScopePersonal && item.CreatedBy == "" {
		return normalizedCommandTemplateInput{}, newCommandTemplateValidationError("个人模板必须包含创建者")
	}
	if item.RiskLevel == "" {
		item.RiskLevel = domain.CommandTemplateRiskNormal
	}
	if item.RiskLevel != domain.CommandTemplateRiskNormal && item.RiskLevel != domain.CommandTemplateRiskDangerous {
		return normalizedCommandTemplateInput{}, newCommandTemplateValidationError("模板风险级别无效")
	}
	for index := range item.Variables {
		item.Variables[index].Name = strings.TrimSpace(item.Variables[index].Name)
		item.Variables[index].Label = strings.TrimSpace(item.Variables[index].Label)
		item.Variables[index].Placeholder = strings.TrimSpace(item.Variables[index].Placeholder)
		item.Variables[index].DefaultValue = strings.TrimSpace(item.Variables[index].DefaultValue)
		if item.Variables[index].Name == "" {
			return normalizedCommandTemplateInput{}, newCommandTemplateValidationError("模板变量名不能为空")
		}
		if item.Variables[index].Label == "" {
			item.Variables[index].Label = item.Variables[index].Name
		}
	}
	return item, nil
}

func buildCommandTemplateID(scope string, name string, username string) string {
	nameSlug := slugifyCommandTemplateName(name)
	userSlug := slugifyCommandTemplateName(username)
	if userSlug == "template" {
		userSlug = "user-" + shortTemplateHash(username)
	}
	if nameSlug == "template" {
		nameSlug = "template-" + shortTemplateHash(strings.TrimSpace(name))
	}
	if scope == domain.CommandTemplateScopeShared {
		return "shared-" + nameSlug
	}
	return "personal-" + userSlug + "-" + nameSlug
}

func shortTemplateHash(value string) string {
	sum := sha1.Sum([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])[:8]
}

func slugifyCommandTemplateName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "template"
	}
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == ' ':
			if !lastDash && builder.Len() > 0 {
				builder.WriteByte('-')
				lastDash = true
			}
		default:
			if !lastDash && builder.Len() > 0 {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}
	result := strings.Trim(builder.String(), "-")
	if result == "" {
		return "template"
	}
	return result
}
