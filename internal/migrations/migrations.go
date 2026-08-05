package migrations

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
	"github.com/pressly/goose/v3"
)

const defaultDir = "/app/migrations"

func Dir(driver, custom string) string {
	if custom != "" {
		return custom
	}
	if envDir := os.Getenv("PODGIST_MIGRATIONS_DIR"); envDir != "" {
		return envDir
	}
	sub := "postgres"
	if driver == "sqlite" || driver == "sqlite3" {
		sub = "sqlite"
	}
	return filepath.Join(defaultDir, sub)
}

func Up(ctx context.Context, driver, dsn, dir string) error {
	resolvedDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolve migrations dir: %w", err)
	}

	gooseDriver := "pgx"
	if driver == "sqlite" || driver == "sqlite3" {
		gooseDriver = "sqlite3"
	}

	db, err := goose.OpenDBWithDriver(gooseDriver, dsn)
	if err != nil {
		return fmt.Errorf("open db for migrations: %w", err)
	}
	defer db.Close()

	if err := goose.UpContext(ctx, db, resolvedDir); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	return nil
}
