package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/storage"
)

const (
	minPasswordLength        = 8
	maxLoginFailures         = 5
	loginFailureWindow       = 15 * time.Minute
	loginLockDuration        = 15 * time.Minute
	sessionTouchWindow       = 5 * time.Minute
	expiredSessionCleanupTTL = 10 * time.Minute
	uninitializedCacheTTL    = 10 * time.Second
)

var (
	ErrInvalidCredentials       = errors.New("用户名或密码错误")
	ErrTooManyLoginAttempts     = errors.New("登录失败次数过多，请稍后再试")
	ErrUnauthenticated          = errors.New("请先登录")
	ErrBootstrapAlreadyDone     = errors.New("初始管理员已创建")
	ErrBootstrapPasswordPolicy  = fmt.Errorf("管理员密码至少需要 %d 位", minPasswordLength)
	ErrPasswordPolicy           = fmt.Errorf("新密码至少需要 %d 位", minPasswordLength)
	ErrSystemUninitialized      = errors.New("系统尚未初始化管理员")
	ErrUserDisabled             = errors.New("当前用户已被禁用")
	ErrInvalidRole              = errors.New("用户角色无效")
	ErrUserNotFound             = errors.New("用户不存在")
	ErrCannotDisableSelf        = errors.New("不能禁用当前登录账号")
	ErrCannotChangeOwnRole      = errors.New("不能修改当前账号角色")
	ErrLastEnabledAdminRequired = errors.New("系统至少需要保留一个启用中的管理员")
	ErrAPITokenNotFound         = errors.New("API Token 不存在")
	ErrInvalidAPITokenScope     = errors.New("API Token 权限范围无效")
)

type AuthUserStore interface {
	Count(ctx context.Context) (int, error)
	Create(ctx context.Context, username string, passwordHash string, role string) (domain.User, error)
	GetByUsername(ctx context.Context, username string) (storage.UserRecord, error)
	GetByID(ctx context.Context, id int64) (storage.UserRecord, error)
	List(ctx context.Context) ([]domain.User, error)
	UpdateLastLoginAt(ctx context.Context, id int64, at time.Time) error
	UpdatePassword(ctx context.Context, id int64, passwordHash string, updatedAt time.Time) error
	Update(ctx context.Context, id int64, input storage.UserUpdateInput, updatedAt time.Time) error
}

type AuthSessionStore interface {
	Create(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time, ip string, userAgent string) error
	GetByTokenHash(ctx context.Context, tokenHash string) (storage.SessionRecord, error)
	Touch(ctx context.Context, sessionID int64, lastSeenAt time.Time) error
	DeleteByTokenHash(ctx context.Context, tokenHash string) error
	DeleteByUserID(ctx context.Context, userID int64) error
	DeleteExpired(ctx context.Context, now time.Time) error
}

type AuthEventStore interface {
	Create(ctx context.Context, event domain.AuthEvent) error
}

type AuthAPITokenStore interface {
	Create(ctx context.Context, userID int64, name string, tokenHash string, prefix string, scopes []string, expiresAt *time.Time, now time.Time) (domain.APIToken, error)
	ListActiveByUserID(ctx context.Context, userID int64) ([]domain.APIToken, error)
	GetByID(ctx context.Context, id int64) (storage.APITokenRecord, error)
	GetByTokenHash(ctx context.Context, tokenHash string) (storage.APITokenRecord, error)
	Touch(ctx context.Context, id int64, usedAt time.Time) error
	Revoke(ctx context.Context, id int64, revokedAt time.Time) error
	DeleteExpiredOrRevokedBefore(ctx context.Context, cutoff time.Time) error
}

type loginAttempt struct {
	count         int
	firstFailedAt time.Time
	lockedUntil   time.Time
}

type AuthService struct {
	users      AuthUserStore
	sessions   AuthSessionStore
	apiTokens  AuthAPITokenStore
	events     AuthEventStore
	sessionTTL time.Duration

	mu                       sync.Mutex
	attempts                 map[string]loginAttempt
	lastExpiredSessionPrune  time.Time
	uninitializedCachedUntil time.Time
}

