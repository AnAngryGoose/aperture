package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookie   = "aperture_session"
	sessionDuration = 24 * time.Hour
	bcryptCost      = 12
)

// requireAuth is middleware that rejects unauthenticated requests with 401.
// It accepts authentication via:
//   - Session cookie (browser clients)
//   - Authorization: Bearer <token> header (API / curl clients)
//
// If no password has been configured yet, all requests are allowed through so
// the setup flow can proceed.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		configured, err := s.hub.Store().IsPasswordSet(r.Context())
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		// First-run: no password configured — let everything through so the
		// frontend can reach /api/auth/status and redirect to /setup.
		if !configured {
			next.ServeHTTP(w, r)
			return
		}

		token := sessionTokenFromRequest(r)
		if token == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		ok, err := s.hub.Store().ValidateSession(r.Context(), token)
		if err != nil || !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// sessionTokenFromRequest extracts the session token from the cookie or the
// Authorization header. Returns "" if neither is present.
func sessionTokenFromRequest(r *http.Request) string {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		return c.Value
	}
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}

// authStatus reports whether a password has been configured and whether the
// current request is authenticated. Always 200 — the frontend uses this to
// decide whether to show setup, login, or proceed normally.
func (s *Server) authStatus(w http.ResponseWriter, r *http.Request) {
	configured, err := s.hub.Store().IsPasswordSet(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	authenticated := false
	if configured {
		token := sessionTokenFromRequest(r)
		if token != "" {
			authenticated, _ = s.hub.Store().ValidateSession(r.Context(), token)
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{
		"configured":    configured,
		"authenticated": authenticated,
	})
}

// authSetup sets the initial admin password. Returns 409 if a password is
// already configured — use the change-password flow instead.
func (s *Server) authSetup(w http.ResponseWriter, r *http.Request) {
	configured, err := s.hub.Store().IsPasswordSet(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if configured {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "already configured; use change-password"})
		return
	}

	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Password) < 8 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password must be at least 8 characters"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcryptCost)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.hub.Store().SetPasswordHash(r.Context(), string(hash)); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	token, err := newSessionToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	exp := time.Now().Add(sessionDuration)
	if err := s.hub.Store().CreateSession(r.Context(), token, exp); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	setSessionCookie(w, token, exp)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// authLogin verifies the password and issues a session cookie.
func (s *Server) authLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	hash, err := s.hub.Store().GetPasswordHash(r.Context())
	if err != nil || hash == "" {
		// No password set — treat as not configured.
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not configured"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(body.Password)); err != nil {
		// Constant-time: bcrypt.CompareHashAndPassword already is.
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid password"})
		return
	}

	token, err := newSessionToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	exp := time.Now().Add(sessionDuration)
	if err := s.hub.Store().CreateSession(r.Context(), token, exp); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	setSessionCookie(w, token, exp)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// authLogout deletes the session and clears the cookie.
func (s *Server) authLogout(w http.ResponseWriter, r *http.Request) {
	token := sessionTokenFromRequest(r)
	if token != "" {
		_ = s.hub.Store().DeleteSession(r.Context(), token)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// authChangePassword replaces the admin password and rotates the session.
func (s *Server) authChangePassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if len(body.New) < 8 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "new password must be at least 8 characters"})
		return
	}

	hash, err := s.hub.Store().GetPasswordHash(r.Context())
	if err != nil || hash == "" {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(body.Current)); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "current password incorrect"})
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(body.New), bcryptCost)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.hub.Store().SetPasswordHash(r.Context(), string(newHash)); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	// Invalidate the old session and issue a fresh one.
	oldToken := sessionTokenFromRequest(r)
	if oldToken != "" {
		_ = s.hub.Store().DeleteSession(r.Context(), oldToken)
	}
	token, err := newSessionToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	exp := time.Now().Add(sessionDuration)
	if err := s.hub.Store().CreateSession(r.Context(), token, exp); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	setSessionCookie(w, token, exp)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func newSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func setSessionCookie(w http.ResponseWriter, token string, exp time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  exp,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// PruneSessions runs a periodic cleanup of expired sessions. Call in a
// goroutine alongside hub.Run.
func PruneSessions(ctx context.Context, st interface {
	PruneExpiredSessions(ctx context.Context) error
}) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = st.PruneExpiredSessions(ctx)
		}
	}
}
