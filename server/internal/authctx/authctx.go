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
