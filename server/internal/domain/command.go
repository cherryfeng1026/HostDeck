package domain

import "time"

type CommandLog struct {
	ID               int64     `json:"id"`
	ServerID         int64     `json:"serverId"`
	ServerName       string    `json:"serverName,omitempty"`
	ExecutorUsername string    `json:"executorUsername,omitempty"`
	Command          string    `json:"command"`
	Stdout           string    `json:"stdout"`
	Stderr           string    `json:"stderr"`
	ExitCode         int       `json:"exitCode"`
	DurationMS       int64     `json:"durationMs"`
	ExecutedAt       time.Time `json:"executedAt"`
}

const (
	CommandTemplateScopeShared   = "shared"
	CommandTemplateScopePersonal = "personal"
)

const (
	CommandTemplateRiskNormal    = "normal"
	CommandTemplateRiskDangerous = "dangerous"
)

type CommandTemplateVariable struct {
	Name         string `json:"name"`
	Label        string `json:"label"`
	Placeholder  string `json:"placeholder,omitempty"`
	DefaultValue string `json:"defaultValue,omitempty"`
	Required     bool   `json:"required"`
}

type CommandTemplate struct {
	ID          string                    `json:"id"`
	Name        string                    `json:"name"`
	Description string                    `json:"description"`
	Command     string                    `json:"command"`
	Scope       string                    `json:"scope"`
	RiskLevel   string                    `json:"riskLevel"`
	CreatedBy   string                    `json:"createdBy,omitempty"`
	IsFavorite  bool                      `json:"isFavorite"`
	Variables   []CommandTemplateVariable `json:"variables,omitempty"`
}

type CommandTemplateCreateInput struct {
	Name        string
	Description string
	Command     string
	Scope       string
	RiskLevel   string
	Variables   []CommandTemplateVariable
}

type CommandTemplateFilter struct {
	Username string
}

type CommandHistoryFilter struct {
	ServerID         int64
	ExecutorUsername string
	Keyword          string
	StartTime        *time.Time
	EndTime          *time.Time
	Limit            int
}
