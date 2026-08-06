package http

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hddq/podgist/internal/domain"
	"github.com/hddq/podgist/internal/service"
)

type contextKey string

const userContextKey contextKey = "user"
const sessionContextKey contextKey = "session"
const sessionCookieName = "sessionid"

func UserFromContext(ctx context.Context) *domain.User {
	u, _ := ctx.Value(userContextKey).(*domain.User)
	return u
}

func SessionFromContext(ctx context.Context) *domain.Session {
	s, _ := ctx.Value(sessionContextKey).(*domain.Session)
	return s
}

func sessionCookie(session *domain.Session) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    session.ID,
		Path:     "/",
		Expires:  session.ExpiresAt,
		MaxAge:   int(time.Until(session.ExpiresAt).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

func clearSessionCookie() *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0).UTC(),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

func unauthorized(w http.ResponseWriter, realm string) {
	w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
}

func parseBasicAuthHeader(r *http.Request) (string, string, bool, error) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return "", "", false, nil
	}

	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Basic") {
		return "", "", false, errors.New("invalid authorization header")
	}

	decoded, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", false, fmt.Errorf("could not decode credentials: %w", err)
	}

	username, password, ok := strings.Cut(string(decoded), ":")
	if !ok {
		return "", "", false, errors.New("invalid basic credentials")
	}

	return username, password, true, nil
}

func BasicAuthMiddleware(auth *service.AuthService, realm string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
				user, sessionErr := auth.GetUserBySessionID(r.Context(), cookie.Value)
				if sessionErr == nil {
					session, refreshed, refreshErr := auth.RefreshSession(r.Context(), cookie.Value)
					if refreshErr != nil {
						http.Error(w, "internal error", http.StatusInternalServerError)
						return
					}
					if refreshed {
						http.SetCookie(w, sessionCookie(session))
					}
					ctx := context.WithValue(r.Context(), userContextKey, user)
					ctx = context.WithValue(ctx, sessionContextKey, session)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				// Only clear the session cookie when the session is
				// genuinely invalid (e.g. expired/not found). Transient
				// database errors should not log the user out.
				if isInvalidSession(sessionErr) {
					http.SetCookie(w, clearSessionCookie())
				} else {
					http.Error(w, "internal error", http.StatusInternalServerError)
					return
				}
			}

			username, password, ok, err := parseBasicAuthHeader(r)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if !ok {
				unauthorized(w, realm)
				return
			}

			user, err := auth.Authenticate(r.Context(), username, password)
			if err != nil {
				unauthorized(w, realm)
				return
			}

			session, err := auth.CreateSession(r.Context(), user.ID)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			http.SetCookie(w, sessionCookie(session))
			ctx := context.WithValue(r.Context(), userContextKey, user)
			ctx = context.WithValue(ctx, sessionContextKey, session)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func CheckUsernameMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pathUsername := chi.URLParam(r, "username")
		user := UserFromContext(r.Context())
		if user == nil || !equalUsername(user.Username, pathUsername) {
			http.Error(w,
				fmt.Sprintf("username in authentication (%s) and in requested resource (%s) don't match", usernameOrEmpty(user), pathUsername),
				http.StatusBadRequest,
			)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func usernameOrEmpty(user *domain.User) string {
	if user == nil {
		return ""
	}
	return user.Username
}

func equalUsername(a, b string) bool {
	return strings.EqualFold(a, b)
}

func SessionAuthMiddleware(auth *service.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(sessionCookieName)
			if err != nil || cookie.Value == "" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			user, err := auth.GetUserBySessionID(r.Context(), cookie.Value)
			if err != nil {
				if isInvalidSession(err) {
					http.SetCookie(w, clearSessionCookie())
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
				} else {
					http.Error(w, "internal error", http.StatusInternalServerError)
				}
				return
			}
			session, refreshed, err := auth.RefreshSession(r.Context(), cookie.Value)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			if refreshed {
				http.SetCookie(w, sessionCookie(session))
			}
			ctx := context.WithValue(r.Context(), userContextKey, user)
			ctx = context.WithValue(ctx, sessionContextKey, session)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger.Info("request", "method", r.Method, "path", r.URL.Path)
			next.ServeHTTP(w, r)
		})
	}
}

// isInvalidSession returns true when the error indicates the session
// itself is invalid (expired, not found, missing) rather than a
// transient infrastructure error (e.g. database locked). Only
// genuinely invalid sessions should trigger cookie clearing / 401.
func isInvalidSession(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return msg == "invalid session" || msg == "missing session"
}
