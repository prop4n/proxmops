// Package store persists proxmops application state (user accounts and
// sessions) in a local SQLite database. It uses the pure-Go modernc.org/sqlite
// driver so the binary stays cgo-free and works in a distroless image.
package store

import (
	"database/sql"
	"fmt"

	// Registers the cgo-free "sqlite" driver for database/sql.
	_ "modernc.org/sqlite"
)

// schema is applied on every Open; statements are idempotent.
const schema = `
CREATE TABLE IF NOT EXISTS accounts (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	username      TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sessions (
	token_hash TEXT PRIMARY KEY,
	account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
	expires_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS settings (
	id         INTEGER PRIMARY KEY CHECK (id = 1),
	data       BLOB NOT NULL,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

// Store is a handle to the application database.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the SQLite database at path and applies the
// schema.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	// One connection avoids "database is locked" under concurrent handlers.
	db.SetMaxOpenConns(1)
	// WAL and busy_timeout ease contention; foreign_keys enables the cascade.
	for _, pragma := range []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA busy_timeout = 5000;",
		"PRAGMA foreign_keys = ON;",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("apply %q: %w", pragma, err)
		}
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the underlying database.
func (s *Store) Close() error { return s.db.Close() }
