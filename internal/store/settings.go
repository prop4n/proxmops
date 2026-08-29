package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// LoadSettings returns the stored settings blob, or ErrNotFound when the
// daemon has not been configured from the web UI yet.
func (s *Store) LoadSettings(ctx context.Context) ([]byte, error) {
	var data []byte
	err := s.db.QueryRowContext(ctx, "SELECT data FROM settings WHERE id = 1").Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load settings: %w", err)
	}
	return data, nil
}

// SaveSettings replaces the settings blob (single row).
func (s *Store) SaveSettings(ctx context.Context, data []byte) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO settings (id, data, updated_at) VALUES (1, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(id) DO UPDATE SET data = excluded.data, updated_at = excluded.updated_at`,
		data)
	if err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	return nil
}
