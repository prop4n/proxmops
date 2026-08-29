// Package store persists proxmops application state (user accounts and
// sessions) in a local SQLite database. It uses the pure-Go modernc.org/sqlite
// driver so the binary stays cgo-free and works in a distroless image.
package store

import (
	"database/sql"
	"fmt"

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
	// SQLite tolerates a single writer; keep one connection to avoid "database
	// is locked" under the daemon's concurrent handlers.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the underlying database.
func (s *Store) Close() error { return s.db.Close() }
