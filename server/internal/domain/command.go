package domain

import "time"

type CommandLog struct {
	ID         int64     `json:"id"`
	ServerID   int64     `json:"serverId"`
	Command    string    `json:"command"`
	Stdout     string    `json:"stdout"`
	Stderr     string    `json:"stderr"`
	ExitCode   int       `json:"exitCode"`
	DurationMS int64     `json:"durationMs"`
	ExecutedAt time.Time `json:"executedAt"`
}
