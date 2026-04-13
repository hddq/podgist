package http

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/hddq/podgist/internal/domain"
	"github.com/hddq/podgist/internal/service"
	"github.com/hddq/podgist/internal/store"
)

type DashboardHandlers struct {
	auth   *service.AuthService
	store  *store.Store
	sync   *service.SyncService
	logger *slog.Logger
}

func NewDashboardHandlers(auth *service.AuthService, st *store.Store, sync *service.SyncService, logger *slog.Logger) *DashboardHandlers {
	return &DashboardHandlers{auth: auth, store: st, sync: sync, logger: logger}
}

func (h *DashboardHandlers) Register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, 1<<20, &body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if body.Username == "" || body.Password == "" {
		http.Error(w, "username and password required", http.StatusBadRequest)
		return
	}

	user, err := h.auth.CreateUser(r.Context(), body.Username, body.Password)
	if err != nil {
		h.logger.Error("register user", "error", err)
		http.Error(w, "registration failed", http.StatusConflict)
		return
	}

	session, err := h.auth.CreateSession(r.Context(), user.ID)
	if err != nil {
		h.logger.Error("create session after register", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, sessionCookie(session))
	writeJSON(w, http.StatusCreated, map[string]any{
		"username":   user.Username,
		"created_at": user.CreatedAt.Format(time.RFC3339),
	})
}

func (h *DashboardHandlers) Login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, 1<<20, &body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	user, err := h.auth.Authenticate(r.Context(), body.Username, body.Password)
	if err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	session, err := h.auth.CreateSession(r.Context(), user.ID)
	if err != nil {
		h.logger.Error("create session", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, sessionCookie(session))
	writeJSON(w, http.StatusOK, map[string]any{
		"username":   user.Username,
		"created_at": user.CreatedAt.Format(time.RFC3339),
	})
}

func (h *DashboardHandlers) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		if err := h.auth.DeleteSession(r.Context(), cookie.Value); err != nil {
			h.logger.Error("delete session", "error", err)
		}
	}
	http.SetCookie(w, clearSessionCookie())
	w.WriteHeader(http.StatusNoContent)
}

func (h *DashboardHandlers) Me(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"username":   user.Username,
		"created_at": user.CreatedAt.Format(time.RFC3339),
	})
}

type episodeActionResponse struct {
	PodcastURL   string `json:"podcast_url"`
	PodcastTitle string `json:"podcast_title,omitempty"`
	EpisodeURL   string `json:"episode_url"`
	EpisodeTitle string `json:"episode_title,omitempty"`
	Action       string `json:"action"`
	Timestamp    string `json:"timestamp"`
	Started      *int   `json:"started,omitempty"`
	Position     *int   `json:"position,omitempty"`
	Total        *int   `json:"total,omitempty"`
	DeviceUID    string `json:"device_uid,omitempty"`
}

type playbackHistoryResponse struct {
	PodcastURL   string `json:"podcast_url"`
	PodcastTitle string `json:"podcast_title,omitempty"`
	EpisodeURL   string `json:"episode_url"`
	EpisodeTitle string `json:"episode_title,omitempty"`
	Timestamp    string `json:"timestamp"`
	Position     *int   `json:"position,omitempty"`
	Total        *int   `json:"total,omitempty"`
	DeviceUID    string `json:"device_uid,omitempty"`
}

func mapActions(actions []domain.EpisodeAction, devices map[int64]string) []episodeActionResponse {
	out := make([]episodeActionResponse, 0, len(actions))
	for _, a := range actions {
		resp := episodeActionResponse{
			PodcastURL:   a.PodcastURL,
			PodcastTitle: a.PodcastTitle,
			EpisodeURL:   a.EpisodeURL,
			EpisodeTitle: a.EpisodeTitle,
			Action:       string(a.Action),
			Timestamp:    a.Timestamp.Format(time.RFC3339),
			Started:      a.Started,
			Position:     a.Position,
			Total:        a.Total,
		}
		if a.DeviceID != nil {
			resp.DeviceUID = devices[*a.DeviceID]
		}
		out = append(out, resp)
	}
	return out
}

