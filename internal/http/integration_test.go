package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/hddq/podgist/internal/domain"
	apphttp "github.com/hddq/podgist/internal/http"
	"github.com/hddq/podgist/internal/migrations"
	"github.com/hddq/podgist/internal/service"
	"github.com/hddq/podgist/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

type testEnv struct {
	server *httptest.Server
	pool   *pgxpool.Pool
	auth   *service.AuthService
}

var (
	httpTestEnv     *testEnv
	httpTestEnvErr  error
	httpTestEnvOnce sync.Once
	pgContainer     *postgres.PostgresContainer
)

func TestMain(m *testing.M) {
	exitCode := m.Run()

	if httpTestEnv != nil {
		httpTestEnv.server.Close()
		httpTestEnv.pool.Close()
	}
	if pgContainer != nil {
		_ = pgContainer.Terminate(context.Background())
	}

	os.Exit(exitCode)
}

func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()

	httpTestEnvOnce.Do(func() {
		ctx := context.Background()

		pgContainer, httpTestEnvErr = postgres.Run(ctx, "postgres:18-alpine",
			postgres.WithDatabase("podgist_test"),
			postgres.WithUsername("test"),
			postgres.WithPassword("test"),
			testcontainers.WithWaitStrategy(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).
					WithStartupTimeout(30*time.Second),
			),
		)
		if httpTestEnvErr != nil {
			return
		}

		connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			httpTestEnvErr = err
			return
		}

		pool, err := pgxpool.New(ctx, connStr)
		if err != nil {
			httpTestEnvErr = err
			return
		}

		migrationsDir, err := filepath.Abs("../../migrations")
		if err != nil {
			pool.Close()
			httpTestEnvErr = err
			return
		}
		if err := migrations.Up(ctx, connStr, migrationsDir); err != nil {
			pool.Close()
			httpTestEnvErr = err
			return
		}

		st := store.New(pool)
		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

		authSvc := service.NewAuthService(st, 4)
		metadataSvc := service.NewPodcastMetadataServiceWithClient(st, logger, &http.Client{Timeout: time.Second}, time.Second, 2*time.Hour)
		subsSvc := service.NewSubscriptionService(st, metadataSvc)
		epsSvc := service.NewEpisodeService(st, 500, metadataSvc)
		devsSvc := service.NewDeviceService(st)
		syncSvc := service.NewSyncService(st)
		settingsSvc := service.NewSettingsService(st)
		updatesSvc := service.NewUpdatesService(st)

		handlers := apphttp.NewHandlers(authSvc, subsSvc, epsSvc, devsSvc, syncSvc, settingsSvc, updatesSvc, 5*1024*1024, logger)
		dashHandlers := apphttp.NewDashboardHandlers(authSvc, st, syncSvc, logger)
		router := apphttp.NewRouter(authSvc, handlers, dashHandlers, "test", logger, fs.FS(nil))

		httpTestEnv = &testEnv{
			server: httptest.NewServer(router),
			pool:   pool,
			auth:   authSvc,
		}
	})

	if httpTestEnvErr != nil {
		t.Fatalf("failed to initialize shared test env: %v", httpTestEnvErr)
	}

	resetTestData(t, httpTestEnv)
	return httpTestEnv
}

func resetTestData(t *testing.T, env *testEnv) {
	t.Helper()

	ctx := t.Context()
	if _, err := env.pool.Exec(ctx, `
		TRUNCATE TABLE
			settings,
			podcast_episodes,
			podcasts,
			sync_group_members,
			sync_groups,
			episode_actions,
			subscription_events,
			subscriptions,
			devices,
			sessions,
			users
		RESTART IDENTITY CASCADE
	`); err != nil {
		t.Fatalf("failed to reset test data: %v", err)
	}

	if _, err := env.auth.CreateUser(ctx, "testuser", "testpass"); err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
}

func (e *testEnv) doRequest(t *testing.T, method, path string, body any, auth bool) *http.Response {
	t.Helper()
	return e.doRequestWithClient(t, http.DefaultClient, method, path, body, auth)
}

func (e *testEnv) doRequestWithClient(t *testing.T, client *http.Client, method, path string, body any, auth bool) *http.Response {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("failed to marshal request body: %v", err)
		}
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, e.server.URL+path, bodyReader)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if auth {
		req.SetBasicAuth("testuser", "testpass")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}

func readBody(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	return result
}

func asStringSlice(t *testing.T, raw any) []string {
	t.Helper()

	values, ok := raw.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", raw)
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		str, ok := value.(string)
		if !ok {
			t.Fatalf("expected string, got %T", value)
		}
		out = append(out, str)
	}
	return out
}

func asStringGroups(t *testing.T, raw any) [][]string {
	t.Helper()

	values, ok := raw.([]any)
	if !ok {
		t.Fatalf("expected []any for groups, got %T", raw)
	}
	out := make([][]string, 0, len(values))
	for _, value := range values {
		out = append(out, asStringSlice(t, value))
	}
	return out
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func eventually(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

func podcastMetadataRow(t *testing.T, env *testEnv, podcastURL string) (title, etag, lastModified string, fetchedAt *time.Time, ok bool) {
	t.Helper()

	var ts *time.Time
	err := env.pool.QueryRow(t.Context(), `
		SELECT title, etag, last_modified, last_fetched_at
		FROM podcasts
		WHERE url = $1
	`, podcastURL).Scan(&title, &etag, &lastModified, &ts)
	if err != nil {
		return "", "", "", nil, false
	}
	return title, etag, lastModified, ts, true
}

func podcastEpisodeCount(t *testing.T, env *testEnv, podcastURL string) int {
	t.Helper()

	var count int
	if err := env.pool.QueryRow(t.Context(), `
		SELECT COUNT(*)
		FROM podcast_episodes pe
		JOIN podcasts p ON p.id = pe.podcast_id
		WHERE p.url = $1
	`, podcastURL).Scan(&count); err != nil {
		t.Fatalf("failed to count podcast episodes: %v", err)
	}
	return count
}

func podcastEpisodeTitle(t *testing.T, env *testEnv, podcastURL, episodeURL string) string {
	t.Helper()

	var title string
	if err := env.pool.QueryRow(t.Context(), `
		SELECT pe.title
		FROM podcast_episodes pe
		JOIN podcasts p ON p.id = pe.podcast_id
		WHERE p.url = $1 AND pe.episode_url = $2
	`, podcastURL, episodeURL).Scan(&title); err != nil {
		t.Fatalf("failed to load episode title: %v", err)
	}
	return title
}

func sessionIDCookie(t *testing.T, resp *http.Response) *http.Cookie {
	t.Helper()
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "sessionid" {
			return cookie
		}
	}
	t.Fatal("expected sessionid cookie")
	return nil
}

func dashboardLogin(t *testing.T, env *testEnv) *http.Client {
	t.Helper()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}

	resp := env.doRequestWithClient(t, client, http.MethodPost, "/api/podgist/v1/login", map[string]string{
		"username": "testuser",
		"password": "testpass",
	}, false)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected dashboard login 200, got %d", resp.StatusCode)
	}
	_ = sessionIDCookie(t, resp)

	return client
}

