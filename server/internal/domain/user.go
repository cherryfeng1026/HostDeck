package domain

import "time"

const (
	RoleAdmin    = "admin"
	RoleOperator = "operator"
	RoleViewer   = "viewer"
)

const (
	AuthEventBootstrapAdminCreated = "bootstrap_admin_created"
	AuthEventLoginFailed           = "login_failed"
	AuthEventLoginSucceeded        = "login_succeeded"
	AuthEventLogout                = "logout"
	AuthEventPasswordChanged       = "password_changed"
)

type User struct {
	ID          int64      `json:"id"`
	Username    string     `json:"username"`
	Role        string     `json:"role"`
	LastLoginAt *time.Time `json:"lastLoginAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

type AuthEvent struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"userId,omitempty"`
	Username  string    `json:"username"`
	EventType string    `json:"eventType"`
	Detail    string    `json:"detail"`
	IP        string    `json:"ip,omitempty"`
	UserAgent string    `json:"userAgent,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

func CanManageInfrastructure(role string) bool {
	switch role {
	case RoleAdmin, RoleOperator:
		return true
	default:
		return false
	}
}

func CanManageUsers(role string) bool {
	switch role {
	case RoleAdmin:
		return true
	default:
		return false
	}
}
