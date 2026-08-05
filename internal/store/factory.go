package store

import (
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"github.com/hddq/podgist/internal/store/postgres"
	"github.com/hddq/podgist/internal/store/sqlite"
)

func New(driver, dsn string) (Store, error) {
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}

	switch driver {
	case "postgres", "pgx":
		return postgres.NewStore(db), nil
	case "sqlite", "sqlite3":
		return sqlite.NewStore(db), nil
	default:
		return nil, fmt.Errorf("unsupported driver: %s", driver)
	}
}
