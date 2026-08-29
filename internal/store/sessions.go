package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Session ties an opaque session token (stored hashed) to an account.
type Session struct {
	TokenHash string
	AccountID int64
	ExpiresAt time.Time
}

// CreateSession stores a session.
func (s *Store) CreateSession(ctx context.Context, tokenHash string, accountID int64, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO sessions (token_hash, account_id, expires_at) VALUES (?, ?, ?)",
		tokenHash, accountID, expiresAt)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// Session returns the session for a token hash, or ErrNotFound.
func (s *Store) Session(ctx context.Context, tokenHash string) (Session, error) {
	var sess Session
	err := s.db.QueryRowContext(ctx,
		"SELECT token_hash, account_id, expires_at FROM sessions WHERE token_hash = ?", tokenHash).
		Scan(&sess.TokenHash, &sess.AccountID, &sess.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("get session: %w", err)
	}
	return sess, nil
}

// DeleteSession removes a session by token hash.
func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash = ?", tokenHash)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// DeleteExpiredSessions removes sessions that expired at or before now.
func (s *Store) DeleteExpiredSessions(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at <= ?", now)
	if err != nil {
		return fmt.Errorf("delete expired sessions: %w", err)
	}
	return nil
}
