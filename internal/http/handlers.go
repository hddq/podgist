package http

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"database/sql"
	"github.com/go-chi/chi/v5"
	"github.com/hddq/podgist/internal/domain"
	"github.com/hddq/podgist/internal/service"
)

type Handlers struct {
	auth          *service.AuthService
	subscriptions *service.SubscriptionService
	episodes      *service.EpisodeService
	devices       *service.DeviceService
	sync          *service.SyncService
	settings      *service.SettingsService
	updates       *service.UpdatesService
	maxReqSize    int
	logger        *slog.Logger
}

func NewHandlers(
	auth *service.AuthService,
	subs *service.SubscriptionService,
	eps *service.EpisodeService,
	devs *service.DeviceService,
	sync *service.SyncService,
	settings *service.SettingsService,
	updates *service.UpdatesService,
	maxReqSize int,
	logger *slog.Logger,
) *Handlers {
	return &Handlers{
		auth:          auth,
		subscriptions: subs,
		episodes:      eps,
		devices:       devs,
		sync:          sync,
		settings:      settings,
		updates:       updates,
		maxReqSize:    maxReqSize,
		logger:        logger,
	}
}

// --- Auth ---

func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		user, err := h.auth.GetUserBySessionID(r.Context(), cookie.Value)
		if err == nil {
			if !equalUsername(user.Username, username) {
				http.Error(w,
					"username in authentication ("+user.Username+") and in requested resource ("+username+") don't match",
					http.StatusBadRequest,
				)
				return
			}
			if err := h.auth.DeleteSession(r.Context(), cookie.Value); err != nil {
				h.logger.Error("delete session", "error", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
		}
	}
	http.SetCookie(w, clearSessionCookie())
	w.WriteHeader(http.StatusOK)
}

// --- Subscriptions ---

