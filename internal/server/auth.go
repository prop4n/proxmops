package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/prop4n/proxmops/internal/auth"
	"github.com/prop4n/proxmops/internal/store"
)

// sessionCookie is the name of the session cookie.
const sessionCookie = "proxmops_session"

// ctxKey is the private type for request-context keys.
type ctxKey int

const accountKey ctxKey = iota

// handleSetupStatus reports whether the first-run setup is still needed.
func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	needed, err := s.auth.NeedsSetup(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"needsSetup": needed})
}

// handleSetup creates the first admin account from the one-time setup token.
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token    string `json:"token"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	session, err := s.auth.Setup(r.Context(), req.Token, req.Username, req.Password)
	switch {
	case errors.Is(err, auth.ErrSetupClosed):
		writeError(w, http.StatusConflict, "setup already completed")
		return
	case errors.Is(err, auth.ErrInvalidBootstrap):
		writeError(w, http.StatusForbidden, "invalid setup token")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	s.setSessionCookie(w, session)
	writeJSON(w, http.StatusCreated, map[string]string{"username": req.Username})
}

// handleLogin authenticates a user and starts a session.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	session, err := s.auth.Login(r.Context(), req.Username, req.Password)
	if errors.Is(err, auth.ErrInvalidCredentials) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	s.setSessionCookie(w, session)
	writeJSON(w, http.StatusOK, map[string]string{"username": req.Username})
}

// handleLogout ends the current session.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		_ = s.auth.Logout(r.Context(), c.Value)
	}
	s.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// handleMe returns the authenticated account.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	account, _ := r.Context().Value(accountKey).(store.Account)
	writeJSON(w, http.StatusOK, map[string]any{
		"id":       account.ID,
		"username": account.Username,
	})
}

// requireAuth rejects unauthenticated requests and injects the account.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		account, err := s.auth.Authenticate(r.Context(), c.Value)
		if err != nil {
			s.clearSessionCookie(w)
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		ctx := context.WithValue(r.Context(), accountKey, account)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// setSessionCookie writes the session cookie.
func (s *Server) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(s.auth.SessionTTL().Seconds()),
	})
}

// clearSessionCookie expires the session cookie.
func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// decodeJSON decodes a JSON request body, writing a 400 on failure.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

// writeJSON encodes v as a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