func NewAuthService(users AuthUserStore, sessions AuthSessionStore, apiTokens AuthAPITokenStore, events AuthEventStore, sessionTTL time.Duration) *AuthService {
	if sessionTTL <= 0 {
		sessionTTL = 24 * time.Hour
	}

	return &AuthService{
		users:      users,
		sessions:   sessions,
		apiTokens:  apiTokens,
		events:     events,
		sessionTTL: sessionTTL,
		attempts:   map[string]loginAttempt{},
	}
}

func (s *AuthService) EnsureBootstrapAdmin(ctx context.Context, username string, password string) error {
	username = normalizeUsername(username)
	password = strings.TrimSpace(password)
	if username == "" && password == "" {
		return nil
	}
	if username == "" || password == "" {
		return errors.New("启动引导管理员需要同时提供用户名和密码")
	}

	_, err := s.CreateInitialAdmin(ctx, username, password, "", "", "startup_config")
	if errors.Is(err, ErrBootstrapAlreadyDone) {
		return nil
	}
	return err
}

func (s *AuthService) CreateInitialAdmin(ctx context.Context, username string, password string, remoteIP string, userAgent string, detail string) (domain.User, error) {
	username = normalizeUsername(username)
	password = strings.TrimSpace(password)
	if username == "" {
		return domain.User{}, errors.New("管理员用户名不能为空")
	}
	if password == "" {
		return domain.User{}, errors.New("管理员密码不能为空")
	}
	if len(password) < minPasswordLength {
		return domain.User{}, ErrBootstrapPasswordPolicy
	}

	count, err := s.users.Count(ctx)
	if err != nil {
		return domain.User{}, err
	}
	if count > 0 {
		return domain.User{}, ErrBootstrapAlreadyDone
	}

	passwordHash, err := hashPassword(password)
	if err != nil {
		return domain.User{}, err
	}

	user, err := s.users.Create(ctx, username, passwordHash, domain.RoleAdmin)
	if err != nil {
		return domain.User{}, err
	}

	s.mu.Lock()
	s.uninitializedCachedUntil = time.Time{}
	s.mu.Unlock()

	_ = s.recordEvent(ctx, domain.AuthEvent{
		UserID:    user.ID,
		Username:  user.Username,
		EventType: domain.AuthEventBootstrapAdminCreated,
		Detail:    detail,
		IP:        remoteIP,
		UserAgent: userAgent,
	})

	return user, nil
}

