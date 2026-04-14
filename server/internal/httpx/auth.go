package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"hostdeck/server/internal/domain"
)

type userContextKey string

const currentUserKey userContextKey = "hostdeck_current_user"

type SessionAuthenticator interface {
	Authenticate(ctx context.Context, token string) (domain.User, error)
}

func NewSessionAuthMiddleware(authenticator SessionAuthenticator, cookieName string) func(http.Handler) http.Handler {
	cookieName = strings.TrimSpace(cookieName)

	return func(next http.Handler) http.Handler {
		if next == nil {
			next = http.NotFoundHandler()
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authenticator == nil || cookieName == "" {
				writeAuthError(w, http.StatusUnauthorized, "请先登录")
				return
			}

			cookie, err := r.Cookie(cookieName)
			if err != nil || strings.TrimSpace(cookie.Value) == "" {
				writeAuthError(w, http.StatusUnauthorized, "请先登录")
				return
			}

			user, err := authenticator.Authenticate(r.Context(), cookie.Value)
			if err != nil {
				writeAuthError(w, http.StatusUnauthorized, "登录状态已失效")
				return
			}

			ctx := context.WithValue(r.Context(), currentUserKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func CurrentUser(ctx context.Context) (domain.User, bool) {
	user, ok := ctx.Value(currentUserKey).(domain.User)
	return user, ok
}

func RequireInfrastructureAccess(next http.Handler) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := CurrentUser(r.Context())
		if !ok || !domain.CanManageInfrastructure(user.Role) {
			writeAuthError(w, http.StatusForbidden, "当前账号没有执行该操作的权限")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func RequireUserManagementAccess(next http.Handler) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := CurrentUser(r.Context())
		if !ok || !domain.CanManageUsers(user.Role) {
			writeAuthError(w, http.StatusForbidden, "当前账号没有查看用户管理信息的权限")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}
