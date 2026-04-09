package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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
  router := apphttp.NewRouterLegacy(authSvc, handlers, "test", logger)

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

func TestLegacyRouterDashboardRouteNotRegistered(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doRequest(t, "GET", "/api/podgist/v1/dashboard", nil, false)
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}