func testUserID(t *testing.T, env *testEnv) int64 {
	t.Helper()

	var userID int64
	if err := env.pool.QueryRow(t.Context(), `SELECT id FROM users WHERE username = 'testuser'`).Scan(&userID); err != nil {
		t.Fatalf("failed to look up test user: %v", err)
	}
	return userID
}

func createDevice(t *testing.T, env *testEnv, uid string) int64 {
	t.Helper()

	userID := testUserID(t, env)
	var deviceID int64
	if err := env.pool.QueryRow(t.Context(), `
		INSERT INTO devices (user_id, uid, caption, type)
		VALUES ($1, $2, '', 'other')
		RETURNING id
	`, userID, uid).Scan(&deviceID); err != nil {
		t.Fatalf("failed to create device %q: %v", uid, err)
	}
	return deviceID
}

func seedEpisodeAction(t *testing.T, env *testEnv, action domain.EpisodeAction) {
	t.Helper()

	if err := store.New(env.pool).AddEpisodeAction(t.Context(), &action); err != nil {
		t.Fatalf("failed to seed episode action: %v", err)
	}
}

func seedPodcastMetadata(t *testing.T, env *testEnv, podcast domain.Podcast, episodes []domain.PodcastEpisodeMetadata) {
	t.Helper()

	if err := store.New(env.pool).UpsertPodcastWithEpisodes(t.Context(), &podcast, episodes); err != nil {
		t.Fatalf("failed to seed podcast metadata: %v", err)
	}
}

// --- Auth Tests ---

func TestLoginSuccess(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doRequest(t, "POST", "/api/2/auth/testuser/login.json", nil, true)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	cookie := sessionIDCookie(t, resp)
	if cookie.Value == "" {
		t.Fatal("expected non-empty sessionid cookie")
	}
}

func TestLoginNoAuth(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doRequest(t, "POST", "/api/2/auth/testuser/login.json", nil, false)
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
	if resp.Header.Get("WWW-Authenticate") == "" {
		t.Error("expected WWW-Authenticate header")
	}
}

