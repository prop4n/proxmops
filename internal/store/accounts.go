package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrNotFound is returned when a lookup matches no row.
var ErrNotFound = errors.New("store: not found")

// Account is a user of the proxmops UI.
type Account struct {
	ID           int64
	Username     string
	PasswordHash string
	CreatedAt    time.Time
}

// CountAccounts returns how many accounts exist.
func (s *Store) CountAccounts(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM accounts").Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count accounts: %w", err)
	}
	return n, nil
}

// CreateAccount inserts a new account and returns it.
func (s *Store) CreateAccount(ctx context.Context, username, passwordHash string) (Account, error) {
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO accounts (username, password_hash) VALUES (?, ?)",
		username, passwordHash)
	if err != nil {
		return Account{}, fmt.Errorf("create account: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Account{}, err
	}
	return s.AccountByID(ctx, id)
}

// AccountByUsername returns the account with the given username.
func (s *Store) AccountByUsername(ctx context.Context, username string) (Account, error) {
	return s.scanAccount(s.db.QueryRowContext(ctx,
		"SELECT id, username, password_hash, created_at FROM accounts WHERE username = ?", username))
}

// AccountByID returns the account with the given id.
func (s *Store) AccountByID(ctx context.Context, id int64) (Account, error) {
	return s.scanAccount(s.db.QueryRowContext(ctx,
		"SELECT id, username, password_hash, created_at FROM accounts WHERE id = ?", id))
}

// scanAccount maps a single row to an Account, translating no-rows to ErrNotFound.
func (s *Store) scanAccount(row *sql.Row) (Account, error) {
	var a Account
	err := row.Scan(&a.ID, &a.Username, &a.PasswordHash, &a.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, fmt.Errorf("scan account: %w", err)
	}
	return a, nil
}
