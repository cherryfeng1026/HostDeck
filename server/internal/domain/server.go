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
	ID                 int64     `json:"id"`
	Name               string    `json:"name"`
	Hostname           string    `json:"hostname"`
	IP                 string    `json:"ip"`
	SSHPort            int       `json:"sshPort"`
	Username           string    `json:"username"`
	AuthType           string    `json:"authType"`
	PasswordConfigured bool      `json:"passwordConfigured"`
	Password           string    `json:"-"`
	CollectorMode      string    `json:"collectorMode"`
	Tags               []string  `json:"tags"`
	Purpose            string    `json:"purpose"`
	Remark             string    `json:"remark"`
	Enabled            bool      `json:"enabled"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}
