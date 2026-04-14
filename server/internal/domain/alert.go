package domain

import "time"

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

type AlertEvent struct {
	RuleID          int64     `json:"ruleId"`
	ServerID        int64     `json:"serverId"`
	ServerName      string    `json:"serverName"`
	Metric          string    `json:"metric"`
	Operator        string    `json:"operator"`
	Threshold       float64   `json:"threshold"`
	CurrentValue    float64   `json:"currentValue"`
	Severity        string    `json:"severity"`
	Message         string    `json:"message"`
	TriggeredAt     time.Time `json:"triggeredAt"`
	DurationSeconds int       `json:"durationSeconds"`
}
