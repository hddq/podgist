package http

import (
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hddq/podgist/internal/service"
)

func NewRouter(
	auth *service.AuthService,
	handlers *Handlers,
	dashHandlers *DashboardHandlers,
	realm string,
	logger *slog.Logger,
	webFS fs.FS,
) http.Handler {
	r := chi.NewRouter()
	r.Use(RequestLogger(logger))

	registerAPIRoutes(r, auth, handlers, realm)
	registerDashboardRoutes(r, auth, dashHandlers)
	registerSPARoutes(r, webFS)

	return r
}

func NewAPIRouter(
	auth *service.AuthService,
	handlers *Handlers,
	realm string,
	logger *slog.Logger,
) http.Handler {
	r := chi.NewRouter()
	r.Use(RequestLogger(logger))

	registerAPIRoutes(r, auth, handlers, realm)

	return r
}

func registerAPIRoutes(r chi.Router, auth *service.AuthService, handlers *Handlers, realm string) {
	authMW := BasicAuthMiddleware(auth, realm)
	userCheck := CheckUsernameMiddleware

	// Auth endpoints
	r.With(authMW, userCheck).Post("/api/2/auth/{username}/login.json", handlers.Login)
	r.Post("/api/2/auth/{username}/logout.json", handlers.Logout)

	// Subscriptions
	r.With(authMW, userCheck).Get("/api/{version}/subscriptions/{username}/{device_uid}.json", handlers.Subscriptions)
	r.With(authMW, userCheck).Post("/api/{version}/subscriptions/{username}/{device_uid}.json", handlers.Subscriptions)

	// Episodes
	r.With(authMW, userCheck).Get("/api/{version}/episodes/{username}.json", handlers.Episodes)
	r.With(authMW, userCheck).Post("/api/{version}/episodes/{username}.json", handlers.Episodes)

	// Devices
	r.With(authMW, userCheck).Get("/api/{version}/devices/{username}/{device_uid}.json", handlers.DeviceUpdate)
	r.With(authMW, userCheck).Post("/api/{version}/devices/{username}/{device_uid}.json", handlers.DeviceUpdate)
	r.With(authMW, userCheck).Put("/api/{version}/devices/{username}/{device_uid}.json", handlers.DeviceUpdate)
	r.With(authMW, userCheck).Get("/api/{version}/devices/{username}.json", handlers.DeviceList)

	// Updates
	r.With(authMW, userCheck).Get("/api/2/updates/{username}/{device_uid}.json", handlers.Updates)

	// Sync
	r.With(authMW, userCheck).Get("/api/2/sync-devices/{username}.json", handlers.SyncDevices)
	r.With(authMW, userCheck).Post("/api/2/sync-devices/{username}.json", handlers.SyncDevices)

	// Settings
	r.With(authMW, userCheck).Get("/api/2/settings/{username}/{scope}.json", handlers.Settings)
	r.With(authMW, userCheck).Post("/api/2/settings/{username}/{scope}.json", handlers.Settings)
}

func registerDashboardRoutes(r chi.Router, auth *service.AuthService, dashHandlers *DashboardHandlers) {
	// Dashboard API — /api/podgist/v1
	if dashHandlers != nil {
		sessionMW := SessionAuthMiddleware(auth)
		r.Route("/api/podgist/v1", func(r chi.Router) {
			r.Post("/register", dashHandlers.Register)
			r.Post("/login", dashHandlers.Login)
			r.Post("/logout", dashHandlers.Logout)

			r.With(sessionMW).Get("/me", dashHandlers.Me)
			r.With(sessionMW).Get("/dashboard", dashHandlers.Dashboard)
			r.With(sessionMW).Get("/history", dashHandlers.History)
			r.With(sessionMW).Get("/subscriptions", dashHandlers.Subscriptions)
			r.With(sessionMW).Get("/devices", dashHandlers.Devices)
			r.With(sessionMW).Get("/account", dashHandlers.Account)
		})
	}
}

func registerSPARoutes(r chi.Router, webFS fs.FS) {
	// Dashboard SPA
	if webFS != nil {
		spaHandler := spaFileServer(webFS)
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/app", http.StatusFound)
		})
		r.Get("/app", spaHandler.ServeHTTP)
		r.Get("/app/*", spaHandler.ServeHTTP)
	}
}

func spaFileServer(webFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(webFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/app")
		path = strings.TrimPrefix(path, "/")
		if path == "" {
			path = "index.html"
		}

		if path == "index.html" {
			serveIndexHTML(w, r, webFS)
			return
		}

		// Try to serve the file directly
		if f, err := webFS.Open(path); err == nil {
			f.Close()
			serveReq := r.Clone(r.Context())
			serveReq.URL.Path = "/" + path
			fileServer.ServeHTTP(w, serveReq)
			return
		}

		// SPA fallback: serve index.html for non-file routes
		serveIndexHTML(w, r, webFS)
	})
}

func serveIndexHTML(w http.ResponseWriter, r *http.Request, webFS fs.FS) {
	data, err := fs.ReadFile(webFS, "index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, "index.html", time.Time{}, strings.NewReader(string(data)))
}
