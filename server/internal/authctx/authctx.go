package authctx

import (
	"context"

	"hostdeck/server/internal/domain"
)

type userContextKey string

type AuthMethod string

const (
	currentUserKey userContextKey = "hostdeck_current_user"
	authMethodKey  userContextKey = "hostdeck_auth_method"
	scopesKey      userContextKey = "hostdeck_scopes"

	AuthMethodSession  AuthMethod = "session"
	AuthMethodAPIToken AuthMethod = "api_token"
)

func WithCurrentUser(ctx context.Context, user domain.User) context.Context {
	return context.WithValue(ctx, currentUserKey, user)
}

func CurrentUser(ctx context.Context) (domain.User, bool) {
	user, ok := ctx.Value(currentUserKey).(domain.User)
	return user, ok
}

func WithAuthMethod(ctx context.Context, method AuthMethod) context.Context {
	return context.WithValue(ctx, authMethodKey, method)
}

func CurrentAuthMethod(ctx context.Context) (AuthMethod, bool) {
	method, ok := ctx.Value(authMethodKey).(AuthMethod)
	return method, ok
}

func WithScopes(ctx context.Context, scopes []string) context.Context {
	return context.WithValue(ctx, scopesKey, append([]string(nil), scopes...))
}

func CurrentScopes(ctx context.Context) []string {
	scopes, _ := ctx.Value(scopesKey).([]string)
	return append([]string(nil), scopes...)
}

func HasScope(ctx context.Context, scope string) bool {
	for _, item := range CurrentScopes(ctx) {
		if item == domain.ScopeAll || item == scope {
			return true
		}
	}
	return false
}
