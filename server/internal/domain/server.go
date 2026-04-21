package domain

import (
	"strings"
	"time"
)

const CollectorModeSSHOnly = "ssh_only"

func NormalizeCollectorMode(mode string) string {
	if strings.TrimSpace(mode) == CollectorModeSSHOnly {
		return CollectorModeSSHOnly
	}
	return CollectorModeSSHOnly
}

type Server struct {
	ID                        int64      `json:"id"`
	Name                      string     `json:"name"`
	Hostname                  string     `json:"hostname"`
	IP                        string     `json:"ip"`
	SSHPort                   int        `json:"sshPort"`
	Username                  string     `json:"username"`
	AuthType                  string     `json:"authType"`
	PasswordConfigured        bool       `json:"passwordConfigured"`
	TrustedHostKeyFingerprint string     `json:"trustedHostKeyFingerprint"`
	Password                  string     `json:"-"`
	CollectorMode             string     `json:"collectorMode"`
	Tags                      []string   `json:"tags"`
	Purpose                   string     `json:"purpose"`
	Remark                    string     `json:"remark"`
	MaintenanceStartAt        *time.Time `json:"maintenanceStartAt,omitempty"`
	MaintenanceEndAt          *time.Time `json:"maintenanceEndAt,omitempty"`
	Enabled                   bool       `json:"enabled"`
	CreatedAt                 time.Time  `json:"createdAt"`
	UpdatedAt                 time.Time  `json:"updatedAt"`
}

func (s Server) InMaintenanceWindow(at time.Time) bool {
	if s.MaintenanceStartAt == nil || s.MaintenanceEndAt == nil {
		return false
	}
	at = at.UTC()
	return !at.Before(s.MaintenanceStartAt.UTC()) && at.Before(s.MaintenanceEndAt.UTC())
}
