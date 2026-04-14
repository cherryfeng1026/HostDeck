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
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/storage"
)

const (
	minPasswordLength   = 8
	maxLoginFailures    = 5
	loginFailureWindow  = 15 * time.Minute
	loginLockDuration   = 15 * time.Minute
	sessionTouchWindow  = 5 * time.Minute
)

var (
	ErrInvalidCredentials      = errors.New("用户名或密码错误")
	ErrTooManyLoginAttempts    = errors.New("登录失败次数过多，请稍后再试")
	ErrUnauthenticated         = errors.New("请先登录")
	ErrBootstrapAlreadyDone    = errors.New("初始管理员已创建")
	ErrBootstrapPasswordPolicy = fmt.Errorf("管理员密码至少需要 %d 位", minPasswordLength)
	ErrPasswordPolicy          = fmt.Errorf("新密码至少需要 %d 位", minPasswordLength)
)

type AuthUserStore interface {
	Count(ctx context.Context) (int, error)
	Create(ctx context.Context, username string, passwordHash string, role string) (domain.User, error)
	GetByUsername(ctx context.Context, username string) (storage.UserRecord, error)
	GetByID(ctx context.Context, id int64) (storage.UserRecord, error)
	List(ctx context.Context) ([]domain.User, error)
	UpdateLastLoginAt(ctx context.Context, id int64, at time.Time) error
	UpdatePassword(ctx context.Context, id int64, passwordHash string, updatedAt time.Time) error
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

type loginAttempt struct {
	count         int
	firstFailedAt time.Time
	lockedUntil   time.Time
}

type AuthService struct {
	users      AuthUserStore
	sessions   AuthSessionStore
	events     AuthEventStore
	sessionTTL time.Duration

	mu       sync.Mutex
	attempts map[string]loginAttempt
}

func NewAuthService(users AuthUserStore, sessions AuthSessionStore, events AuthEventStore, sessionTTL time.Duration) *AuthService {
	if sessionTTL <= 0 {
		sessionTTL = 24 * time.Hour
	}

	return &AuthService{
		users:      users,
		sessions:   sessions,
		events:     events,
		sessionTTL: sessionTTL,
		attempts:   map[string]loginAttempt{},
	}
}

func (s *AuthService) EnsureBootstrapAdmin(ctx context.Context, username string, password string) error {
	username = normalizeUsername(username)
	password = strings.TrimSpace(password)
	if username == "" && password == "" {
		count, err := s.users.Count(ctx)
		if err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		_, err = s.CreateInitialAdmin(ctx, "admin", "admin123", "", "", "startup_default")
		return err
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

	attemptKey := loginAttemptKey(username, remoteIP)
	if s.isLocked(attemptKey, time.Now().UTC()) {
		return domain.User{}, "", time.Time{}, ErrTooManyLoginAttempts
	}
	if s.sessions != nil {
		_ = s.sessions.DeleteExpired(ctx, time.Now().UTC())
	}

	record, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.registerFailure(attemptKey, time.Now().UTC())
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

	s.clearFailures(attemptKey)
	token, tokenHash, err := newSessionToken()
	if err != nil {
		return domain.User{}, "", time.Time{}, err
	}

	now := time.Now().UTC()
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

	return user, token, expiresAt, nil
}

func (s *AuthService) Authenticate(ctx context.Context, token string) (domain.User, error) {
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

	if now.Sub(session.LastSeenAt) >= sessionTouchWindow {
		_ = s.sessions.Touch(ctx, session.ID, now)
	}

	return record.User, nil
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
	return nil
}

func (s *AuthService) ListUsers(ctx context.Context) ([]domain.User, error) {
	return s.users.List(ctx)
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