func (s *AuthService) Login(ctx context.Context, username string, password string, remoteIP string, userAgent string) (domain.User, string, time.Time, error) {
	username = normalizeUsername(username)
	password = strings.TrimSpace(password)
	if username == "" || password == "" {
		return domain.User{}, "", time.Time{}, ErrInvalidCredentials
	}

	initialized, err := s.IsInitialized(ctx)
	if err != nil {
		return domain.User{}, "", time.Time{}, err
	}
	if !initialized {
		return domain.User{}, "", time.Time{}, ErrSystemUninitialized
	}

	attemptKey := loginAttemptKey(username, remoteIP)
	now := time.Now().UTC()
	if s.isLocked(attemptKey, now) {
		slog.Warn("login blocked", "username", username, "remoteIP", remoteIP, "reason", "too_many_attempts")
		return domain.User{}, "", time.Time{}, ErrTooManyLoginAttempts
	}
	s.pruneStaleAttempts(now)
	if err := s.pruneExpiredSessionsIfNeeded(ctx, now); err != nil {
		slog.Warn("prune expired sessions failed", "error", err)
	}

	record, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.registerFailure(attemptKey, time.Now().UTC())
			slog.Warn("login failed", "username", username, "remoteIP", remoteIP, "reason", "invalid_credentials")
			_ = s.recordEvent(ctx, domain.AuthEvent{
				Username:  username,
				EventType: domain.AuthEventLoginFailed,
				Detail:    "invalid_credentials",
				IP:        remoteIP,
				UserAgent: userAgent,
			})
			return domain.User{}, "", time.Time{}, ErrInvalidCredentials
		}
		return domain.User{}, "", time.Time{}, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(record.PasswordHash), []byte(password)); err != nil {
		s.registerFailure(attemptKey, time.Now().UTC())
		slog.Warn("login failed", "username", record.Username, "userID", record.ID, "remoteIP", remoteIP, "reason", "invalid_credentials")
		_ = s.recordEvent(ctx, domain.AuthEvent{
			UserID:    record.ID,
			Username:  record.Username,
			EventType: domain.AuthEventLoginFailed,
			Detail:    "invalid_credentials",
			IP:        remoteIP,
			UserAgent: userAgent,
		})
		return domain.User{}, "", time.Time{}, ErrInvalidCredentials
	}
	if !record.Enabled {
		slog.Warn("login failed", "username", record.Username, "userID", record.ID, "remoteIP", remoteIP, "reason", "user_disabled")
		_ = s.recordEvent(ctx, domain.AuthEvent{
			UserID:    record.ID,
			Username:  record.Username,
			EventType: domain.AuthEventLoginFailed,
			Detail:    "user_disabled",
			IP:        remoteIP,
			UserAgent: userAgent,
		})
		return domain.User{}, "", time.Time{}, ErrUserDisabled
	}

	s.clearFailures(attemptKey)
	token, tokenHash, err := newSessionToken()
	if err != nil {
		return domain.User{}, "", time.Time{}, err
	}

	expiresAt := now.Add(s.sessionTTL)
	if err := s.sessions.Create(ctx, record.ID, tokenHash, expiresAt, remoteIP, userAgent); err != nil {
		return domain.User{}, "", time.Time{}, err
	}
	if err := s.users.UpdateLastLoginAt(ctx, record.ID, now); err != nil {
		return domain.User{}, "", time.Time{}, err
	}

	user := record.User
	user.LastLoginAt = &now
	_ = s.recordEvent(ctx, domain.AuthEvent{
		UserID:    user.ID,
		Username:  user.Username,
		EventType: domain.AuthEventLoginSucceeded,
		Detail:    "session_created",
		IP:        remoteIP,
		UserAgent: userAgent,
	})
	slog.Info("login succeeded", "username", user.Username, "userID", user.ID, "remoteIP", remoteIP, "expiresAt", expiresAt)

	return user, token, expiresAt, nil
}

func (s *AuthService) Authenticate(ctx context.Context, token string) (domain.User, error) {
	return s.AuthenticateSession(ctx, token)
}

func (s *AuthService) AuthenticateSession(ctx context.Context, token string) (domain.User, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return domain.User{}, ErrUnauthenticated
	}

	tokenHash := hashSessionToken(token)
	session, err := s.sessions.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.User{}, ErrUnauthenticated
		}
		return domain.User{}, err
	}

	now := time.Now().UTC()
	if !session.ExpiresAt.After(now) {
		_ = s.sessions.DeleteByTokenHash(ctx, tokenHash)
		return domain.User{}, ErrUnauthenticated
	}

	record, err := s.users.GetByID(ctx, session.UserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.User{}, ErrUnauthenticated
		}
		return domain.User{}, err
	}
	if !record.Enabled {
		_ = s.sessions.DeleteByTokenHash(ctx, tokenHash)
		return domain.User{}, ErrUserDisabled
	}

	if now.Sub(session.LastSeenAt) >= sessionTouchWindow {
		_ = s.sessions.Touch(ctx, session.ID, now)
	}

	return record.User, nil
}

