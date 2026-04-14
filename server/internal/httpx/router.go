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
		api.RegisterShellRoutes(target, options.shellHandler)
		if options.authHandler != nil {
			target.Get("/api/auth/me", options.authHandler.CurrentUser)
			target.Post("/api/auth/change-password", options.authHandler.ChangePassword)
			target.With(RequireUserManagementAccess).Get("/api/users", options.authHandler.ListUsers)
		}
	}
	registerManagedProtected := func(target chi.Router) {
		api.RegisterServerWriteRoutes(target, serverHandler)
		api.RegisterProbeRoutes(target, probeHandler)
		api.RegisterCommandRoutes(target, commandHandler)
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
