package httpx

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"hostdeck/server/internal/authctx"
	"hostdeck/server/internal/domain"
)

type SessionAuthenticator interface {
	AuthenticateSession(ctx context.Context, token string) (domain.User, error)
	AuthenticateAPIToken(ctx context.Context, token string) (domain.User, []string, error)
}

func NewSessionAuthMiddleware(authenticator SessionAuthenticator, cookieName string) func(http.Handler) http.Handler {
	cookieName = strings.TrimSpace(cookieName)

	return func(next http.Handler) http.Handler {
		if next == nil {
			next = http.NotFoundHandler()
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authenticator == nil {
				logAuthFailure(r, "authenticator_unavailable")
				writeAuthError(w, http.StatusUnauthorized, "请先登录")
				return
			}

			if token := bearerTokenFromRequest(r); token != "" {
				user, scopes, err := authenticator.AuthenticateAPIToken(r.Context(), token)
				if err != nil {
					logAuthFailure(r, "invalid_api_token")
					writeAuthError(w, http.StatusUnauthorized, "API Token 无效或已失效")
					return
				}
				ctx := authctx.WithCurrentUser(r.Context(), user)
				ctx = authctx.WithAuthMethod(ctx, authctx.AuthMethodAPIToken)
				ctx = authctx.WithScopes(ctx, scopes)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			if cookieName == "" {
				logAuthFailure(r, "session_cookie_unconfigured")
				writeAuthError(w, http.StatusUnauthorized, "请先登录")
				return
			}
			cookie, err := r.Cookie(cookieName)
			if err != nil || strings.TrimSpace(cookie.Value) == "" {
				logAuthFailure(r, "missing_session_cookie")
				writeAuthError(w, http.StatusUnauthorized, "请先登录")
				return
			}

			user, err := authenticator.AuthenticateSession(r.Context(), cookie.Value)
			if err != nil {
				logAuthFailure(r, "invalid_or_expired_session")
				writeAuthError(w, http.StatusUnauthorized, "登录状态已失效")
				return
			}

			ctx := authctx.WithCurrentUser(r.Context(), user)
			ctx = authctx.WithAuthMethod(ctx, authctx.AuthMethodSession)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func CurrentUser(ctx context.Context) (domain.User, bool) {
	return authctx.CurrentUser(ctx)
}

func CurrentAuthMethod(ctx context.Context) (authctx.AuthMethod, bool) {
	return authctx.CurrentAuthMethod(ctx)
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

func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if next == nil {
			next = http.NotFoundHandler()
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			method, ok := CurrentAuthMethod(r.Context())
			if !ok {
				writeAuthError(w, http.StatusForbidden, "当前账号没有执行该操作的权限")
				return
			}
			if method == authctx.AuthMethodSession || authctx.HasScope(r.Context(), scope) {
				next.ServeHTTP(w, r)
				return
			}
			writeAuthError(w, http.StatusForbidden, "API Token 缺少所需权限范围")
		})
	}
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

func RequireSessionAuth(next http.Handler) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, ok := CurrentAuthMethod(r.Context())
		if !ok || method != authctx.AuthMethodSession {
			writeAuthError(w, http.StatusForbidden, "API Token 不支持该操作")
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

func bearerTokenFromRequest(r *http.Request) string {
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	if value == "" || len(value) < 8 {
		return ""
	}
	if !strings.EqualFold(value[:7], "Bearer ") {
		return ""
	}
	return strings.TrimSpace(value[7:])
}

func logAuthFailure(r *http.Request, reason string) {
	slog.Warn(
		"authentication failed",
		"reason", reason,
		"method", r.Method,
		"path", r.URL.Path,
		"query", r.URL.RawQuery,
		"remoteAddr", strings.TrimSpace(r.RemoteAddr),
		"hasAuthorization", bearerTokenFromRequest(r) != "",
		"userAgent", r.UserAgent(),
	)
}
