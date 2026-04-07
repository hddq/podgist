package migrations_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/hddq/podgist/internal/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestUpIsIdempotent(t *testing.T) {
	ctx := t.Context()

	pgContainer, err := postgres.Run(ctx, "postgres:18-alpine",
		postgres.WithDatabase("podgist_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("failed to start postgres: %v", err)
	}
	t.Cleanup(func() { pgContainer.Terminate(context.Background()) })

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	migrationsDir, err := filepath.Abs("../../migrations")
	if err != nil {
		t.Fatalf("failed to resolve migrations path: %v", err)
	}

	if err := migrations.Up(ctx, connStr, migrationsDir); err != nil {
		t.Fatalf("first migration run failed: %v", err)
	}

	firstVersion, firstAppliedCount := currentGooseVersionState(t, ctx, connStr)

	if err := migrations.Up(ctx, connStr, migrationsDir); err != nil {
		t.Fatalf("second migration run failed: %v", err)
	}

	secondVersion, secondAppliedCount := currentGooseVersionState(t, ctx, connStr)

	if firstVersion == 0 {
		t.Fatal("expected at least one applied migration version")
	}
	if secondVersion != firstVersion {
		t.Fatalf("expected current goose version to remain %d, got %d", firstVersion, secondVersion)
	}
	if secondAppliedCount != firstAppliedCount {
		t.Fatalf("expected applied migration row count to remain %d, got %d", firstAppliedCount, secondAppliedCount)
	}
}

func currentGooseVersionState(t *testing.T, ctx context.Context, connStr string) (int64, int) {
	t.Helper()

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	var version int64
	var appliedCount int
	if err := pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(version_id), 0), COUNT(*)
		FROM goose_db_version
		WHERE is_applied = true
	`).Scan(&version, &appliedCount); err != nil {
		t.Fatalf("failed to query goose version state: %v", err)
	}

	return version, appliedCount
}
