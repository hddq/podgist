package http_test

import (
	"bytes"
	"context"
	"encoding/json"
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
		subsSvc := service.NewSubscriptionService(st)
		epsSvc := service.NewEpisodeService(st, 500)
		devsSvc := service.NewDeviceService(st)
		syncSvc := service.NewSyncService(st)
		settingsSvc := service.NewSettingsService(st)
		updatesSvc := service.NewUpdatesService(st)

		handlers := apphttp.NewHandlers(authSvc, subsSvc, epsSvc, devsSvc, syncSvc, settingsSvc, updatesSvc, 5*1024*1024, logger)
		dashHandlers := apphttp.NewDashboardHandlers(authSvc, st, logger)
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
	subsSvc := service.NewSubscriptionService(st)
	epsSvc := service.NewEpisodeService(st, 500)
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

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}
