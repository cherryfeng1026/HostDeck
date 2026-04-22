package domain

import "time"

const (
	AlertStatusPending      = "pending"
	AlertStatusActive       = "active"
	AlertStatusAcknowledged = "acknowledged"
	AlertStatusMuted        = "muted"
)

const (
	AlertEventTriggered    = "triggered"
	AlertEventAcknowledged = "acknowledged"
	AlertEventMuted        = "muted"
	AlertEventResolved     = "resolved"
)

type AlertRule struct {
	ID              int64     `json:"id"`
	Metric          string    `json:"metric"`
	Operator        string    `json:"operator"`
	Threshold       float64   `json:"threshold"`
	DurationSeconds int       `json:"durationSeconds"`
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type AlertState struct {
	ID              int64
	RuleID          int64
	ServerID        int64
	Metric          string
	Operator        string
	Threshold       float64
	CurrentValue    float64
	Severity        string
	Message         string
	Status          string
	DurationSeconds int
	FirstTriggeredAt time.Time
	LastTriggeredAt  time.Time
	AcknowledgedAt   *time.Time
	AcknowledgedBy   string
	MutedUntil       *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type AlertEvent struct {
	ID              int64      `json:"id"`
	RuleID          int64      `json:"ruleId"`
	ServerID        int64      `json:"serverId"`
	ServerName      string     `json:"serverName"`
	Metric          string     `json:"metric"`
	Operator        string     `json:"operator"`
	Threshold       float64    `json:"threshold"`
	CurrentValue    float64    `json:"currentValue"`
	Severity        string     `json:"severity"`
	Message         string     `json:"message"`
	Status          string     `json:"status"`
	TriggeredAt     time.Time  `json:"triggeredAt"`
	LastTriggeredAt time.Time  `json:"lastTriggeredAt"`
	AcknowledgedAt  *time.Time `json:"acknowledgedAt,omitempty"`
	AcknowledgedBy  string     `json:"acknowledgedBy,omitempty"`
	MutedUntil      *time.Time `json:"mutedUntil,omitempty"`
	DurationSeconds int        `json:"durationSeconds"`
}

type AlertHistoryEvent struct {
	ID            int64     `json:"id"`
	AlertID       int64     `json:"alertId"`
	RuleID        int64     `json:"ruleId"`
	ServerID      int64     `json:"serverId"`
	ServerName    string    `json:"serverName"`
	EventType     string    `json:"eventType"`
	Metric        string    `json:"metric"`
	Operator      string    `json:"operator"`
	Threshold     float64   `json:"threshold"`
	CurrentValue  float64   `json:"currentValue"`
	Severity      string    `json:"severity"`
	Message       string    `json:"message"`
	Status        string    `json:"status"`
	TriggeredAt   time.Time `json:"triggeredAt"`
	CreatedAt     time.Time `json:"createdAt"`
	ActorUsername string    `json:"actorUsername,omitempty"`
	Detail        string    `json:"detail,omitempty"`
}

type AlertNotificationSettings struct {
	Enabled               bool      `json:"enabled"`
	WebhookURL            string    `json:"webhookURL"`
	WebhookConfigured     bool      `json:"webhookConfigured,omitempty"`
	ClearWebhookURL       bool      `json:"clearWebhookURL,omitempty"`
	WebhookTimeoutSeconds int       `json:"webhookTimeoutSeconds"`
	CreatedAt             time.Time `json:"createdAt,omitempty"`
	UpdatedAt             time.Time `json:"updatedAt,omitempty"`
}
