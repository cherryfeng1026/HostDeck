package api

import (
	"crypto/subtle"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"hostdeck/server/internal/authctx"
	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/service"
	"hostdeck/server/internal/storage"
)

type AuthHandler struct {
	service        *service.AuthService
	cookieName     string
	cookieSecure   bool
	bootstrapToken string
}

type sessionLoginPayload struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type changePasswordPayload struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

type createUserPayload struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type updateUserPayload struct {
	Role    string `json:"role"`
	Enabled bool   `json:"enabled"`
}

type resetUserPasswordPayload struct {
	NewPassword string `json:"newPassword"`
}

type createAPITokenPayload struct {
	Name           string   `json:"name"`
	ExpiresInHours int      `json:"expiresInHours"`
	Scopes         []string `json:"scopes"`
}

func NewAuthHandler(service *service.AuthService, cookieName string, cookieSecure bool, bootstrapToken string) *AuthHandler {
	return &AuthHandler{
		service:        service,
		cookieName:     strings.TrimSpace(cookieName),
		cookieSecure:   cookieSecure,
		bootstrapToken: strings.TrimSpace(bootstrapToken),
	}
}

func RegisterPublicAuthRoutes(r chi.Router, h *AuthHandler) {
	if h == nil {
		return
	}

	r.Get("/api/auth/status", h.Status)
	r.Post("/api/auth/login", h.Login)
	r.Post("/api/auth/logout", h.Logout)
	r.Post("/api/auth/bootstrap-admin", h.BootstrapAdmin)
}

func RegisterProtectedAuthRoutes(r chi.Router, h *AuthHandler) {
	if h == nil {
		return
	}

	r.Get("/api/auth/me", h.CurrentUser)
	r.Post("/api/auth/change-password", h.ChangePassword)
	r.Get("/api/auth/api-tokens", h.ListAPITokens)
	r.Post("/api/auth/api-tokens", h.CreateAPIToken)
	r.Delete("/api/auth/api-tokens/{id}", h.RevokeAPIToken)
	r.Get("/api/users", h.ListUsers)
	r.Post("/api/users", h.CreateUser)
	r.Put("/api/users/{id}", h.UpdateUser)
	r.Post("/api/users/{id}/reset-password", h.ResetUserPassword)
	r.Post("/api/users/{id}/revoke-sessions", h.RevokeUserSessions)
}

