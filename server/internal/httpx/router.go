package httpx

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"hostdeck/server/internal/api"
)

type RouterOption func(*routerOptions)

type routerOptions struct {
	authHandler   *api.AuthHandler
	shellHandler  *api.ShellHandler
	apiMiddleware func(http.Handler) http.Handler
	actionGuard   func(http.Handler) http.Handler
}

func WithAuthHandler(handler *api.AuthHandler) RouterOption {
	return func(options *routerOptions) {
		options.authHandler = handler
	}
}

func WithShellHandler(handler *api.ShellHandler) RouterOption {
	return func(options *routerOptions) {
		options.shellHandler = handler
	}
}

func WithAPIMiddleware(middleware func(http.Handler) http.Handler) RouterOption {
	return func(options *routerOptions) {
		options.apiMiddleware = middleware
	}
}

func WithActionGuard(middleware func(http.Handler) http.Handler) RouterOption {
	return func(options *routerOptions) {
		options.actionGuard = middleware
	}
}

func NewRouter() http.Handler {
	return NewRouterWithHandlers(nil, nil, nil, nil, nil, nil)
}

func NewRouterWithHandlers(
	serverHandler *api.ServerHandler,
	probeHandler *api.ProbeHandler,
	overviewHandler *api.OverviewHandler,
	detailHandler *api.ServerDetailHandler,
	commandHandler *api.CommandHandler,
	alertHandler *api.AlertHandler,
	opts ...RouterOption,
) http.Handler {
	options := routerOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}

	r := chi.NewRouter()
	api.RegisterHealthRoutes(r)
	api.RegisterPublicAuthRoutes(r, options.authHandler)

	registerCommonProtected := func(target chi.Router) {
		api.RegisterOverviewRoutes(target, overviewHandler)
		api.RegisterServerDetailRoutes(target, detailHandler)
		api.RegisterServerReadRoutes(target, serverHandler)
		api.RegisterAlertReadRoutes(target, alertHandler)
		api.RegisterCommandTemplateReadRoutes(target, commandHandler)
		api.RegisterShellRoutes(target, options.shellHandler)
		if options.authHandler != nil {
			target.With(RequireSessionAuth).Get("/api/auth/me", options.authHandler.CurrentUser)
			target.With(RequireSessionAuth).Post("/api/auth/change-password", options.authHandler.ChangePassword)
			target.With(RequireSessionAuth).Get("/api/auth/api-tokens", options.authHandler.ListAPITokens)
			target.With(RequireSessionAuth).Post("/api/auth/api-tokens", options.authHandler.CreateAPIToken)
			target.With(RequireSessionAuth).Delete("/api/auth/api-tokens/{id}", options.authHandler.RevokeAPIToken)
			target.With(RequireSessionAuth, RequireUserManagementAccess).Group(func(userMgmt chi.Router) {
				userMgmt.Get("/api/users", options.authHandler.ListUsers)
				userMgmt.Post("/api/users", options.authHandler.CreateUser)
				userMgmt.Put("/api/users/{id}", options.authHandler.UpdateUser)
				userMgmt.Post("/api/users/{id}/reset-password", options.authHandler.ResetUserPassword)
				userMgmt.Post("/api/users/{id}/revoke-sessions", options.authHandler.RevokeUserSessions)
			})
		}
	}
	registerManagedProtected := func(target chi.Router) {
		api.RegisterServerWriteRoutes(target, serverHandler)
		api.RegisterProbeRoutes(target, probeHandler)
		api.RegisterCommandHistoryRoutes(target, commandHandler)
		api.RegisterCommandWriteRoutes(target, commandHandler)
		api.RegisterAlertWriteRoutes(target, alertHandler)
	}

	if options.apiMiddleware != nil {
		r.Group(func(protected chi.Router) {
			protected.Use(options.apiMiddleware)
			registerCommonProtected(protected)
			if options.actionGuard != nil {
				protected.With(options.actionGuard).Group(func(guarded chi.Router) {
					registerManagedProtected(guarded)
				})
				return
			}
			registerManagedProtected(protected)
		})
		return r
	}

	registerCommonProtected(r)
	if options.actionGuard != nil {
		r.With(options.actionGuard).Group(func(guarded chi.Router) {
			registerManagedProtected(guarded)
		})
		return r
	}
	registerManagedProtected(r)
	return r
}
