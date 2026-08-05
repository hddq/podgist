package migrations_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/hddq/podgist/internal/migrations"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	_ "modernc.org/sqlite"
)

func TestUpIsIdempotentPostgres(t *testing.T) {
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
		t.Skipf("skipping postgres test: %v", err)
	}
	t.Cleanup(func() { pgContainer.Terminate(context.Background()) })

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	migrationsDir, err := filepath.Abs("../../migrations/postgres")
	if err != nil {
		t.Fatalf("failed to resolve migrations path: %v", err)
	}

	if err := migrations.Up(ctx, "postgres", connStr, migrationsDir); err != nil {
		t.Fatalf("first migration run failed: %v", err)
	}

	firstVersion, firstAppliedCount := currentGooseVersionState(t, "pgx", connStr)

	if err := migrations.Up(ctx, "postgres", connStr, migrationsDir); err != nil {
		t.Fatalf("second migration run failed: %v", err)
	}

	secondVersion, secondAppliedCount := currentGooseVersionState(t, "pgx", connStr)

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

func TestUpIsIdempotentSQLite(t *testing.T) {
	ctx := t.Context()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	migrationsDir, err := filepath.Abs("../../migrations/sqlite")
	if err != nil {
		t.Fatalf("failed to resolve migrations path: %v", err)
	}

	if err := migrations.Up(ctx, "sqlite", dbPath, migrationsDir); err != nil {
		t.Fatalf("first migration run failed: %v", err)
	}

	firstVersion, firstAppliedCount := currentGooseVersionState(t, "sqlite", dbPath)

	if err := migrations.Up(ctx, "sqlite", dbPath, migrationsDir); err != nil {
		t.Fatalf("second migration run failed: %v", err)
	}

	secondVersion, secondAppliedCount := currentGooseVersionState(t, "sqlite", dbPath)

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

func currentGooseVersionState(t *testing.T, driver, connStr string) (int64, int) {
	t.Helper()

	db, err := sql.Open(driver, connStr)
	if err != nil {
		t.Fatalf("failed to create db conn: %v", err)
	}
	defer db.Close()

	whereClause := "is_applied = true"
	if driver == "sqlite" {
		whereClause = "is_applied = 1"
	}

	var version int64
	var appliedCount int
	if err := db.QueryRow(`
		SELECT COALESCE(MAX(version_id), 0), COUNT(*)
		FROM goose_db_version
		WHERE ` + whereClause).Scan(&version, &appliedCount); err != nil {
		t.Fatalf("failed to query goose version state: %v", err)
	}

	return version, appliedCount
}
