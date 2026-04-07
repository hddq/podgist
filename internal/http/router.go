package http

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/hddq/podgist/internal/service"
)

func NewRouter(
	auth *service.AuthService,
	handlers *Handlers,
	realm string,
	logger *slog.Logger,
) http.Handler {
	r := chi.NewRouter()
	r.Use(RequestLogger(logger))

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

	return r
}
