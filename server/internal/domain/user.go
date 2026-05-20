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
	AuthEventUserCreated           = "user_created"
	AuthEventUserUpdated           = "user_updated"
	AuthEventUserPasswordReset     = "user_password_reset"
	AuthEventUserSessionsRevoked   = "user_sessions_revoked"
	AuthEventAPITokenCreated       = "api_token_created"
	AuthEventAPITokenRevoked       = "api_token_revoked"
)

const (
	ScopeAll                   = "*"
	ScopeServersRead           = "servers:read"
	ScopeServersWrite          = "servers:write"
	ScopeCommandsRead          = "commands:read"
	ScopeCommandsExecute       = "commands:execute"
	ScopeCommandTemplatesWrite = "commands:templates:write"
	ScopeAlertsRead            = "alerts:read"
	ScopeAlertsWrite           = "alerts:write"
)

type User struct {
	ID          int64      `json:"id"`
	Username    string     `json:"username"`
	Role        string     `json:"role"`
	Enabled     bool       `json:"enabled"`
	LastLoginAt *time.Time `json:"lastLoginAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

func NormalizeUserRole(role string) string {
	switch role {
	case RoleAdmin, RoleOperator, RoleViewer:
		return role
	default:
		return ""
	}
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

type APIToken struct {
	ID         int64      `json:"id"`
	UserID     int64      `json:"userId"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
	IsActive   bool       `json:"isActive"`
	Scopes     []string   `json:"scopes"`
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