func (s *AuthService) AuthenticateAPIToken(ctx context.Context, token string) (domain.User, []string, error) {
	token = strings.TrimSpace(token)
	if token == "" || s.apiTokens == nil {
		return domain.User{}, nil, ErrUnauthenticated
	}

	tokenHash := hashSessionToken(token)
	record, err := s.apiTokens.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.User{}, nil, ErrUnauthenticated
		}
		return domain.User{}, nil, err
	}
	if record.RevokedAt != nil && !record.RevokedAt.IsZero() {
		return domain.User{}, nil, ErrUnauthenticated
	}
	if record.ExpiresAt != nil && !record.ExpiresAt.After(time.Now().UTC()) {
		return domain.User{}, nil, ErrUnauthenticated
	}

	userRecord, err := s.users.GetByID(ctx, record.UserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.User{}, nil, ErrUnauthenticated
		}
		return domain.User{}, nil, err
	}
	if !userRecord.Enabled {
		return domain.User{}, nil, ErrUserDisabled
	}

	now := time.Now().UTC()
	if record.LastUsedAt == nil || now.Sub(*record.LastUsedAt) >= sessionTouchWindow {
		_ = s.apiTokens.Touch(ctx, record.ID, now)
	}
	return userRecord.User, record.Scopes, nil
}

func (s *AuthService) Logout(ctx context.Context, token string, remoteIP string, userAgent string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}

	tokenHash := hashSessionToken(token)
	session, err := s.sessions.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if err := s.sessions.DeleteByTokenHash(ctx, tokenHash); err != nil {
		return err
	}

	record, err := s.users.GetByID(ctx, session.UserID)
	if err == nil {
		_ = s.recordEvent(ctx, domain.AuthEvent{
			UserID:    record.ID,
			Username:  record.Username,
			EventType: domain.AuthEventLogout,
			Detail:    "session_deleted",
			IP:        remoteIP,
			UserAgent: userAgent,
		})
	}

	return nil
}

func (s *AuthService) ChangePassword(ctx context.Context, userID int64, currentPassword string, newPassword string, remoteIP string, userAgent string) error {
	currentPassword = strings.TrimSpace(currentPassword)
	newPassword = strings.TrimSpace(newPassword)
	if userID <= 0 {
		return ErrUnauthenticated
	}
	if currentPassword == "" {
		return errors.New("当前密码不能为空")
	}
	if len(newPassword) < minPasswordLength {
		return ErrPasswordPolicy
	}

	record, err := s.users.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUnauthenticated
		}
		return err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(record.PasswordHash), []byte(currentPassword)); err != nil {
		return ErrInvalidCredentials
	}

	passwordHash, err := hashPassword(newPassword)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := s.users.UpdatePassword(ctx, userID, passwordHash, now); err != nil {
		return err
	}
	if err := s.sessions.DeleteByUserID(ctx, userID); err != nil {
		return err
	}
	_ = s.recordEvent(ctx, domain.AuthEvent{
		UserID:    record.ID,
		Username:  record.Username,
		EventType: domain.AuthEventPasswordChanged,
		Detail:    "all_sessions_revoked",
		IP:        remoteIP,
		UserAgent: userAgent,
	})
	slog.Info("password changed", "username", record.Username, "userID", record.ID, "remoteIP", remoteIP)
	return nil
}

func (s *AuthService) ListUsers(ctx context.Context) ([]domain.User, error) {
	return s.users.List(ctx)
}

func (s *AuthService) CreateUser(ctx context.Context, actor domain.User, username string, password string, role string, remoteIP string, userAgent string) (domain.User, error) {
	username = normalizeUsername(username)
	password = strings.TrimSpace(password)
	role = domain.NormalizeUserRole(strings.TrimSpace(role))
	if username == "" {
		return domain.User{}, errors.New("用户名不能为空")
	}
	if len(password) < minPasswordLength {
		return domain.User{}, ErrPasswordPolicy
	}
	if role == "" {
		return domain.User{}, ErrInvalidRole
	}

	passwordHash, err := hashPassword(password)
	if err != nil {
		return domain.User{}, err
	}
	user, err := s.users.Create(ctx, username, passwordHash, role)
	if err != nil {
		return domain.User{}, err
	}
	_ = s.recordEvent(ctx, domain.AuthEvent{
		UserID:    user.ID,
		Username:  user.Username,
		EventType: domain.AuthEventUserCreated,
		Detail:    "created_by:" + actor.Username,
		IP:        remoteIP,
		UserAgent: userAgent,
	})
	return user, nil
}

