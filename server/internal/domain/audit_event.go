package domain

import "time"

const (
	AuditKindServer    = "server"
	AuditKindAlertRule = "alert_rule"
	AuditKindAlert     = "alert"
	AuditKindCommand   = "command"
)

type AuditEvent struct {
	ID         int64     `json:"id"`
	Kind       string    `json:"kind"`
	Severity   string    `json:"severity"`
	Title      string    `json:"title"`
	Summary    string    `json:"summary"`
	ServerID   int64     `json:"serverId,omitempty"`
	ServerName string    `json:"serverName,omitempty"`
	Username   string    `json:"username,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}