func (h *AuthHandler) Status(w http.ResponseWriter, r *http.Request) {
	response := map[string]any{
		"bootstrapEnabled": h.bootstrapToken != "",
		"authenticated":    false,
	}

	if _, err := h.sessionTokenFromRequest(r); err == nil {
		user, authErr := h.currentUser(r)
		switch {
		case authErr == nil:
			response["initialized"] = true
			response["authenticated"] = true
			for key, value := range authResponse(user) {
				response[key] = value
			}
			writeJSON(w, http.StatusOK, response)
			return
		case errors.Is(authErr, service.ErrUnauthenticated), errors.Is(authErr, service.ErrUserDisabled):
		default:
			writeError(w, http.StatusInternalServerError, authErr)
			return
		}
	}

	initialized, err := h.service.IsInitialized(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	response["initialized"] = initialized
	writeJSON(w, http.StatusOK, response)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	payload, err := decodeJSON[sessionLoginPayload](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	user, token, expiresAt, err := h.service.Login(r.Context(), payload.Username, payload.Password, clientIP(r), r.UserAgent())
	if err != nil {
		status := http.StatusUnauthorized
		switch {
		case errors.Is(err, service.ErrTooManyLoginAttempts):
			status = http.StatusTooManyRequests
		case errors.Is(err, service.ErrSystemUninitialized):
			status = http.StatusPreconditionFailed
		case errors.Is(err, service.ErrUserDisabled):
			status = http.StatusForbidden
		}
		writeError(w, status, err)
		return
	}

	h.writeSessionCookie(w, token, expiresAt)
	writeJSON(w, http.StatusOK, authResponse(user))
}

func (h *AuthHandler) CurrentUser(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	writeJSON(w, http.StatusOK, authResponse(user))
}

func (h *AuthHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *AuthHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	actor, err := h.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	payload, err := decodeJSON[createUserPayload](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	user, err := h.service.CreateUser(r.Context(), actor, payload.Username, payload.Password, payload.Role, clientIP(r), r.UserAgent())
	if err != nil {
		writeUserManagementError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

func (h *AuthHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	actor, err := h.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	userID, err := parseUserID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	payload, err := decodeJSON[updateUserPayload](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	user, err := h.service.UpdateUser(r.Context(), actor, userID, payload.Role, payload.Enabled, clientIP(r), r.UserAgent())
	if err != nil {
		writeUserManagementError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (h *AuthHandler) ResetUserPassword(w http.ResponseWriter, r *http.Request) {
	actor, err := h.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	userID, err := parseUserID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	payload, err := decodeJSON[resetUserPasswordPayload](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.service.ResetUserPassword(r.Context(), actor, userID, payload.NewPassword, clientIP(r), r.UserAgent()); err != nil {
		writeUserManagementError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) RevokeUserSessions(w http.ResponseWriter, r *http.Request) {
	actor, err := h.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	userID, err := parseUserID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.service.RevokeUserSessions(r.Context(), actor, userID, clientIP(r), r.UserAgent()); err != nil {
		writeUserManagementError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) ListAPITokens(w http.ResponseWriter, r *http.Request) {
	actor, err := h.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	items, err := h.service.ListAPITokens(r.Context(), actor)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *AuthHandler) CreateAPIToken(w http.ResponseWriter, r *http.Request) {
	actor, err := h.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	payload, err := decodeJSON[createAPITokenPayload](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	item, plainToken, err := h.service.CreateAPIToken(r.Context(), actor, payload.Name, payload.ExpiresInHours, payload.Scopes, clientIP(r), r.UserAgent())
	if err != nil {
		writeUserManagementError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"token": plainToken,
		"item":  item,
	})
}

func (h *AuthHandler) RevokeAPIToken(w http.ResponseWriter, r *http.Request) {
	actor, err := h.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	tokenID, err := parseUserID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.service.RevokeAPIToken(r.Context(), actor, tokenID, clientIP(r), r.UserAgent()); err != nil {
		writeUserManagementError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	token, err := h.sessionTokenFromRequest(r)
	if err == nil {
		if logoutErr := h.service.Logout(r.Context(), token, clientIP(r), r.UserAgent()); logoutErr != nil {
			writeError(w, http.StatusInternalServerError, logoutErr)
			return
		}
	}

	h.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}

	payload, err := decodeJSON[changePasswordPayload](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if err := h.service.ChangePassword(r.Context(), user.ID, payload.CurrentPassword, payload.NewPassword, clientIP(r), r.UserAgent()); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, service.ErrInvalidCredentials) {
			status = http.StatusUnauthorized
		}
		writeError(w, status, err)
		return
	}

	h.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) BootstrapAdmin(w http.ResponseWriter, r *http.Request) {
	if h.bootstrapToken == "" {
		writeError(w, http.StatusNotFound, errors.New("初始化引导未启用"))
		return
	}
	if !constantTimeEqual(strings.TrimSpace(r.Header.Get("X-HostDeck-Bootstrap-Token")), h.bootstrapToken) {
		writeError(w, http.StatusForbidden, errors.New("初始化令牌无效"))
		return
	}

	payload, err := decodeJSON[sessionLoginPayload](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if _, err := h.service.CreateInitialAdmin(r.Context(), payload.Username, payload.Password, clientIP(r), r.UserAgent(), "bootstrap_api"); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	user, token, expiresAt, err := h.service.Login(r.Context(), payload.Username, payload.Password, clientIP(r), r.UserAgent())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	h.writeSessionCookie(w, token, expiresAt)
	writeJSON(w, http.StatusCreated, authResponse(user))
}

func authResponse(user domain.User) map[string]any {
	return map[string]any{
		"user": user,
		"permissions": map[string]bool{
			"canManageInfrastructure": domain.CanManageInfrastructure(user.Role),
			"canManageUsers":          domain.CanManageUsers(user.Role),
			"canChangeOwnPassword":    true,
		},
	}
}

func parseUserID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

func writeUserManagementError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, service.ErrUserNotFound), errors.Is(err, service.ErrAPITokenNotFound):
		status = http.StatusNotFound
	case errors.Is(err, service.ErrInvalidRole), errors.Is(err, service.ErrPasswordPolicy), errors.Is(err, service.ErrCannotDisableSelf), errors.Is(err, service.ErrCannotChangeOwnRole), errors.Is(err, service.ErrLastEnabledAdminRequired), errors.Is(err, service.ErrInvalidAPITokenScope):
		status = http.StatusBadRequest
	case errors.Is(err, storage.ErrUserUsernameConflict):
		status = http.StatusConflict
	}
	writeError(w, status, err)
}

func (h *AuthHandler) currentUser(r *http.Request) (domain.User, error) {
	if user, ok := authctx.CurrentUser(r.Context()); ok {
		return user, nil
	}
	token, err := h.sessionTokenFromRequest(r)
	if err != nil {
		return domain.User{}, err
	}
	return h.service.AuthenticateSession(r.Context(), token)
}

func (h *AuthHandler) sessionTokenFromRequest(r *http.Request) (string, error) {
	cookie, err := r.Cookie(h.cookieName)
	if err != nil {
		return "", service.ErrUnauthenticated
	}
	token := strings.TrimSpace(cookie.Value)
	if token == "" {
		return "", service.ErrUnauthenticated
	}
	return token, nil
}

func (h *AuthHandler) writeSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteStrictMode,
		Expires:  expiresAt,
		MaxAge:   maxAgeSeconds(expiresAt),
	})
}

func (h *AuthHandler) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Unix(0, 0).UTC(),
		MaxAge:   -1,
	})
}

func maxAgeSeconds(expiresAt time.Time) int {
	seconds := int(time.Until(expiresAt).Seconds())
	if seconds < 0 {
		return 0
	}
	return seconds
}

func constantTimeEqual(left string, right string) bool {
	if len(left) != len(right) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}