func (s *AuthService) UpdateUser(ctx context.Context, actor domain.User, userID int64, role string, enabled bool, remoteIP string, userAgent string) (domain.User, error) {
	if userID <= 0 {
		return domain.User{}, ErrUserNotFound
	}
	role = domain.NormalizeUserRole(strings.TrimSpace(role))
	if role == "" {
		return domain.User{}, ErrInvalidRole
	}

	record, err := s.users.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.User{}, ErrUserNotFound
		}
		return domain.User{}, err
	}
	if actor.ID == userID {
		if !enabled {
			return domain.User{}, ErrCannotDisableSelf
		}
		if role != record.Role {
			return domain.User{}, ErrCannotChangeOwnRole
		}
	}
	if err := s.validateAdminRetention(ctx, record.User, role, enabled); err != nil {
		return domain.User{}, err
	}

	now := time.Now().UTC()
	if err := s.users.Update(ctx, userID, storage.UserUpdateInput{Role: role, Enabled: &enabled}, now); err != nil {
		return domain.User{}, err
	}
	updated, err := s.users.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.User{}, ErrUserNotFound
		}
		return domain.User{}, err
	}
	if !enabled || role != record.Role {
		if err := s.sessions.DeleteByUserID(ctx, userID); err != nil {
			return domain.User{}, err
		}
	}
	_ = s.recordEvent(ctx, domain.AuthEvent{
		UserID:    updated.ID,
		Username:  updated.Username,
		EventType: domain.AuthEventUserUpdated,
		Detail:    "updated_by:" + actor.Username,
		IP:        remoteIP,
		UserAgent: userAgent,
	})
	slog.Info(
		"user updated",
		"actor", actor.Username,
		"target", updated.Username,
		"targetUserID", updated.ID,
		"role", updated.Role,
		"enabled", updated.Enabled,
		"sessionsRevoked", !enabled || role != record.Role,
		"remoteIP", remoteIP,
	)
	return updated.User, nil
}

func (s *AuthService) ResetUserPassword(ctx context.Context, actor domain.User, userID int64, newPassword string, remoteIP string, userAgent string) error {
	if userID <= 0 {
		return ErrUserNotFound
	}
	newPassword = strings.TrimSpace(newPassword)
	if len(newPassword) < minPasswordLength {
		return ErrPasswordPolicy
	}
	record, err := s.users.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUserNotFound
		}
		return err
	}
	passwordHash, err := hashPassword(newPassword)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := s.users.UpdatePassword(ctx, userID, passwordHash, now); err != nil {
		return err
	}
	if err := s.sessions.DeleteByUserID(ctx, userID); err != nil {
		return err
	}
	_ = s.recordEvent(ctx, domain.AuthEvent{
		UserID:    record.ID,
		Username:  record.Username,
		EventType: domain.AuthEventUserPasswordReset,
		Detail:    "reset_by:" + actor.Username,
		IP:        remoteIP,
		UserAgent: userAgent,
	})
	slog.Warn("user password reset", "actor", actor.Username, "target", record.Username, "targetUserID", record.ID, "remoteIP", remoteIP)
	return nil
}

func (s *AuthService) RevokeUserSessions(ctx context.Context, actor domain.User, userID int64, remoteIP string, userAgent string) error {
	if userID <= 0 {
		return ErrUserNotFound
	}
	record, err := s.users.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUserNotFound
		}
		return err
	}
	if err := s.sessions.DeleteByUserID(ctx, userID); err != nil {
		return err
	}
	_ = s.recordEvent(ctx, domain.AuthEvent{
		UserID:    record.ID,
		Username:  record.Username,
		EventType: domain.AuthEventUserSessionsRevoked,
		Detail:    "revoked_by:" + actor.Username,
		IP:        remoteIP,
		UserAgent: userAgent,
	})
	slog.Warn("user sessions revoked", "actor", actor.Username, "target", record.Username, "targetUserID", record.ID, "remoteIP", remoteIP)
	return nil
}