func (h *Handlers) Subscriptions(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	deviceUID := chi.URLParam(r, "device_uid")

	dev, err := h.devices.GetOrCreate(r.Context(), user.ID, deviceUID)
	if err != nil {
		h.logger.Error("get/create device", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	now := time.Now().UTC()

	switch r.Method {
	case http.MethodGet:
		var since *time.Time
		if s := r.URL.Query().Get("since"); s != "" {
			ts, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				http.Error(w, "since-value is not a valid timestamp", http.StatusBadRequest)
				return
			}
			t := time.Unix(ts, 0).UTC()
			since = &t
		}

		result, err := h.subscriptions.GetSubscriptions(r.Context(), user.ID, dev.ID, since, now)
		if err != nil {
			h.logger.Error("get subscriptions", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, result)

	case http.MethodPost:
		var body struct {
			Add    []string `json:"add"`
			Remove []string `json:"remove"`
		}
		if err := readJSON(r, h.maxReqSize, &body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		result, err := h.subscriptions.UpdateSubscriptions(r.Context(), user.ID, dev.ID, body.Add, body.Remove, now)
		if err != nil {
			h.logger.Error("update subscriptions", "error", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// --- Episodes ---

func (h *Handlers) Episodes(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	now := time.Now().UTC()

	switch r.Method {
	case http.MethodGet:
		var since *time.Time
		if s := r.URL.Query().Get("since"); s != "" {
			ts, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				http.Error(w, "since-value is not a valid timestamp", http.StatusBadRequest)
				return
			}
			t := time.Unix(ts, 0).UTC()
			since = &t
		}

		var podcastURL *string
		if p := r.URL.Query().Get("podcast"); p != "" {
			podcastURL = &p
		}

		var deviceID *int64
		if d := r.URL.Query().Get("device"); d != "" {
			dev, err := h.devices.Get(r.Context(), user.ID, d)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					http.Error(w, "device not found", http.StatusNotFound)
					return
				}
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			deviceID = &dev.ID
		}

		result, err := h.episodes.GetActions(r.Context(), user.ID, since, podcastURL, deviceID, now)
		if err != nil {
			h.logger.Error("get episodes", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, result)

	case http.MethodPost:
		var actions []service.EpisodeActionInput
		if err := readJSON(r, h.maxReqSize, &actions); err != nil {
			http.Error(w, "Could not decode episode update POST data", http.StatusBadRequest)
			return
		}

		result, err := h.episodes.UploadActions(r.Context(), user.ID, actions, now)
		if err != nil {
			h.logger.Error("upload episodes", "error", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// --- Devices ---

func (h *Handlers) DeviceUpdate(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	deviceUID := chi.URLParam(r, "device_uid")

	switch r.Method {
	case http.MethodGet:
		dev, err := h.devices.Get(r.Context(), user.ID, deviceUID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "device not found", http.StatusNotFound)
				return
			}
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, service.DeviceData{
			ID:      dev.UID,
			Caption: dev.Caption,
			Type:    string(dev.Type),
		})

	case http.MethodPost, http.MethodPut:
		var body struct {
			Caption *string `json:"caption"`
			Type    *string `json:"type"`
		}
		if err := readJSON(r, h.maxReqSize, &body); err != nil {
			http.Error(w, "Could not decode device update POST data", http.StatusBadRequest)
			return
		}

		if body.Caption != nil && *body.Caption == "" {
			http.Error(w, "caption must not be empty", http.StatusBadRequest)
			return
		}
		if body.Type != nil && !domain.ValidDeviceType(*body.Type) {
			http.Error(w, "invalid device type "+*body.Type, http.StatusBadRequest)
			return
		}

		if err := h.devices.Update(r.Context(), user.ID, deviceUID, body.Caption, body.Type); err != nil {
			h.logger.Error("update device", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func (h *Handlers) DeviceList(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	devices, err := h.devices.List(r.Context(), user.ID)
	if err != nil {
		h.logger.Error("list devices", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, devices)
}

// --- Sync ---

func (h *Handlers) SyncDevices(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	switch r.Method {
	case http.MethodGet:
		status, err := h.sync.GetStatus(r.Context(), user.ID)
		if err != nil {
			h.logger.Error("get sync status", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, status)

	case http.MethodPost:
		var body struct {
			Synchronize     [][]string `json:"synchronize"`
			StopSynchronize []string   `json:"stop-synchronize"`
		}
		if err := readJSON(r, h.maxReqSize, &body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		status, err := h.sync.UpdateStatus(r.Context(), user.ID, body.Synchronize, body.StopSynchronize)
		if err != nil {
			h.logger.Error("update sync status", "error", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, status)
	}
}

// --- Settings ---

func (h *Handlers) Settings(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	scope := chi.URLParam(r, "scope")

	scopeType, scopeID, err := service.ParseScope(
		scope,
		r.URL.Query().Get("device"),
		r.URL.Query().Get("podcast"),
		r.URL.Query().Get("episode"),
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		settings, err := h.settings.Get(r.Context(), user.ID, scopeType, scopeID)
		if err != nil {
			h.logger.Error("get settings", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, settings)

	case http.MethodPost:
		var body service.SettingsUpdateRequest
		if err := readJSON(r, h.maxReqSize, &body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		result, err := h.settings.Update(r.Context(), user.ID, scopeType, scopeID, &body)
		if err != nil {
			h.logger.Error("update settings", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// --- Updates ---

func (h *Handlers) Updates(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	deviceUID := chi.URLParam(r, "device_uid")

	dev, err := h.devices.Get(r.Context(), user.ID, deviceUID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "device not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	sinceStr := r.URL.Query().Get("since")
	if sinceStr == "" {
		http.Error(w, "parameter since missing", http.StatusBadRequest)
		return
	}

	sinceVal, err := strconv.ParseFloat(sinceStr, 64)
	if err != nil {
		http.Error(w, "'since' is not a valid timestamp", http.StatusBadRequest)
		return
	}
	since := time.Unix(int64(sinceVal), 0).UTC()
	now := time.Now().UTC()

	result, err := h.updates.GetUpdates(r.Context(), user.ID, dev.ID, since, now)
	if err != nil {
		h.logger.Error("get updates", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
