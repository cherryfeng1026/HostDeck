package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/service"
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
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	payload, err := decodeSessionPayload(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	user, token, expiresAt, err := h.service.Login(r.Context(), payload.Username, payload.Password, clientIP(r), r.UserAgent())
	if err != nil {
		status := http.StatusUnauthorized
		if errors.Is(err, service.ErrTooManyLoginAttempts) {
			status = http.StatusTooManyRequests
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

	payload, err := decodeChangePasswordPayload(r)
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

	payload, err := decodeSessionPayload(r)
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
			"canManageUsers":         domain.CanManageUsers(user.Role),
			"canChangeOwnPassword":   true,
		},
	}
}

func decodeSessionPayload(r *http.Request) (sessionLoginPayload, error) {
	defer r.Body.Close()

	var payload sessionLoginPayload
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return sessionLoginPayload{}, err
	}
	return payload, nil
}

func decodeChangePasswordPayload(r *http.Request) (changePasswordPayload, error) {
	defer r.Body.Close()

	var payload changePasswordPayload
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return changePasswordPayload{}, err
	}
	return payload, nil
}

func (h *AuthHandler) currentUser(r *http.Request) (domain.User, error) {
	token, err := h.sessionTokenFromRequest(r)
	if err != nil {
		return domain.User{}, err
	}
	return h.service.Authenticate(r.Context(), token)
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
	forwardedFor := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if forwardedFor != "" {
		parts := strings.Split(forwardedFor, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}
