package store_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/hddq/podgist/internal/domain"
	"github.com/hddq/podgist/internal/migrations"
	"github.com/hddq/podgist/internal/store"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func runStoreTests(t *testing.T, st store.Store) {
	ctx := t.Context()

	// 1. User & Auth tests
	user, err := st.CreateUser(ctx, "testuser", "hashed_password")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	fetchedUser, err := st.GetUserByUsername(ctx, "testuser")
	if err != nil {
		t.Fatalf("GetUserByUsername failed: %v", err)
	}
	if fetchedUser.ID != user.ID {
		t.Fatalf("expected user ID %d, got %d", user.ID, fetchedUser.ID)
	}

	// 2. Session tests
	sess, err := st.CreateSession(ctx, "sess_123", user.ID, time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if sess.ID != "sess_123" {
		t.Fatalf("expected session ID sess_123, got %s", sess.ID)
	}

	sessUser, err := st.GetUserBySessionID(ctx, "sess_123", time.Now().UTC())
	if err != nil {
		t.Fatalf("GetUserBySessionID failed: %v", err)
	}
	if sessUser.ID != user.ID {
		t.Fatalf("expected session user ID %d, got %d", user.ID, sessUser.ID)
	}

	// 3. Device tests
	dev, err := st.GetOrCreateDevice(ctx, user.ID, "dev_uid_1")
	if err != nil {
		t.Fatalf("GetOrCreateDevice failed: %v", err)
	}
	if dev.UID != "dev_uid_1" {
		t.Fatalf("expected device UID dev_uid_1, got %s", dev.UID)
	}

	// 4. Subscriptions tests
	err = st.AddSubscription(ctx, user.ID, dev.ID, "https://example.com/feed.xml", time.Now().UTC())
	if err != nil {
		t.Fatalf("AddSubscription failed: %v", err)
	}

	subs, err := st.GetCurrentSubscriptions(ctx, user.ID, dev.ID)
	if err != nil {
		t.Fatalf("GetCurrentSubscriptions failed: %v", err)
	}
	if len(subs) != 1 || subs[0] != "https://example.com/feed.xml" {
		t.Fatalf("expected [https://example.com/feed.xml], got %v", subs)
	}

	// 5. Dashboard Summary
	summary, err := st.GetDashboardSummary(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetDashboardSummary failed: %v", err)
	}
	if summary.SubscriptionCount != 1 || summary.DeviceCount != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}

	// 6. Sync Group & Aggregated Subscriptions ordering
	dev2, err := st.GetOrCreateDevice(ctx, user.ID, "dev_uid_2")
	if err != nil {
		t.Fatalf("GetOrCreateDevice dev2 failed: %v", err)
	}
	if err := st.SetSyncGroup(ctx, user.ID, []string{dev2.UID, dev.UID}); err != nil {
		t.Fatalf("SetSyncGroup failed: %v", err)
	}
	aggSubs, err := st.GetAggregatedSubscriptions(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetAggregatedSubscriptions failed: %v", err)
	}
	if len(aggSubs) != 1 || len(aggSubs[0].Devices) != 2 || aggSubs[0].Devices[0] != "dev_uid_1" || aggSubs[0].Devices[1] != "dev_uid_2" {
		t.Fatalf("unexpected aggregated subscriptions: %+v", aggSubs)
	}

	// 7. Episode Actions (including unlimited limit=0)
	now := time.Now().UTC()
	if err := st.AddEpisodeAction(ctx, &domain.EpisodeAction{
		UserID:     user.ID,
		DeviceID:   &dev.ID,
		PodcastURL: "https://example.com/feed.xml",
		EpisodeURL: "https://example.com/ep1.mp3",
		Action:     domain.ActionPlay,
		Timestamp:  now,
		CreatedAt:  now,
	}); err != nil {
		t.Fatalf("AddEpisodeAction failed: %v", err)
	}
	actions, err := st.GetEpisodeActionsSince(ctx, user.ID, nil, nil, nil, 0)
	if err != nil {
		t.Fatalf("GetEpisodeActionsSince (limit=0) failed: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("expected 1 episode action, got %d", len(actions))
	}

	// 8. Podcast Metadata with nil LastFetchedAt & nil PublishedAt
	podcast := &domain.Podcast{
		URL:           "https://example.com/feed.xml",
		Title:         "Example Podcast",
		LastFetchedAt: nil,
	}
	if err := st.UpsertPodcastWithEpisodes(ctx, podcast, []domain.PodcastEpisodeMetadata{
		{
			EpisodeURL:  "https://example.com/ep1.mp3",
			Title:       "Episode 1",
			PublishedAt: nil,
		},
	}); err != nil {
		t.Fatalf("UpsertPodcastWithEpisodes failed: %v", err)
	}
	fetchedPodcast, err := st.GetPodcastByURL(ctx, "https://example.com/feed.xml")
	if err != nil {
		t.Fatalf("GetPodcastByURL failed: %v", err)
	}
	if fetchedPodcast.LastFetchedAt != nil {
		t.Fatalf("expected nil LastFetchedAt, got %v", fetchedPodcast.LastFetchedAt)
	}
}

func TestStoreSQLite(t *testing.T) {
	ctx := t.Context()
	dbPath := filepath.Join(t.TempDir(), "store_test.db")

	migrationsDir, err := filepath.Abs("../../migrations/sqlite")
	if err != nil {
		t.Fatalf("failed to resolve migrations dir: %v", err)
	}

	if err := migrations.Up(ctx, "sqlite", dbPath, migrationsDir); err != nil {
		t.Fatalf("failed to run sqlite migrations: %v", err)
	}

	st, err := store.New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to initialize sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	runStoreTests(t, st)
}

func TestStorePostgres(t *testing.T) {
	ctx := t.Context()

	pgContainer, err := postgres.Run(ctx, "postgres:18-alpine",
		postgres.WithDatabase("podgist_store_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Skipf("skipping postgres store test: %v", err)
	}
	t.Cleanup(func() { pgContainer.Terminate(context.Background()) })

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	migrationsDir, err := filepath.Abs("../../migrations/postgres")
	if err != nil {
		t.Fatalf("failed to resolve migrations dir: %v", err)
	}

	if err := migrations.Up(ctx, "postgres", connStr, migrationsDir); err != nil {
		t.Fatalf("failed to run postgres migrations: %v", err)
	}

	st, err := store.New("postgres", connStr)
	if err != nil {
		t.Fatalf("failed to initialize postgres store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	runStoreTests(t, st)
}