func (s *AuthService) CreateAPIToken(ctx context.Context, actor domain.User, name string, expiresInHours int, scopes []string, remoteIP string, userAgent string) (domain.APIToken, string, error) {
	if actor.ID <= 0 || s.apiTokens == nil {
		return domain.APIToken{}, "", ErrUnauthenticated
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.APIToken{}, "", errors.New("API Token 名称不能为空")
	}
	if expiresInHours < 0 {
		return domain.APIToken{}, "", errors.New("过期时间无效")
	}
	normalizedScopes, err := normalizeAPITokenScopes(scopes)
	if err != nil {
		return domain.APIToken{}, "", err
	}

	token, tokenHash, err := newSessionToken()
	if err != nil {
		return domain.APIToken{}, "", err
	}
	now := time.Now().UTC()
	prefix := token
	if len(prefix) > 10 {
		prefix = prefix[:10]
	}
	var expiresAt *time.Time
	if expiresInHours > 0 {
		value := now.Add(time.Duration(expiresInHours) * time.Hour)
		expiresAt = &value
	}
	item, err := s.apiTokens.Create(ctx, actor.ID, name, tokenHash, prefix, normalizedScopes, expiresAt, now)
	if err != nil {
		return domain.APIToken{}, "", err
	}
	_ = s.recordEvent(ctx, domain.AuthEvent{
		UserID:    actor.ID,
		Username:  actor.Username,
		EventType: domain.AuthEventAPITokenCreated,
		Detail:    "token:" + name,
		IP:        remoteIP,
		UserAgent: userAgent,
	})
	return item, token, nil
}

func (s *AuthService) ListAPITokens(ctx context.Context, actor domain.User) ([]domain.APIToken, error) {
	if actor.ID <= 0 || s.apiTokens == nil {
		return nil, ErrUnauthenticated
	}
	return s.apiTokens.ListActiveByUserID(ctx, actor.ID)
}

func (s *AuthService) RevokeAPIToken(ctx context.Context, actor domain.User, tokenID int64, remoteIP string, userAgent string) error {
	if actor.ID <= 0 || s.apiTokens == nil {
		return ErrUnauthenticated
	}
	record, err := s.apiTokens.GetByID(ctx, tokenID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAPITokenNotFound
		}
		return err
	}
	if record.UserID != actor.ID {
		return ErrAPITokenNotFound
	}
	if record.RevokedAt != nil && !record.RevokedAt.IsZero() {
		return ErrAPITokenNotFound
	}
	if err := s.apiTokens.Revoke(ctx, tokenID, time.Now().UTC()); err != nil {
		return err
	}
	_ = s.recordEvent(ctx, domain.AuthEvent{
		UserID:    actor.ID,
		Username:  actor.Username,
		EventType: domain.AuthEventAPITokenRevoked,
		Detail:    "token:" + record.Name,
		IP:        remoteIP,
		UserAgent: userAgent,
	})
	return nil
}

func (s *AuthService) CleanupAPITokens(ctx context.Context, now time.Time, retention time.Duration) error {
	if s.apiTokens == nil {
		return nil
	}
	cutoff := now.UTC()
	if retention > 0 {
		cutoff = cutoff.Add(-retention)
	}
	return s.apiTokens.DeleteExpiredOrRevokedBefore(ctx, cutoff)
}

func (s *AuthService) validateAdminRetention(ctx context.Context, current domain.User, nextRole string, nextEnabled bool) error {
	if current.Role != domain.RoleAdmin || !current.Enabled {
		return nil
	}
	if nextRole == domain.RoleAdmin && nextEnabled {
		return nil
	}
	users, err := s.users.List(ctx)
	if err != nil {
		return err
	}
	for _, user := range users {
		if user.ID == current.ID {
			continue
		}
		if user.Role == domain.RoleAdmin && user.Enabled {
			return nil
		}
	}
	return ErrLastEnabledAdminRequired
}