func mapPlaybackHistory(history []domain.PlaybackHistoryEntry, devices map[int64]string) []playbackHistoryResponse {
	out := make([]playbackHistoryResponse, 0, len(history))
	for _, entry := range history {
		resp := playbackHistoryResponse{
			PodcastURL:   entry.PodcastURL,
			PodcastTitle: entry.PodcastTitle,
			EpisodeURL:   entry.EpisodeURL,
			EpisodeTitle: entry.EpisodeTitle,
			Timestamp:    entry.Timestamp.Format(time.RFC3339),
			Position:     entry.Position,
			Total:        entry.Total,
		}
		if entry.DeviceID != nil {
			resp.DeviceUID = devices[*entry.DeviceID]
		}
		out = append(out, resp)
	}
	return out
}

func (h *DashboardHandlers) deviceUIDMap(ctx context.Context, userID int64) map[int64]string {
	devices, err := h.store.ListDevices(ctx, userID)
	if err != nil {
		return nil
	}
	m := make(map[int64]string, len(devices))
	for _, d := range devices {
		m[d.ID] = d.UID
	}
	return m
}

func (h *DashboardHandlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	summary, err := h.store.GetDashboardSummary(r.Context(), user.ID)
	if err != nil {
		h.logger.Error("dashboard summary", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	recent, err := h.store.GetRecentEpisodeActions(r.Context(), user.ID, 10)
	if err != nil {
		h.logger.Error("recent actions", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	devMap := h.deviceUIDMap(r.Context(), user.ID)

	writeJSON(w, http.StatusOK, map[string]any{
		"subscription_count":   summary.SubscriptionCount,
		"device_count":         summary.DeviceCount,
		"episode_action_count": summary.EpisodeActionCount,
		"recent_actions":       mapActions(recent, devMap),
	})
}

func (h *DashboardHandlers) History(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	history, err := h.store.GetPlaybackHistory(r.Context(), user.ID, 200)
	if err != nil {
		h.logger.Error("history", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	devMap := h.deviceUIDMap(r.Context(), user.ID)
	writeJSON(w, http.StatusOK, mapPlaybackHistory(history, devMap))
}

func (h *DashboardHandlers) Subscriptions(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	subs, err := h.store.GetAggregatedSubscriptions(r.Context(), user.ID)
	if err != nil {
		h.logger.Error("subscriptions", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if subs == nil {
		subs = []store.AggregatedSubscription{}
	}
	writeJSON(w, http.StatusOK, subs)
}

func (h *DashboardHandlers) Devices(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	devices, err := h.store.GetDevicesWithSubCount(r.Context(), user.ID)
	if err != nil {
		h.logger.Error("devices", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if devices == nil {
		devices = []store.DeviceWithSubCount{}
	}
	writeJSON(w, http.StatusOK, devices)
}

func (h *DashboardHandlers) SyncDevices(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	switch r.Method {
	case http.MethodGet:
		status, err := h.sync.GetStatus(r.Context(), user.ID)
		if err != nil {
			h.logger.Error("dashboard get sync status", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, status)

	case http.MethodPost:
		var body struct {
			Synchronize     [][]string `json:"synchronize"`
			StopSynchronize []string   `json:"stop-synchronize"`
		}
		if err := readJSON(r, 1<<20, &body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		status, err := h.sync.UpdateStatus(r.Context(), user.ID, body.Synchronize, body.StopSynchronize)
		if err != nil {
			h.logger.Error("dashboard update sync status", "error", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, status)
	}
}

func (h *DashboardHandlers) Account(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	session := SessionFromContext(r.Context())

	expiresAt := ""
	if session != nil {
		expiresAt = session.ExpiresAt.Format(time.RFC3339)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"username":           user.Username,
		"created_at":         user.CreatedAt.Format(time.RFC3339),
		"session_expires_at": expiresAt,
	})
}
