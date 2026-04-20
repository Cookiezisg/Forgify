package storage

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"sync"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

var (
	db   *sql.DB
	mu   sync.Mutex
	once sync.Once
)

func Init(dataDir string) error {
	var initErr error
	once.Do(func() {
		if err := ensureDir(dataDir); err != nil {
			initErr = err
			return
		}
		conn, err := sql.Open("sqlite", dataDir+"/forgify.db?_journal_mode=WAL&_busy_timeout=5000")
		if err != nil {
			initErr = err
			return
		}
		conn.SetMaxOpenConns(1)
		db = conn
		initErr = migrate(conn)
	})
	return initErr
}

func DB() *sql.DB { return db }

func Exec(query string, args ...any) (sql.Result, error) {
	mu.Lock()
	defer mu.Unlock()
	return db.Exec(query, args...)
}

func migrate(conn *sql.DB) error {
	if _, err := conn.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at DATETIME DEFAULT (datetime('now'))
	)`); err != nil {
		return err
	}

	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		var version int
		fmt.Sscanf(e.Name(), "%d_", &version)

		var count int
		if err := conn.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version=?", version).Scan(&count); err != nil {
			return fmt.Errorf("check migration %d: %w", version, err)
		}
		if count > 0 {
			continue
		}

		sqlBytes, err := fs.ReadFile(migrationFiles, "migrations/"+e.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", e.Name(), err)
		}
		if _, err := conn.Exec(string(sqlBytes)); err != nil {
			return fmt.Errorf("migration %d: %w", version, err)
		}
		if _, err := conn.Exec("INSERT INTO schema_migrations (version) VALUES (?)", version); err != nil {
			return fmt.Errorf("record migration %d: %w", version, err)
		}
	}
	return nil
}
