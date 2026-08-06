package store

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"github.com/hddq/podgist/internal/store/postgres"
	"github.com/hddq/podgist/internal/store/sqlite"
)

func New(driver, dsn string) (Store, error) {
	if driver == "sqlite" || driver == "sqlite3" {
		dsn = sqliteDSN(dsn)
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}

	switch driver {
	case "postgres", "pgx":
		return postgres.NewStore(db), nil
	case "sqlite", "sqlite3":
		// SQLite only supports a single writer at a time. Limiting to 1 open
		// connection prevents "database is locked" errors under concurrent
		// HTTP requests. WAL mode (set via DSN pragmas) still allows
		// concurrent reads from separate connections, but with a single
		// connection the pool serialises all access which is the safest
		// option for a low-traffic self-hosted server.
		db.SetMaxOpenConns(1)
		return sqlite.NewStore(db), nil
	default:
		return nil, fmt.Errorf("unsupported driver: %s", driver)
	}
}

// sqliteDSN ensures the SQLite DSN includes essential pragmas for
// reliable concurrent access: WAL journal mode, a busy timeout, and
// foreign key enforcement.
func sqliteDSN(dsn string) string {
	// modernc.org/sqlite uses URI-style DSNs:
	//   file:/path/to/db?_pragma=journal_mode(WAL)
	// Plain paths like "/data/podgist.db" also work but need conversion.
	if !strings.HasPrefix(dsn, "file:") {
		dsn = "file:" + dsn
	}

	u, err := url.Parse(dsn)
	if err != nil {
		// If we can't parse it, return as-is and let sql.Open report the error.
		return dsn
	}

	q := u.Query()

	// Collect existing _pragma values so we don't duplicate.
	existing := make(map[string]bool)
	for _, v := range q["_pragma"] {
		key := strings.SplitN(v, "(", 2)[0]
		existing[strings.ToLower(key)] = true
	}

	if !existing["journal_mode"] {
		q.Add("_pragma", "journal_mode(WAL)")
	}
	if !existing["busy_timeout"] {
		q.Add("_pragma", "busy_timeout(5000)")
	}
	if !existing["foreign_keys"] {
		q.Add("_pragma", "foreign_keys(ON)")
	}

	u.RawQuery = q.Encode()
	return u.String()
}
