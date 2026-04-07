package migrations

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

const defaultDir = "/app/migrations"

func Dir(custom string) string {
	if custom != "" {
		return custom
	}
	if envDir := os.Getenv("PODGIST_MIGRATIONS_DIR"); envDir != "" {
		return envDir
	}
	return defaultDir
}

func Up(ctx context.Context, dsn, dir string) error {
	resolvedDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolve migrations dir: %w", err)
	}

	db, err := goose.OpenDBWithDriver("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open db for migrations: %w", err)
	}
	defer db.Close()

	if err := goose.UpContext(ctx, db, resolvedDir); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	return nil
}
