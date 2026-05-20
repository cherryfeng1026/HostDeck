package domain

import "time"

type CommandLog struct {
	ID                 int64     `json:"id"`
	ServerID           int64     `json:"serverId"`
	ServerName         string    `json:"serverName,omitempty"`
	ServerIP           string    `json:"serverIp,omitempty"`
	ExecutorUsername   string    `json:"executorUsername,omitempty"`
	ExecutorAuthMethod string    `json:"executorAuthMethod,omitempty"`
	Command            string    `json:"command"`
	Stdout             string    `json:"stdout"`
	Stderr             string    `json:"stderr"`
	ExitCode           int       `json:"exitCode"`
	DurationMS         int64     `json:"durationMs"`
	ExecutedAt         time.Time `json:"executedAt"`
	Source             string    `json:"source"`
	TemplateID         string    `json:"templateId,omitempty"`
	RiskLevel          string    `json:"riskLevel"`
	RiskConfirmed      bool      `json:"riskConfirmed"`
	RequestID          string    `json:"requestId,omitempty"`
}

const (
	CommandTemplateScopeShared   = "shared"
	CommandTemplateScopePersonal = "personal"
)

const (
	CommandTemplateRiskNormal    = "normal"
	CommandTemplateRiskDangerous = "dangerous"
)

const (
	CommandSourceCustom   = "custom"
	CommandSourceTemplate = "template"
)

type CommandExecutionInput struct {
	ServerID           int64
	ServerIDs          []int64
	Command            string
	Timeout            time.Duration
	Source             string
	TemplateID         string
	RiskLevel          string
	RiskConfirmed      bool
	ExecutorUsername   string
	ExecutorAuthMethod string
	RequestID          string
}

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