func (s *AuthService) IsInitialized(ctx context.Context) (bool, error) {
	now := time.Now().UTC()

	s.mu.Lock()
	cached := s.uninitializedCachedUntil
	s.mu.Unlock()
	if cached.After(now) {
		return false, nil
	}

	count, err := s.users.Count(ctx)
	if err != nil {
		return false, err
	}
	if count == 0 {
		s.mu.Lock()
		s.uninitializedCachedUntil = now.Add(uninitializedCacheTTL)
		s.mu.Unlock()
		return false, nil
	}

	s.mu.Lock()
	s.uninitializedCachedUntil = time.Time{}
	s.mu.Unlock()
	return true, nil
}

func (s *AuthService) pruneExpiredSessionsIfNeeded(ctx context.Context, now time.Time) error {
	if s.sessions == nil {
		return nil
	}

	s.mu.Lock()
	lastRun := s.lastExpiredSessionPrune
	if !lastRun.IsZero() && now.Sub(lastRun) < expiredSessionCleanupTTL {
		s.mu.Unlock()
		return nil
	}
	s.lastExpiredSessionPrune = now
	s.mu.Unlock()

	if err := s.sessions.DeleteExpired(ctx, now); err != nil {
		s.mu.Lock()
		if s.lastExpiredSessionPrune.Equal(now) {
			s.lastExpiredSessionPrune = lastRun
		}
		s.mu.Unlock()
		return err
	}

	return nil
}

func (s *AuthService) isLocked(key string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	attempt, ok := s.attempts[key]
	if !ok {
		return false
	}
	if !attempt.lockedUntil.IsZero() && attempt.lockedUntil.After(now) {
		return true
	}
	if !attempt.lockedUntil.IsZero() && !attempt.lockedUntil.After(now) {
		delete(s.attempts, key)
	}
	return false
}

func (s *AuthService) pruneStaleAttempts(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, attempt := range s.attempts {
		if !attempt.lockedUntil.IsZero() && attempt.lockedUntil.After(now) {
			continue
		}
		if now.Sub(attempt.firstFailedAt) > loginFailureWindow*2 {
			delete(s.attempts, key)
		}
	}
}

func (s *AuthService) registerFailure(key string, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	attempt := s.attempts[key]
	if attempt.firstFailedAt.IsZero() || now.Sub(attempt.firstFailedAt) > loginFailureWindow {
		attempt = loginAttempt{count: 0, firstFailedAt: now}
	}
	attempt.count++
	if attempt.count >= maxLoginFailures {
		attempt.lockedUntil = now.Add(loginLockDuration)
		slog.Warn("login locked due to too many attempts", "key", key, "lockedUntil", attempt.lockedUntil, "count", attempt.count)
	}
	s.attempts[key] = attempt
}

func (s *AuthService) clearFailures(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.attempts, key)
}

func (s *AuthService) recordEvent(ctx context.Context, event domain.AuthEvent) error {
	if s.events == nil {
		return nil
	}
	return s.events.Create(ctx, event)
}

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func normalizeUsername(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeAPITokenScopes(scopes []string) ([]string, error) {
	validScopes := map[string]struct{}{
		domain.ScopeAll:                   {},
		domain.ScopeServersRead:           {},
		domain.ScopeServersWrite:          {},
		domain.ScopeCommandsRead:          {},
		domain.ScopeCommandsExecute:       {},
		domain.ScopeCommandTemplatesWrite: {},
		domain.ScopeAlertsRead:            {},
		domain.ScopeAlertsWrite:           {},
	}
	seen := map[string]struct{}{}
	items := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if _, ok := validScopes[scope]; !ok {
			return nil, ErrInvalidAPITokenScope
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		items = append(items, scope)
	}
	if len(items) == 0 {
		return nil, ErrInvalidAPITokenScope
	}
	if _, hasAll := seen[domain.ScopeAll]; hasAll && len(items) > 1 {
		return nil, ErrInvalidAPITokenScope
	}
	return items, nil
}

func loginAttemptKey(username string, remoteIP string) string {
	remoteIP = strings.TrimSpace(remoteIP)
	if remoteIP == "" {
		return username
	}
	return username + "|" + remoteIP
}

func newSessionToken() (string, string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}

	token := base64.RawURLEncoding.EncodeToString(buf)
	return token, hashSessionToken(token), nil
}

func hashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