func TestLoginWrongPassword(t *testing.T) {
	env := setupTestEnv(t)
	req, err := http.NewRequest("POST", env.server.URL+"/api/2/auth/testuser/login.json", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetBasicAuth("testuser", "wrongpass")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestLoginUsernameMismatch(t *testing.T) {
	env := setupTestEnv(t)
	req, err := http.NewRequest("POST", env.server.URL+"/api/2/auth/otheruser/login.json", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetBasicAuth("testuser", "testpass")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestLogout(t *testing.T) {
	env := setupTestEnv(t)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}

	loginResp := env.doRequestWithClient(t, client, "POST", "/api/2/auth/testuser/login.json", nil, true)
	sessionCookie := *sessionIDCookie(t, loginResp)
	loginResp.Body.Close()

	resp := env.doRequestWithClient(t, client, "POST", "/api/2/auth/testuser/logout.json", nil, false)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	cleared := sessionIDCookie(t, resp)
	if cleared.MaxAge >= 0 {
		t.Error("expected cleared sessionid cookie")
	}

	req, err := http.NewRequest("GET", env.server.URL+"/api/2/devices/testuser.json", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&sessionCookie)
	postLogoutResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer postLogoutResp.Body.Close()
	if postLogoutResp.StatusCode != 401 {
		t.Fatalf("expected old session to be rejected with 401, got %d", postLogoutResp.StatusCode)
	}
}

func TestSessionCookieAllowsProtectedRequests(t *testing.T) {
	env := setupTestEnv(t)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}

	loginResp := env.doRequestWithClient(t, client, "POST", "/api/2/auth/testuser/login.json", nil, true)
	loginResp.Body.Close()

	body := map[string]any{
		"add":    []string{"https://example.com/feed1.xml"},
		"remove": []string{},
	}
	resp := env.doRequestWithClient(t, client, "POST", "/api/2/subscriptions/testuser/dev1.json", body, false)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestLoginSessionUsernameMismatch(t *testing.T) {
	env := setupTestEnv(t)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}

	loginResp := env.doRequestWithClient(t, client, "POST", "/api/2/auth/testuser/login.json", nil, true)
	loginResp.Body.Close()

	req, err := http.NewRequest("POST", env.server.URL+"/api/2/auth/otheruser/login.json", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetBasicAuth("testuser", "testpass")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestLogoutSessionUsernameMismatch(t *testing.T) {
	env := setupTestEnv(t)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}

	loginResp := env.doRequestWithClient(t, client, "POST", "/api/2/auth/testuser/login.json", nil, true)
	loginResp.Body.Close()

	resp := env.doRequestWithClient(t, client, "POST", "/api/2/auth/otheruser/logout.json", nil, false)
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

// --- Subscription Tests ---

func TestSubscriptionAddAndGet(t *testing.T) {
	env := setupTestEnv(t)

	body := map[string]any{
		"add":    []string{"https://example.com/feed1.xml", "https://example.com/feed2.xml"},
		"remove": []string{},
	}
	resp := env.doRequest(t, "POST", "/api/2/subscriptions/testuser/dev1.json", body, true)
	result := readBody(t, resp)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if result["timestamp"] == nil {
		t.Error("expected timestamp")
	}
	if result["update_urls"] == nil {
		t.Error("expected update_urls")
	}

	resp = env.doRequest(t, "GET", "/api/2/subscriptions/testuser/dev1.json", nil, true)
	result = readBody(t, resp)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	add := result["add"].([]any)
	if len(add) != 2 {
		t.Errorf("expected 2 subscriptions, got %d", len(add))
	}
}

func TestSubscriptionSinceDiff(t *testing.T) {
	env := setupTestEnv(t)

	body := map[string]any{
		"add":    []string{"https://example.com/feed1.xml"},
		"remove": []string{},
	}
	resp := env.doRequest(t, "POST", "/api/2/subscriptions/testuser/dev1.json", body, true)
	result := readBody(t, resp)
	ts := int64(result["timestamp"].(float64))

	time.Sleep(time.Second)
	body2 := map[string]any{
		"add":    []string{"https://example.com/feed2.xml"},
		"remove": []string{"https://example.com/feed1.xml"},
	}
	resp = env.doRequest(t, "POST", "/api/2/subscriptions/testuser/dev1.json", body2, true)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = env.doRequest(t, "GET", "/api/2/subscriptions/testuser/dev1.json?since="+itoa(ts+1), nil, true)
	result = readBody(t, resp)
	add := result["add"].([]any)
	remove := result["remove"].([]any)
	if len(add) != 1 || add[0] != "https://example.com/feed2.xml" {
		t.Errorf("unexpected add: %v", add)
	}
	if len(remove) != 1 || remove[0] != "https://example.com/feed1.xml" {
		t.Errorf("unexpected remove: %v", remove)
	}
}

// --- Episode Tests ---

func TestEpisodeUploadAndGet(t *testing.T) {
	env := setupTestEnv(t)

	actions := []map[string]any{
		{
			"podcast":   "https://example.com/feed.xml",
			"episode":   "https://example.com/ep1.mp3",
			"action":    "play",
			"timestamp": "2024-01-01T12:00:00",
			"started":   0,
			"position":  120,
			"total":     3600,
		},
		{
			"podcast": "https://example.com/feed.xml",
			"episode": "https://example.com/ep2.mp3",
			"action":  "download",
		},
	}

	resp := env.doRequest(t, "POST", "/api/2/episodes/testuser.json", actions, true)
	result := readBody(t, resp)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if result["timestamp"] == nil {
		t.Error("expected timestamp")
	}

	resp = env.doRequest(t, "GET", "/api/2/episodes/testuser.json", nil, true)
	result = readBody(t, resp)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	actionsOut := result["actions"].([]any)
	if len(actionsOut) != 2 {
		t.Errorf("expected 2 actions, got %d", len(actionsOut))
	}
}

func TestEpisodeInvalidAction(t *testing.T) {
	env := setupTestEnv(t)

	actions := []map[string]any{
		{
			"podcast": "https://example.com/feed.xml",
			"episode": "https://example.com/ep1.mp3",
			"action":  "invalid",
		},
	}

	resp := env.doRequest(t, "POST", "/api/2/episodes/testuser.json", actions, true)
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// --- Device Tests ---

func TestDeviceListAndUpdate(t *testing.T) {
	env := setupTestEnv(t)

	body := map[string]any{
		"caption": "My Phone",
		"type":    "mobile",
	}
	resp := env.doRequest(t, "POST", "/api/2/devices/testuser/phone1.json", body, true)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	resp = env.doRequest(t, "GET", "/api/2/devices/testuser.json", nil, true)
	var devices []map[string]any
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&devices); err != nil {
		t.Fatalf("failed to decode devices response: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	if devices[0]["id"] != "phone1" {
		t.Errorf("expected id phone1, got %v", devices[0]["id"])
	}
	if devices[0]["caption"] != "My Phone" {
		t.Errorf("expected caption My Phone, got %v", devices[0]["caption"])
	}
}

func TestDeviceGetNotFound(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doRequest(t, "GET", "/api/2/devices/testuser/nonexistent.json", nil, true)
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestDeviceInvalidType(t *testing.T) {
	env := setupTestEnv(t)
	body := map[string]any{"type": "invalid"}
	resp := env.doRequest(t, "POST", "/api/2/devices/testuser/dev1.json", body, true)
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestDeviceEmptyCaption(t *testing.T) {
	env := setupTestEnv(t)
	body := map[string]any{"caption": ""}
	resp := env.doRequest(t, "POST", "/api/2/devices/testuser/dev1.json", body, true)
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// --- Sync Tests ---

func TestSyncDevices(t *testing.T) {
	env := setupTestEnv(t)

	for _, uid := range []string{"dev1", "dev2", "dev3"} {
		body := map[string]any{"caption": uid, "type": "other"}
		resp := env.doRequest(t, "POST", "/api/2/devices/testuser/"+uid+".json", body, true)
		resp.Body.Close()
	}

	resp := env.doRequest(t, "GET", "/api/2/sync-devices/testuser.json", nil, true)
	result := readBody(t, resp)
	unsynced := result["not-synchronized"].([]any)
	if len(unsynced) != 3 {
		t.Errorf("expected 3 unsynced, got %d", len(unsynced))
	}

	body := map[string]any{
		"synchronize":      [][]string{{"dev1", "dev2"}},
		"stop-synchronize": []string{},
	}
	resp = env.doRequest(t, "POST", "/api/2/sync-devices/testuser.json", body, true)
	result = readBody(t, resp)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	synced := result["synchronized"].([]any)
	if len(synced) != 1 {
		t.Errorf("expected 1 sync group, got %d", len(synced))
	}

	body = map[string]any{
		"synchronize":      [][]string{},
		"stop-synchronize": []string{"dev1", "dev2"},
	}
	resp = env.doRequest(t, "POST", "/api/2/sync-devices/testuser.json", body, true)
	result = readBody(t, resp)
	synced = result["synchronized"].([]any)
	if len(synced) != 0 {
		t.Errorf("expected 0 sync groups after stop, got %d", len(synced))
	}
}

func TestSyncTooFewDevices(t *testing.T) {
	env := setupTestEnv(t)
	body := map[string]any{
		"synchronize": [][]string{{"dev1"}},
	}
	resp := env.doRequest(t, "POST", "/api/2/sync-devices/testuser.json", body, true)
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestSyncGroupMergesAndPropagatesSubscriptions(t *testing.T) {
	env := setupTestEnv(t)

	for _, uid := range []string{"dev1", "dev2"} {
		resp := env.doRequest(t, "POST", "/api/2/devices/testuser/"+uid+".json", map[string]any{
			"caption": uid,
			"type":    "other",
		}, true)
		resp.Body.Close()
	}

	const (
		feedA = "https://example.com/feeds/a.xml"
		feedB = "https://example.com/feeds/b.xml"
		feedC = "https://example.com/feeds/c.xml"
	)

	resp := env.doRequest(t, "POST", "/api/2/subscriptions/testuser/dev1.json", map[string]any{
		"add":    []string{feedA},
		"remove": []string{},
	}, true)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	resp = env.doRequest(t, "POST", "/api/2/subscriptions/testuser/dev2.json", map[string]any{
		"add":    []string{feedB},
		"remove": []string{},
	}, true)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	resp = env.doRequest(t, "POST", "/api/2/sync-devices/testuser.json", map[string]any{
		"synchronize":      [][]string{{"dev1", "dev2"}},
		"stop-synchronize": []string{},
	}, true)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	resp = env.doRequest(t, "GET", "/api/2/subscriptions/testuser/dev1.json", nil, true)
	dev1Subs := asStringSlice(t, readBody(t, resp)["add"])
	if !containsString(dev1Subs, feedA) || !containsString(dev1Subs, feedB) {
		t.Fatalf("expected dev1 to contain merged subscriptions, got %v", dev1Subs)
	}

	resp = env.doRequest(t, "GET", "/api/2/subscriptions/testuser/dev2.json", nil, true)
	dev2Subs := asStringSlice(t, readBody(t, resp)["add"])
	if !containsString(dev2Subs, feedA) || !containsString(dev2Subs, feedB) {
		t.Fatalf("expected dev2 to contain merged subscriptions, got %v", dev2Subs)
	}

	resp = env.doRequest(t, "POST", "/api/2/subscriptions/testuser/dev1.json", map[string]any{
		"add":    []string{feedC},
		"remove": []string{},
	}, true)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	resp = env.doRequest(t, "GET", "/api/2/subscriptions/testuser/dev2.json", nil, true)
	dev2Subs = asStringSlice(t, readBody(t, resp)["add"])
	if !containsString(dev2Subs, feedC) {
		t.Fatalf("expected dev2 to receive propagated subscription, got %v", dev2Subs)
	}

	resp = env.doRequest(t, "POST", "/api/2/subscriptions/testuser/dev2.json", map[string]any{
		"add":    []string{},
		"remove": []string{feedA},
	}, true)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	resp = env.doRequest(t, "GET", "/api/2/subscriptions/testuser/dev1.json", nil, true)
	dev1Subs = asStringSlice(t, readBody(t, resp)["add"])
	if containsString(dev1Subs, feedA) {
		t.Fatalf("expected feedA to be removed from dev1 after propagated delete, got %v", dev1Subs)
	}
}

func TestDashboardSyncDevicesEndpoint(t *testing.T) {
	env := setupTestEnv(t)
	client := dashboardLogin(t, env)

	for _, uid := range []string{"dev1", "dev2"} {
		resp := env.doRequest(t, "POST", "/api/2/devices/testuser/"+uid+".json", map[string]any{
			"caption": uid,
			"type":    "other",
		}, true)
		resp.Body.Close()
	}

	resp := env.doRequestWithClient(t, client, http.MethodGet, "/api/podgist/v1/sync-devices", nil, false)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	initialStatus := readBody(t, resp)
	initialUnsynced := asStringSlice(t, initialStatus["not-synchronized"])
	if !containsString(initialUnsynced, "dev1") || !containsString(initialUnsynced, "dev2") {
		t.Fatalf("expected both devices unsynced initially, got %v", initialUnsynced)
	}

	resp = env.doRequestWithClient(t, client, http.MethodPost, "/api/podgist/v1/sync-devices", map[string]any{
		"synchronize":      [][]string{{"dev1", "dev2"}},
		"stop-synchronize": []string{},
	}, false)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	syncedStatus := readBody(t, resp)
	groups := asStringGroups(t, syncedStatus["synchronized"])
	if len(groups) != 1 || len(groups[0]) != 2 {
		t.Fatalf("expected one sync group with two devices, got %v", groups)
	}

	resp = env.doRequestWithClient(t, client, http.MethodPost, "/api/podgist/v1/sync-devices", map[string]any{
		"synchronize":      [][]string{},
		"stop-synchronize": []string{"dev1"},
	}, false)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	afterStopStatus := readBody(t, resp)
	if len(asStringGroups(t, afterStopStatus["synchronized"])) != 0 {
		t.Fatalf("expected no sync groups after stopping one device from a pair, got %v", afterStopStatus["synchronized"])
	}
	unsynced := asStringSlice(t, afterStopStatus["not-synchronized"])
	if !containsString(unsynced, "dev1") || !containsString(unsynced, "dev2") {
		t.Fatalf("expected both devices to be unsynced after stop, got %v", unsynced)
	}
}

// --- Settings Tests ---

func TestSettingsAccountScope(t *testing.T) {
	env := setupTestEnv(t)

	body := map[string]any{
		"set":    map[string]any{"theme": "dark", "lang": "en"},
		"remove": []string{},
	}
	resp := env.doRequest(t, "POST", "/api/2/settings/testuser/account.json", body, true)
	result := readBody(t, resp)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if result["theme"] != "dark" {
		t.Errorf("expected theme=dark, got %v", result["theme"])
	}

	resp = env.doRequest(t, "GET", "/api/2/settings/testuser/account.json", nil, true)
	result = readBody(t, resp)
	if result["theme"] != "dark" || result["lang"] != "en" {
		t.Errorf("unexpected settings: %v", result)
	}

	body = map[string]any{
		"set":    map[string]any{},
		"remove": []string{"lang"},
	}
	resp = env.doRequest(t, "POST", "/api/2/settings/testuser/account.json", body, true)
	result = readBody(t, resp)
	if _, ok := result["lang"]; ok {
		t.Error("expected lang to be removed")
	}
}

func TestSettingsDeviceScope(t *testing.T) {
	env := setupTestEnv(t)

	devBody := map[string]any{"caption": "dev1", "type": "mobile"}
	resp := env.doRequest(t, "POST", "/api/2/devices/testuser/dev1.json", devBody, true)
	resp.Body.Close()

	body := map[string]any{
		"set": map[string]any{"auto_download": true},
	}
	resp = env.doRequest(t, "POST", "/api/2/settings/testuser/device.json?device=dev1", body, true)
	result := readBody(t, resp)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if result["auto_download"] != true {
		t.Errorf("expected auto_download=true, got %v", result["auto_download"])
	}
}

func TestSettingsInvalidScope(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doRequest(t, "GET", "/api/2/settings/testuser/invalid.json", nil, true)
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// --- Updates Tests ---

func TestUpdates(t *testing.T) {
	env := setupTestEnv(t)

	devBody := map[string]any{"caption": "dev1", "type": "mobile"}
	resp := env.doRequest(t, "POST", "/api/2/devices/testuser/dev1.json", devBody, true)
	resp.Body.Close()

	subBody := map[string]any{
		"add":    []string{"https://example.com/feed.xml"},
		"remove": []string{},
	}
	resp = env.doRequest(t, "POST", "/api/2/subscriptions/testuser/dev1.json", subBody, true)
	resp.Body.Close()

	resp = env.doRequest(t, "GET", "/api/2/updates/testuser/dev1.json?since=0", nil, true)
	result := readBody(t, resp)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if result["timestamp"] == nil {
		t.Error("expected timestamp")
	}
	add := result["add"].([]any)
	if len(add) != 1 {
		t.Errorf("expected 1 add, got %d", len(add))
	}
}

func TestUpdatesMissingSince(t *testing.T) {
	env := setupTestEnv(t)

	devBody := map[string]any{"caption": "dev1", "type": "mobile"}
	resp := env.doRequest(t, "POST", "/api/2/devices/testuser/dev1.json", devBody, true)
	resp.Body.Close()

	resp = env.doRequest(t, "GET", "/api/2/updates/testuser/dev1.json", nil, true)
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestUpdatesDeviceNotFound(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doRequest(t, "GET", "/api/2/updates/testuser/nonexistent.json?since=0", nil, true)
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

// --- Error Cases ---

func TestMalformedJSON(t *testing.T) {
	env := setupTestEnv(t)
	req, err := http.NewRequest("POST", env.server.URL+"/api/2/subscriptions/testuser/dev1.json", bytes.NewReader([]byte("not json")))
	if err != nil {
		t.Fatal(err)
	}
	req.SetBasicAuth("testuser", "testpass")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestBadTimestamp(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doRequest(t, "GET", "/api/2/subscriptions/testuser/dev1.json?since=notanumber", nil, true)
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestProtectedEndpointNoAuth(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doRequest(t, "GET", "/api/2/devices/testuser.json", nil, false)
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAPIRouterDashboardRouteNotRegistered(t *testing.T) {
	st := store.New(nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	authSvc := service.NewAuthService(st, 4)
	metadataSvc := service.NewPodcastMetadataServiceWithClient(st, logger, &http.Client{Timeout: time.Second}, time.Second, 2*time.Hour)
	subsSvc := service.NewSubscriptionService(st, metadataSvc)
	epsSvc := service.NewEpisodeService(st, 500, metadataSvc)
	devsSvc := service.NewDeviceService(st)
	syncSvc := service.NewSyncService(st)
	settingsSvc := service.NewSettingsService(st)
	updatesSvc := service.NewUpdatesService(st)
	handlers := apphttp.NewHandlers(authSvc, subsSvc, epsSvc, devsSvc, syncSvc, settingsSvc, updatesSvc, 5*1024*1024, logger)
	router := apphttp.NewAPIRouter(authSvc, handlers, "test", logger)

	req := httptest.NewRequest(http.MethodGet, "/api/podgist/v1/dashboard", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestSubscriptionFetchesPodcastMetadataAsync(t *testing.T) {
	env := setupTestEnv(t)

	const episodeURL = "https://cdn.example.com/show/ep-1.mp3"

	feedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"feed-v1"`)
		w.Header().Set("Last-Modified", "Sat, 12 Apr 2026 10:00:00 GMT")
		w.Header().Set("Content-Type", "application/rss+xml")
		io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd">
  <channel>
    <title>Async Podcast</title>
    <description>Metadata test feed</description>
    <link>https://example.com/show</link>
    <itunes:author>Podgist</itunes:author>
    <itunes:image href="https://example.com/show.jpg"></itunes:image>
    <item>
      <title>Episode One</title>
      <description>First episode</description>
      <guid>ep-1</guid>
      <pubDate>Sat, 12 Apr 2026 09:00:00 +0000</pubDate>
      <itunes:duration>01:02:03</itunes:duration>
      <enclosure url="`+episodeURL+`" type="audio/mpeg" length="12345"></enclosure>
    </item>
  </channel>
</rss>`)
	}))
	defer feedServer.Close()

	resp := env.doRequest(t, http.MethodPost, "/api/2/subscriptions/testuser/dev1.json", map[string]any{
		"add":    []string{feedServer.URL},
		"remove": []string{},
	}, true)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	eventually(t, 3*time.Second, func() bool {
		title, _, _, fetchedAt, ok := podcastMetadataRow(t, env, feedServer.URL)
		return ok && title == "Async Podcast" && fetchedAt != nil && podcastEpisodeCount(t, env, feedServer.URL) == 1
	})

	title, etag, lastModified, fetchedAt, ok := podcastMetadataRow(t, env, feedServer.URL)
	if !ok {
		t.Fatal("expected podcast metadata row")
	}
	if title != "Async Podcast" {
		t.Fatalf("expected title Async Podcast, got %q", title)
	}
	if etag != `"feed-v1"` {
		t.Fatalf("expected etag %q, got %q", `"feed-v1"`, etag)
	}
	if lastModified != "Sat, 12 Apr 2026 10:00:00 GMT" {
		t.Fatalf("expected last modified to be stored, got %q", lastModified)
	}
	if fetchedAt == nil {
		t.Fatal("expected last_fetched_at to be set")
	}
	if got := podcastEpisodeTitle(t, env, feedServer.URL, episodeURL); got != "Episode One" {
		t.Fatalf("expected episode title Episode One, got %q", got)
	}
}

func TestEpisodeMetadataRefreshHonorsCooldownAndUsesConditionalGet(t *testing.T) {
	env := setupTestEnv(t)

	var (
		mu          sync.Mutex
		requests    int
		feedVersion = 1
	)

	feedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		version := feedVersion
		mu.Unlock()

		if version == 2 && r.Header.Get("If-None-Match") == `"feed-v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		w.Header().Set("ETag", fmt.Sprintf(`"feed-v%d"`, version))
		w.Header().Set("Last-Modified", "Sat, 12 Apr 2026 10:00:00 GMT")
		w.Header().Set("Content-Type", "application/rss+xml")

		extraEpisode := ""
		if version >= 3 {
			extraEpisode = `
    <item>
      <title>Episode Two</title>
      <description>Second episode</description>
      <guid>ep-2</guid>
      <pubDate>Sat, 12 Apr 2026 11:00:00 +0000</pubDate>
      <enclosure url="https://cdn.example.com/show/ep-2.mp3" type="audio/mpeg" length="45678"></enclosure>
    </item>`
		}

		io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Refresh Podcast</title>
    <description>Conditional GET feed</description>
    <link>https://example.com/refresh</link>
    <item>
      <title>Episode One</title>
      <description>First episode</description>
      <guid>ep-1</guid>
      <pubDate>Sat, 12 Apr 2026 09:00:00 +0000</pubDate>
      <enclosure url="https://cdn.example.com/show/ep-1.mp3" type="audio/mpeg" length="12345"></enclosure>
    </item>`+extraEpisode+`
  </channel>
</rss>`)
	}))
	defer feedServer.Close()

	resp := env.doRequest(t, http.MethodPost, "/api/2/subscriptions/testuser/dev1.json", map[string]any{
		"add":    []string{feedServer.URL},
		"remove": []string{},
	}, true)
	resp.Body.Close()

	eventually(t, 3*time.Second, func() bool {
		return podcastEpisodeCount(t, env, feedServer.URL) == 1
	})

	mu.Lock()
	if requests != 1 {
		t.Fatalf("expected initial fetch count 1, got %d", requests)
	}
	mu.Unlock()

	resp = env.doRequest(t, http.MethodPost, "/api/2/episodes/testuser.json", []map[string]any{{
		"podcast":   feedServer.URL,
		"episode":   "https://cdn.example.com/show/ep-2.mp3",
		"action":    "play",
		"timestamp": "2026-04-12T12:00:00Z",
	}}, true)
	resp.Body.Close()

	time.Sleep(250 * time.Millisecond)

	mu.Lock()
	if requests != 1 {
		t.Fatalf("expected no fetch inside cooldown, got %d requests", requests)
	}
	mu.Unlock()

	title, etag, lastModified, before304, ok := podcastMetadataRow(t, env, feedServer.URL)
	if !ok || before304 == nil {
		t.Fatal("expected stored podcast metadata before conditional refresh")
	}
	if title != "Refresh Podcast" || etag != `"feed-v1"` || lastModified == "" {
		t.Fatal("expected initial metadata to be stored before refresh test")
	}

	if _, err := env.pool.Exec(t.Context(), `
		UPDATE podcasts
		SET last_fetched_at = $2
		WHERE url = $1
	`, feedServer.URL, time.Now().UTC().Add(-3*time.Hour)); err != nil {
		t.Fatalf("failed to age last_fetched_at: %v", err)
	}

	mu.Lock()
	feedVersion = 2
	mu.Unlock()

	resp = env.doRequest(t, http.MethodPost, "/api/2/episodes/testuser.json", []map[string]any{{
		"podcast":   feedServer.URL,
		"episode":   "https://cdn.example.com/show/ep-2.mp3",
		"action":    "play",
		"timestamp": "2026-04-12T12:05:00Z",
	}}, true)
	resp.Body.Close()

	eventually(t, 3*time.Second, func() bool {
		_, _, _, fetchedAt, ok := podcastMetadataRow(t, env, feedServer.URL)
		if !ok || fetchedAt == nil {
			return false
		}
		return fetchedAt.After(*before304)
	})

	mu.Lock()
	if requests != 2 {
		t.Fatalf("expected one conditional fetch, got %d requests", requests)
	}
	mu.Unlock()

	if podcastEpisodeCount(t, env, feedServer.URL) != 1 {
		t.Fatal("expected 304 refresh not to add new episodes")
	}

	if _, err := env.pool.Exec(t.Context(), `
		UPDATE podcasts
		SET last_fetched_at = $2
		WHERE url = $1
	`, feedServer.URL, time.Now().UTC().Add(-3*time.Hour)); err != nil {
		t.Fatalf("failed to age last_fetched_at for 200 refresh: %v", err)
	}

	mu.Lock()
	feedVersion = 3
	mu.Unlock()

	resp = env.doRequest(t, http.MethodPost, "/api/2/episodes/testuser.json", []map[string]any{{
		"podcast":   feedServer.URL,
		"episode":   "https://cdn.example.com/show/ep-2.mp3",
		"action":    "play",
		"timestamp": "2026-04-12T12:10:00Z",
	}}, true)
	resp.Body.Close()

	eventually(t, 3*time.Second, func() bool {
		return podcastEpisodeCount(t, env, feedServer.URL) == 2
	})

	mu.Lock()
	if requests != 3 {
		t.Fatalf("expected third request after cooldown expiry, got %d", requests)
	}
	mu.Unlock()

	if got := podcastEpisodeTitle(t, env, feedServer.URL, "https://cdn.example.com/show/ep-2.mp3"); got != "Episode Two" {
		t.Fatalf("expected refreshed episode title Episode Two, got %q", got)
	}
}

func TestEpisodeMetadataFetchUsesSingleflightPerPodcast(t *testing.T) {
	env := setupTestEnv(t)

	var (
		mu       sync.Mutex
		requests int
	)

	feedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()

		time.Sleep(150 * time.Millisecond)
		w.Header().Set("Content-Type", "application/rss+xml")
		io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Singleflight Podcast</title>
    <description>Burst test feed</description>
    <link>https://example.com/singleflight</link>
    <item>
      <title>Episode One</title>
      <guid>ep-1</guid>
      <enclosure url="https://cdn.example.com/show/ep-1.mp3" type="audio/mpeg" length="12345"></enclosure>
    </item>
  </channel>
</rss>`)
	}))
	defer feedServer.Close()

	actions := make([]map[string]any, 0, 10)
	for i := 0; i < 10; i++ {
		actions = append(actions, map[string]any{
			"podcast":   feedServer.URL,
			"episode":   fmt.Sprintf("https://cdn.example.com/show/missing-%d.mp3", i),
			"action":    "play",
			"timestamp": "2026-04-12T13:00:00Z",
		})
	}

	resp := env.doRequest(t, http.MethodPost, "/api/2/episodes/testuser.json", actions, true)
	resp.Body.Close()

	eventually(t, 3*time.Second, func() bool {
		title, _, _, fetchedAt, ok := podcastMetadataRow(t, env, feedServer.URL)
		return ok && title == "Singleflight Podcast" && fetchedAt != nil
	})

	mu.Lock()
	defer mu.Unlock()
	if requests != 1 {
		t.Fatalf("expected single feed request for burst upload, got %d", requests)
	}
}

func TestDashboardHistoryAggregatesPlaybackByEpisode(t *testing.T) {
	env := setupTestEnv(t)
	client := dashboardLogin(t, env)
	userID := testUserID(t, env)
	dev1ID := createDevice(t, env, "dev-1")
	dev2ID := createDevice(t, env, "dev-2")

	base := time.Date(2026, 4, 11, 10, 0, 0, 0, time.UTC)
	pos30 := 30
	pos120 := 120
	pos180 := 180
	total300 := 300

	seedEpisodeAction(t, env, domain.EpisodeAction{
		UserID:     userID,
		DeviceID:   &dev1ID,
		PodcastURL: "https://example.com/feed.xml",
		EpisodeURL: "https://example.com/episodes/1.mp3",
		Action:     domain.ActionPlay,
		Timestamp:  base,
		Position:   &pos30,
		Total:      &total300,
		CreatedAt:  base,
	})
	seedEpisodeAction(t, env, domain.EpisodeAction{
		UserID:     userID,
		DeviceID:   &dev1ID,
		PodcastURL: "https://example.com/feed.xml",
		EpisodeURL: "https://example.com/episodes/1.mp3",
		Action:     domain.ActionDownload,
		Timestamp:  base.Add(2 * time.Minute),
		Position:   &pos30,
		Total:      &total300,
		CreatedAt:  base.Add(2 * time.Minute),
	})
	seedEpisodeAction(t, env, domain.EpisodeAction{
		UserID:     userID,
		DeviceID:   &dev2ID,
		PodcastURL: "https://example.com/feed.xml",
		EpisodeURL: "https://example.com/episodes/1.mp3",
		Action:     domain.ActionPlay,
		Timestamp:  base.Add(4 * time.Minute),
		Position:   &pos180,
		Total:      &total300,
		CreatedAt:  base.Add(4 * time.Minute),
	})
	seedEpisodeAction(t, env, domain.EpisodeAction{
		UserID:     userID,
		DeviceID:   &dev1ID,
		PodcastURL: "https://example.com/feed.xml",
		EpisodeURL: "https://example.com/episodes/2.mp3",
		Action:     domain.ActionDownload,
		Timestamp:  base.Add(5 * time.Minute),
		Position:   &pos120,
		Total:      &total300,
		CreatedAt:  base.Add(5 * time.Minute),
	})
	seedEpisodeAction(t, env, domain.EpisodeAction{
		UserID:     userID,
		DeviceID:   &dev1ID,
		PodcastURL: "https://example.com/feed.xml",
		EpisodeURL: "https://example.com/episodes/3.mp3",
		Action:     domain.ActionPlay,
		Timestamp:  base.Add(3 * time.Minute),
		Position:   &pos120,
		Total:      &total300,
		CreatedAt:  base.Add(3 * time.Minute),
	})

	resp := env.doRequestWithClient(t, client, http.MethodGet, "/api/podgist/v1/history", nil, false)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var history []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&history); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(history))
	}

	if _, ok := history[0]["action"]; ok {
		t.Fatal("did not expect action field in playback history response")
	}

	if got := history[0]["episode_url"]; got != "https://example.com/episodes/1.mp3" {
		t.Fatalf("expected latest played episode first, got %v", got)
	}
	if got := history[0]["device_uid"]; got != "dev-2" {
		t.Fatalf("expected latest playback device dev-2, got %v", got)
	}
	if got := history[0]["position"]; got != float64(pos180) {
		t.Fatalf("expected latest playback position %d, got %v", pos180, got)
	}
	if got := history[0]["total"]; got != float64(total300) {
		t.Fatalf("expected total %d, got %v", total300, got)
	}
	gotTimestamp, ok := history[0]["timestamp"].(string)
	if !ok {
		t.Fatalf("expected timestamp string, got %T", history[0]["timestamp"])
	}
	parsedTimestamp, err := time.Parse(time.RFC3339, gotTimestamp)
	if err != nil {
		t.Fatalf("failed to parse timestamp %q: %v", gotTimestamp, err)
	}
	if !parsedTimestamp.Equal(base.Add(4 * time.Minute)) {
		t.Fatalf("expected latest playback timestamp %s, got %s", base.Add(4*time.Minute).Format(time.RFC3339), gotTimestamp)
	}

	if got := history[1]["episode_url"]; got != "https://example.com/episodes/3.mp3" {
		t.Fatalf("expected second distinct played episode, got %v", got)
	}
	if got := history[1]["device_uid"]; got != "dev-1" {
		t.Fatalf("expected device dev-1, got %v", got)
	}
}

func TestDashboardHistoryUsesDeterministicLatestPlayOrdering(t *testing.T) {
	env := setupTestEnv(t)
	client := dashboardLogin(t, env)
	userID := testUserID(t, env)
	devID := createDevice(t, env, "tie-device")

	ts := time.Date(2026, 4, 11, 12, 0, 0, 0, time.UTC)
	pos10 := 10
	pos90 := 90
	pos110 := 110
	total200 := 200

	seedEpisodeAction(t, env, domain.EpisodeAction{
		UserID:     userID,
		DeviceID:   &devID,
		PodcastURL: "https://example.com/feed.xml",
		EpisodeURL: "https://example.com/episodes/tie.mp3",
		Action:     domain.ActionPlay,
		Timestamp:  ts,
		Position:   &pos10,
		Total:      &total200,
		CreatedAt:  ts,
	})
	seedEpisodeAction(t, env, domain.EpisodeAction{
		UserID:     userID,
		DeviceID:   &devID,
		PodcastURL: "https://example.com/feed.xml",
		EpisodeURL: "https://example.com/episodes/tie.mp3",
		Action:     domain.ActionPlay,
		Timestamp:  ts,
		Position:   &pos90,
		Total:      &total200,
		CreatedAt:  ts.Add(time.Minute),
	})
	seedEpisodeAction(t, env, domain.EpisodeAction{
		UserID:     userID,
		DeviceID:   &devID,
		PodcastURL: "https://example.com/feed.xml",
		EpisodeURL: "https://example.com/episodes/tie.mp3",
		Action:     domain.ActionPlay,
		Timestamp:  ts,
		Position:   &pos110,
		Total:      &total200,
		CreatedAt:  ts.Add(time.Minute),
	})

	resp := env.doRequestWithClient(t, client, http.MethodGet, "/api/podgist/v1/history", nil, false)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var history []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&history); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}
	if got := history[0]["position"]; got != float64(pos110) {
		t.Fatalf("expected last inserted play to win tie, got %v", got)
	}
}

func TestDashboardEndpointsIncludeMetadataTitles(t *testing.T) {
	env := setupTestEnv(t)
	client := dashboardLogin(t, env)
	userID := testUserID(t, env)
	deviceID := createDevice(t, env, "named-device")

	podcastURL := "https://example.com/feed.xml"
	episodeURL := "https://example.com/episodes/1.mp3"
	now := time.Date(2026, 4, 12, 14, 0, 0, 0, time.UTC)
	pos180 := 180
	total300 := 300

	seedPodcastMetadata(t, env, domain.Podcast{
		URL:           podcastURL,
		Title:         "Named Podcast",
		LastFetchedAt: &now,
	}, []domain.PodcastEpisodeMetadata{{
		EpisodeURL: episodeURL,
		Title:      "Named Episode",
	}})

	if err := store.New(env.pool).AddSubscription(t.Context(), userID, deviceID, podcastURL, now); err != nil {
		t.Fatalf("failed to seed subscription: %v", err)
	}

	seedEpisodeAction(t, env, domain.EpisodeAction{
		UserID:     userID,
		DeviceID:   &deviceID,
		PodcastURL: podcastURL,
		EpisodeURL: episodeURL,
		Action:     domain.ActionPlay,
		Timestamp:  now,
		Position:   &pos180,
		Total:      &total300,
		CreatedAt:  now,
	})

	resp := env.doRequestWithClient(t, client, http.MethodGet, "/api/podgist/v1/dashboard", nil, false)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected dashboard 200, got %d", resp.StatusCode)
	}

	var dashboard map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&dashboard); err != nil {
		t.Fatalf("failed to decode dashboard response: %v", err)
	}

	recentActions, ok := dashboard["recent_actions"].([]any)
	if !ok || len(recentActions) != 1 {
		t.Fatalf("expected one recent action, got %#v", dashboard["recent_actions"])
	}
	action, ok := recentActions[0].(map[string]any)
	if !ok {
		t.Fatalf("expected recent action object, got %T", recentActions[0])
	}
	if got := action["podcast_title"]; got != "Named Podcast" {
		t.Fatalf("expected dashboard podcast_title, got %v", got)
	}
	if got := action["episode_title"]; got != "Named Episode" {
		t.Fatalf("expected dashboard episode_title, got %v", got)
	}

	resp = env.doRequestWithClient(t, client, http.MethodGet, "/api/podgist/v1/history", nil, false)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected history 200, got %d", resp.StatusCode)
	}

	var history []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&history); err != nil {
		t.Fatalf("failed to decode history response: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("expected one history entry, got %d", len(history))
	}
	if got := history[0]["podcast_title"]; got != "Named Podcast" {
		t.Fatalf("expected history podcast_title, got %v", got)
	}
	if got := history[0]["episode_title"]; got != "Named Episode" {
		t.Fatalf("expected history episode_title, got %v", got)
	}

	resp = env.doRequestWithClient(t, client, http.MethodGet, "/api/podgist/v1/subscriptions", nil, false)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected subscriptions 200, got %d", resp.StatusCode)
	}

	var subscriptions []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&subscriptions); err != nil {
		t.Fatalf("failed to decode subscriptions response: %v", err)
	}
	if len(subscriptions) != 1 {
		t.Fatalf("expected one subscription, got %d", len(subscriptions))
	}
	if got := subscriptions[0]["podcast_title"]; got != "Named Podcast" {
		t.Fatalf("expected subscriptions podcast_title, got %v", got)
	}
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}
